package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"downaria-api/internal/core/ports"
)

// Ensure PublicStatsStore implements ports.StatsStore
var _ ports.StatsStore = (*PublicStatsStore)(nil)

// PublicStatsSnapshot is kept for backward compatibility
type PublicStatsSnapshot = ports.StatsSnapshot

type PublicStatsPersistenceOptions struct {
	Enabled        bool
	FilePath       string
	FlushInterval  time.Duration
	FlushThreshold int
}

type publicStatsPersistedState struct {
	DayKey           string   `json:"dayKey"`
	TodayVisits      int64    `json:"todayVisits"`
	TotalVisits      int64    `json:"totalVisits"`
	TotalExtractions int64    `json:"totalExtractions"`
	TotalDownloads   int64    `json:"totalDownloads"`
	SeenVisitorKeys  []string `json:"seenVisitorKeys"`
}

type PublicStatsStore struct {
	mu sync.Mutex

	dayKey string

	todayVisits      int64
	totalVisits      int64
	totalExtractions int64
	totalDownloads   int64

	seenVisitorKeys map[string]struct{}

	persistenceEnabled bool
	persistFilePath    string
	flushInterval      time.Duration
	flushThreshold     int
	dirty              bool
	dirtySeq           uint64

	buffer *statsBuffer
}

func NewPublicStatsStore(now time.Time, opts ...PublicStatsPersistenceOptions) *PublicStatsStore {
	store := &PublicStatsStore{
		dayKey:          dayStringUTC(now),
		seenVisitorKeys: make(map[string]struct{}),
		flushInterval:   5 * time.Second,
		flushThreshold:  10,
	}

	if len(opts) > 0 {
		opt := opts[0]
		store.persistenceEnabled = opt.Enabled
		store.persistFilePath = strings.TrimSpace(opt.FilePath)
		if opt.FlushInterval >= time.Second {
			store.flushInterval = opt.FlushInterval
		}
		if opt.FlushThreshold > 0 {
			store.flushThreshold = opt.FlushThreshold
		}
	}

	if store.persistenceEnabled {
		if err := store.loadFromDisk(now); err != nil {
			log.Printf("[stats] persistence load skipped: %v", err)
		}
		store.buffer = newStatsBuffer(store.flushThreshold, store.flushInterval, store.flushIfDirty, func(err error) {
			log.Printf("[stats] persistence flush failed: %v", err)
		})
	}

	return store
}

func (s *PublicStatsStore) RecordVisitor(visitorKey string, now time.Time) {
	key := strings.TrimSpace(visitorKey)
	if key == "" {
		key = "anonymous"
	}

	s.mu.Lock()

	s.rotateIfNeeded(now)
	if _, exists := s.seenVisitorKeys[key]; exists {
		s.mu.Unlock()
		return
	}

	s.seenVisitorKeys[key] = struct{}{}
	s.todayVisits++
	s.totalVisits++
	s.markDirty()
	buffer := s.buffer
	s.mu.Unlock()

	if buffer != nil {
		buffer.Record()
	}
}

func (s *PublicStatsStore) RecordExtraction(now time.Time) {
	s.mu.Lock()

	s.rotateIfNeeded(now)
	s.totalExtractions++
	s.markDirty()
	buffer := s.buffer
	s.mu.Unlock()

	if buffer != nil {
		buffer.Record()
	}
}

func (s *PublicStatsStore) RecordDownload(now time.Time) {
	s.mu.Lock()

	s.rotateIfNeeded(now)
	s.totalDownloads++
	s.markDirty()
	buffer := s.buffer
	s.mu.Unlock()

	if buffer != nil {
		buffer.Record()
	}
}

func (s *PublicStatsStore) Snapshot(now time.Time) ports.StatsSnapshot {
	s.mu.Lock()
	buffer := s.buffer
	rotated := false

	if s.rotateIfNeeded(now) {
		s.markDirty()
		rotated = true
	}

	snapshot := ports.StatsSnapshot{
		TodayVisits:      s.todayVisits,
		TotalVisits:      s.totalVisits,
		TotalExtractions: s.totalExtractions,
		TotalDownloads:   s.totalDownloads,
	}
	s.mu.Unlock()

	if rotated && buffer != nil {
		buffer.Record()
	}

	return snapshot
}

func (s *PublicStatsStore) rotateIfNeeded(now time.Time) bool {
	currentDay := dayStringUTC(now)
	if currentDay == s.dayKey {
		return false
	}

	s.dayKey = currentDay
	s.todayVisits = 0
	s.seenVisitorKeys = make(map[string]struct{})
	return true
}

func (s *PublicStatsStore) markDirty() {
	if !s.persistenceEnabled {
		return
	}
	s.dirty = true
	s.dirtySeq++
}

func (s *PublicStatsStore) loadFromDisk(now time.Time) error {
	if s.persistFilePath == "" {
		return fmt.Errorf("persist file path is empty")
	}

	raw, err := os.ReadFile(s.persistFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	var state publicStatsPersistedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}

	seen := make(map[string]struct{}, len(state.SeenVisitorKeys))
	for _, key := range state.SeenVisitorKeys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		seen[trimmed] = struct{}{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.dayKey = strings.TrimSpace(state.DayKey)
	if s.dayKey == "" {
		s.dayKey = dayStringUTC(now)
	}
	s.todayVisits = state.TodayVisits
	s.totalVisits = state.TotalVisits
	s.totalExtractions = state.TotalExtractions
	s.totalDownloads = state.TotalDownloads
	s.seenVisitorKeys = seen
	s.dirty = false

	return nil
}

func (s *PublicStatsStore) flushIfDirty() error {
	state, dirtySeq, ok := s.snapshotForPersist()
	if !ok {
		return nil
	}

	if err := writePersistedStateAtomic(s.persistFilePath, state); err != nil {
		return err
	}

	s.mu.Lock()
	if s.dirty && s.dirtySeq == dirtySeq {
		s.dirty = false
	}
	s.mu.Unlock()

	return nil
}

func (s *PublicStatsStore) Close() error {
	s.mu.Lock()
	buffer := s.buffer
	s.buffer = nil
	s.mu.Unlock()

	if buffer != nil {
		buffer.Stop()
	}

	return s.flushIfDirty()
}

func (s *PublicStatsStore) snapshotForPersist() (publicStatsPersistedState, uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.persistenceEnabled || !s.dirty || s.persistFilePath == "" {
		return publicStatsPersistedState{}, 0, false
	}

	seen := make([]string, 0, len(s.seenVisitorKeys))
	for key := range s.seenVisitorKeys {
		seen = append(seen, key)
	}
	sort.Strings(seen)

	state := publicStatsPersistedState{
		DayKey:           s.dayKey,
		TodayVisits:      s.todayVisits,
		TotalVisits:      s.totalVisits,
		TotalExtractions: s.totalExtractions,
		TotalDownloads:   s.totalDownloads,
		SeenVisitorKeys:  seen,
	}

	return state, s.dirtySeq, true
}

func writePersistedStateAtomic(path string, state publicStatsPersistedState) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("persist path is empty")
	}

	dirPath := filepath.Dir(path)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	tempFile, err := os.CreateTemp(dirPath, ".public_stats_*.tmp")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()

	if _, err := tempFile.Write(raw); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	if err := os.Rename(tempPath, path); err == nil {
		return nil
	}

	if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		_ = os.Remove(tempPath)
		return removeErr
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	return nil
}

func dayStringUTC(now time.Time) string {
	return now.UTC().Format("2006-01-02")
}
