package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestJobManagerCompletesAndStoresError(t *testing.T) {
	artifacts, err := NewArtifactStore(filepath.Join(t.TempDir(), "artifacts"), time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewJobManager(filepath.Join(t.TempDir(), "jobs"), artifacts, time.Minute, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	job, err := manager.Create("download", func(ctx context.Context, update func(JobUpdate)) (*Artifact, JobMetadata, *JobError) {
		return nil, JobMetadata{SelectedFormats: []string{"720p"}}, &JobError{Code: "boom", Message: "failed"}
	})
	if err != nil {
		t.Fatal(err)
	}
	got := waitJob(t, manager, job.ID)
	if got.State != StateFailed || got.Error == nil || got.Error.Code != "boom" {
		t.Fatalf("job = %#v", got)
	}
}

func TestJobManagerCancelStopsRunningJob(t *testing.T) {
	artifacts, err := NewArtifactStore(filepath.Join(t.TempDir(), "artifacts"), time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewJobManager(filepath.Join(t.TempDir(), "jobs"), artifacts, time.Minute, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	started := make(chan struct{})
	job, err := manager.Create("download", func(ctx context.Context, update func(JobUpdate)) (*Artifact, JobMetadata, *JobError) {
		close(started)
		<-ctx.Done()
		return nil, JobMetadata{}, &JobError{Code: "cancelled", Message: ctx.Err().Error()}
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started

	if err := manager.Cancel(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}

	got := waitJobState(t, manager, job.ID, StateCancelled)
	if got.Error == nil || got.Error.Code != "job_cancelled" {
		t.Fatalf("job = %#v", got)
	}
	if active := manager.ActiveCount(); active != 0 {
		t.Fatalf("active count = %d", active)
	}
}

func TestJobManagerCancelMissingJob(t *testing.T) {
	manager, err := NewJobManager(filepath.Join(t.TempDir(), "jobs"), nil, time.Minute, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	if err := manager.Cancel(context.Background(), "missing"); err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func waitJob(t *testing.T, manager *JobManager, id string) *Job {
	return waitJobState(t, manager, id, StateCompleted, StateFailed)
}

func waitJobState(t *testing.T, manager *JobManager, id string, states ...string) *Job {
	t.Helper()
	want := map[string]struct{}{}
	for _, state := range states {
		want[state] = struct{}{}
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, err := manager.Get(id)
		if err == nil {
			if _, ok := want[job.State]; ok {
				return job
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish", id)
	return nil
}
