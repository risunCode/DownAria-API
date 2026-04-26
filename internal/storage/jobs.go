package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	runtime "downaria-api/internal/runtime"
)

const (
	StatePending     = "pending"
	StateDownloading = "downloading"
	StateMerging     = "merging"
	StateConverting  = "converting"
	StateCompleted   = "completed"
	StateFailed      = "failed"
	StateExpired     = "expired"
	StateCancelled   = "cancelled"
)

type JobError struct {
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}

type Job struct {
	ID              string    `json:"id"`
	Type            string    `json:"type"`
	State           string    `json:"state"`
	CreatedAt       time.Time `json:"created_at"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
	Message         string    `json:"message,omitempty"`
	Progress        int       `json:"progress,omitempty"` // 0-100
	SelectedFormats []string  `json:"selected_formats,omitempty"`
	Artifact        *Artifact `json:"artifact,omitempty"`
	Error           *JobError `json:"error,omitempty"`
}

type JobMetadata struct {
	SelectedFormats []string
}

type JobManager struct {
	dir        string
	artifacts  *ArtifactStore
	jobTimeout time.Duration
	mu         sync.Mutex
	fileMu     sync.Mutex
	active     map[string]time.Time
	cancels    map[string]context.CancelFunc
	semaphore  chan struct{}
	done       chan struct{}
	closeOnce  sync.Once
}

type JobUpdate struct {
	State    string
	Message  string
	Progress int
}

type JobRunner func(context.Context, func(JobUpdate)) (*Artifact, JobMetadata, *JobError)

func NewJobManager(dir string, artifactStore *ArtifactStore, jobTimeout time.Duration, maxConcurrent int) (*JobManager, error) {
	if strings.TrimSpace(dir) == "" {
		dir = runtime.Subdir("jobs")
	}
	if jobTimeout <= 0 {
		jobTimeout = 30 * time.Minute
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 10
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create job dir: %w", err)
	}
	m := &JobManager{
		dir:        dir,
		artifacts:  artifactStore,
		jobTimeout: jobTimeout,
		active:     map[string]time.Time{},
		cancels:    map[string]context.CancelFunc{},
		semaphore:  make(chan struct{}, maxConcurrent),
		done:       make(chan struct{}),
	}
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				m.cleanupStuckJobs(30 * time.Minute)
			case <-m.done:
				return
			}
		}
	}()
	return m, nil
}

// Close stops background goroutines. Safe to call multiple times.
func (m *JobManager) Close() error {
	m.closeOnce.Do(func() { close(m.done) })
	return nil
}

func (m *JobManager) Create(jobType string, runner JobRunner) (*Job, error) {
	if m == nil {
		return nil, fmt.Errorf("job manager is nil")
	}
	id, err := randomHexID("job_")
	if err != nil {
		return nil, err
	}
	job := &Job{ID: id, Type: strings.TrimSpace(jobType), State: StatePending, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := m.save(job); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.active[id] = time.Now().UTC()
	m.mu.Unlock()

	select {
	case m.semaphore <- struct{}{}:
	default:
		m.mu.Lock()
		delete(m.active, id)
		m.mu.Unlock()
		return nil, fmt.Errorf("too many concurrent jobs")
	}
	go m.run(job, runner)
	return job, nil
}

func (m *JobManager) run(job *Job, runner JobRunner) {
	defer func() { <-m.semaphore }()
	defer func() {
		if recovered := recover(); recovered != nil {
			m.mu.Lock()
			delete(m.active, job.ID)
			delete(m.cancels, job.ID)
			m.mu.Unlock()
			job.State = StateFailed
			job.Error = &JobError{Code: "job_panic", Message: fmt.Sprintf("job panicked: %v", recovered)}
			job.UpdatedAt = time.Now().UTC()
			_ = m.save(job)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), m.jobTimeout)
	defer cancel()
	m.mu.Lock()
	m.cancels[job.ID] = cancel
	m.mu.Unlock()

	if current, err := m.read(job.ID); err == nil && current.State == StateCancelled {
		m.mu.Lock()
		delete(m.active, job.ID)
		delete(m.cancels, job.ID)
		m.mu.Unlock()
		return
	}

	update := func(u JobUpdate) {
		if ctx.Err() != nil {
			return
		}
		if strings.TrimSpace(u.State) != "" {
			job.State = u.State
		}
		if strings.TrimSpace(u.Message) != "" {
			job.Message = u.Message
		}
		if u.Progress >= 0 {
			job.Progress = u.Progress
		}
		job.UpdatedAt = time.Now().UTC()
		_ = m.save(job)
	}
	job.StartedAt = time.Now().UTC()
	update(JobUpdate{State: stageForType(job.Type)})
	artifact, meta, jobErr := runner(ctx, update)
	m.mu.Lock()
	delete(m.active, job.ID)
	delete(m.cancels, job.ID)
	m.mu.Unlock()
	job.UpdatedAt = time.Now().UTC()
	job.SelectedFormats = meta.SelectedFormats
	if ctx.Err() == context.Canceled {
		job.State = StateCancelled
		job.Error = &JobError{Code: "job_cancelled", Message: "job was cancelled"}
		_ = m.save(job)
		return
	}
	if jobErr != nil {
		job.State = StateFailed
		job.Error = jobErr
		_ = m.save(job)
		return
	}
	job.State = StateCompleted
	job.Artifact = artifact
	job.Error = nil
	_ = m.save(job)
}

func (m *JobManager) Cancel(ctx context.Context, id string) error {
	if m == nil {
		return fmt.Errorf("job manager is nil")
	}
	id = strings.TrimSpace(id)
	job, err := m.read(id)
	if err != nil {
		return err
	}
	switch job.State {
	case StateCompleted, StateFailed, StateExpired, StateCancelled:
		return nil
	}
	m.mu.Lock()
	cancel := m.cancels[id]
	delete(m.active, id)
	delete(m.cancels, id)
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	job.State = StateCancelled
	job.Message = "Job cancelled"
	job.Error = &JobError{Code: "job_cancelled", Message: "job was cancelled"}
	job.UpdatedAt = time.Now().UTC()
	return m.save(job)
}

func (m *JobManager) Get(id string) (*Job, error) {
	if m == nil {
		return nil, fmt.Errorf("job manager is nil")
	}
	job, err := m.read(strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if job.Artifact != nil && m.artifacts != nil {
		artifact, err := m.artifacts.Get(job.Artifact.ID)
		if err != nil {
			job.State = StateExpired
			job.Artifact = nil
			job.UpdatedAt = time.Now().UTC()
			_ = m.save(job)
		} else {
			job.Artifact = artifact
		}
	}
	return job, nil
}

func (m *JobManager) Artifact(id string) (*Job, *Artifact, error) {
	job, err := m.Get(id)
	if err != nil {
		return nil, nil, err
	}
	if job.Artifact == nil {
		return job, nil, fmt.Errorf("artifact is unavailable")
	}
	return job, job.Artifact, nil
}

func (m *JobManager) ActiveCount() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.active)
}

func (m *JobManager) ActiveJobs() []*Job {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	ids := make([]string, 0, len(m.active))
	for id := range m.active {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	var jobs []*Job
	for _, id := range ids {
		if job, err := m.read(id); err == nil {
			jobs = append(jobs, job)
		}
	}
	return jobs
}

func (m *JobManager) cleanupStuckJobs(maxDuration time.Duration) {
	m.mu.Lock()
	var stuck []string
	cutoff := time.Now().UTC().Add(-maxDuration)
	for id, startedAt := range m.active {
		if startedAt.Before(cutoff) {
			stuck = append(stuck, id)
		}
	}
	for _, id := range stuck {
		delete(m.active, id)
	}
	m.mu.Unlock()

	for _, id := range stuck {
		if job, err := m.read(id); err == nil {
			job.State = StateFailed
			job.Error = &JobError{Code: "job_timeout", Message: "job exceeded maximum duration"}
			job.UpdatedAt = time.Now().UTC()
			_ = m.save(job)
		}
	}
}

func (m *JobManager) save(job *Job) error {
	m.fileMu.Lock()
	defer m.fileMu.Unlock()
	path := filepath.Join(m.dir, job.ID+".json")
	if err := writeJSONFile(path, job); err != nil {
		return fmt.Errorf("encode job: %w", err)
	}
	return nil
}

func (m *JobManager) read(id string) (*Job, error) {
	m.fileMu.Lock()
	defer m.fileMu.Unlock()
	data, err := os.ReadFile(filepath.Join(m.dir, id+".json"))
	if err != nil {
		return nil, err
	}
	var job Job
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("decode job: %w", err)
	}
	return &job, nil
}

func stageForType(jobType string) string {
	switch strings.ToLower(strings.TrimSpace(jobType)) {
	case "download":
		return StateDownloading
	case "convert":
		return StateConverting
	case "merge":
		return StateMerging
	default:
		return StatePending
	}
}
