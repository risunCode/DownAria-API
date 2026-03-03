package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRecordVisitor(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "stats.json")

	opts := PublicStatsPersistenceOptions{
		Enabled:  false,
		FilePath: tempFile,
	}

	now := time.Now()
	store := NewPublicStatsStore(now, opts)

	t.Run("visitor is recorded", func(t *testing.T) {
		store.RecordVisitor("visitor-1", now)
		snapshot := store.Snapshot(now)

		if snapshot.TotalVisits != 1 {
			t.Errorf("expected TotalVisits to be 1, got %d", snapshot.TotalVisits)
		}
		if snapshot.TodayVisits != 1 {
			t.Errorf("expected TodayVisits to be 1, got %d", snapshot.TodayVisits)
		}
	})

	t.Run("same visitor not counted twice", func(t *testing.T) {
		store.RecordVisitor("visitor-1", now)
		snapshot := store.Snapshot(now)

		if snapshot.TotalVisits != 1 {
			t.Errorf("expected TotalVisits to be 1, got %d", snapshot.TotalVisits)
		}
	})

	t.Run("different visitors counted separately", func(t *testing.T) {
		store.RecordVisitor("visitor-2", now)
		store.RecordVisitor("visitor-3", now)
		snapshot := store.Snapshot(now)

		if snapshot.TotalVisits != 3 {
			t.Errorf("expected TotalVisits to be 3, got %d", snapshot.TotalVisits)
		}
	})
}

func TestRecordExtraction(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "stats.json")

	opts := PublicStatsPersistenceOptions{
		Enabled:  false,
		FilePath: tempFile,
	}

	now := time.Now()
	store := NewPublicStatsStore(now, opts)

	t.Run("extraction count increments", func(t *testing.T) {
		store.RecordExtraction(now)
		snapshot := store.Snapshot(now)

		if snapshot.TotalExtractions != 1 {
			t.Errorf("expected TotalExtractions to be 1, got %d", snapshot.TotalExtractions)
		}
	})

	t.Run("multiple extractions accumulate", func(t *testing.T) {
		store.RecordExtraction(now)
		store.RecordExtraction(now)
		store.RecordExtraction(now)
		snapshot := store.Snapshot(now)

		if snapshot.TotalExtractions != 4 {
			t.Errorf("expected TotalExtractions to be 4, got %d", snapshot.TotalExtractions)
		}
	})
}

func TestRecordDownload(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "stats.json")

	opts := PublicStatsPersistenceOptions{
		Enabled:  false,
		FilePath: tempFile,
	}

	now := time.Now()
	store := NewPublicStatsStore(now, opts)

	t.Run("download count increments", func(t *testing.T) {
		store.RecordDownload(now)
		store.RecordDownload(now)
		snapshot := store.Snapshot(now)

		if snapshot.TotalDownloads != 2 {
			t.Errorf("expected TotalDownloads to be 2, got %d", snapshot.TotalDownloads)
		}
	})
}

func TestSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "stats.json")

	opts := PublicStatsPersistenceOptions{
		Enabled:  false,
		FilePath: tempFile,
	}

	now := time.Now()
	store := NewPublicStatsStore(now, opts)

	t.Run("snapshot returns correct values", func(t *testing.T) {
		store.RecordVisitor("v1", now)
		store.RecordExtraction(now)
		store.RecordDownload(now)

		snapshot := store.Snapshot(now)

		if snapshot.TodayVisits != 1 {
			t.Errorf("expected TodayVisits to be 1, got %d", snapshot.TodayVisits)
		}
		if snapshot.TotalVisits != 1 {
			t.Errorf("expected TotalVisits to be 1, got %d", snapshot.TotalVisits)
		}
		if snapshot.TotalExtractions != 1 {
			t.Errorf("expected TotalExtractions to be 1, got %d", snapshot.TotalExtractions)
		}
		if snapshot.TotalDownloads != 1 {
			t.Errorf("expected TotalDownloads to be 1, got %d", snapshot.TotalDownloads)
		}
	})

	t.Run("snapshot after multiple operations", func(t *testing.T) {
		store.RecordVisitor("v2", now)
		store.RecordVisitor("v3", now)
		store.RecordExtraction(now)
		store.RecordExtraction(now)
		store.RecordDownload(now)

		snapshot := store.Snapshot(now)

		if snapshot.TodayVisits != 3 {
			t.Errorf("expected TodayVisits to be 3, got %d", snapshot.TodayVisits)
		}
		if snapshot.TotalVisits != 3 {
			t.Errorf("expected TotalVisits to be 3, got %d", snapshot.TotalVisits)
		}
		if snapshot.TotalExtractions != 3 {
			t.Errorf("expected TotalExtractions to be 3, got %d", snapshot.TotalExtractions)
		}
		if snapshot.TotalDownloads != 2 {
			t.Errorf("expected TotalDownloads to be 2, got %d", snapshot.TotalDownloads)
		}
	})
}

func TestDayRotation(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "stats.json")

	opts := PublicStatsPersistenceOptions{
		Enabled:  false,
		FilePath: tempFile,
	}

	today := time.Now()
	tomorrow := today.Add(24 * time.Hour)

	store := NewPublicStatsStore(today, opts)

	store.RecordVisitor("v1", today)
	store.RecordVisitor("v2", today)
	store.RecordExtraction(today)
	store.RecordDownload(today)

	t.Run("todayVisits reset on new day", func(t *testing.T) {
		snapshot := store.Snapshot(tomorrow)

		if snapshot.TodayVisits != 0 {
			t.Errorf("expected TodayVisits to be 0 after day rotation, got %d", snapshot.TodayVisits)
		}
		if snapshot.TotalVisits != 2 {
			t.Errorf("expected TotalVisits to remain 2, got %d", snapshot.TotalVisits)
		}
		if snapshot.TotalExtractions != 1 {
			t.Errorf("expected TotalExtractions to remain 1, got %d", snapshot.TotalExtractions)
		}
		if snapshot.TotalDownloads != 1 {
			t.Errorf("expected TotalDownloads to remain 1, got %d", snapshot.TotalDownloads)
		}
	})

	t.Run("visitor keys cleared on new day", func(t *testing.T) {
		store.RecordVisitor("v1", tomorrow)
		snapshot := store.Snapshot(tomorrow)

		if snapshot.TodayVisits != 1 {
			t.Errorf("expected TodayVisits to be 1 after recording v1 on new day, got %d", snapshot.TodayVisits)
		}
		if snapshot.TotalVisits != 3 {
			t.Errorf("expected TotalVisits to be 3, got %d", snapshot.TotalVisits)
		}
	})
}

func TestConcurrency(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "stats.json")

	opts := PublicStatsPersistenceOptions{
		Enabled:  false,
		FilePath: tempFile,
	}

	now := time.Now()
	store := NewPublicStatsStore(now, opts)

	t.Run("thread safety with concurrent operations", func(t *testing.T) {
		var wg sync.WaitGroup
		numGoroutines := 100

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				store.RecordVisitor(string(rune('a'+id%26)), now)
			}(i)
		}

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				store.RecordExtraction(now)
			}()
		}

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				store.RecordDownload(now)
			}()
		}

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				_ = store.Snapshot(now)
			}()
		}

		wg.Wait()

		snapshot := store.Snapshot(now)

		if snapshot.TotalVisits != 26 {
			t.Errorf("expected TotalVisits to be 26 (unique visitors), got %d", snapshot.TotalVisits)
		}
		if snapshot.TotalExtractions != int64(numGoroutines) {
			t.Errorf("expected TotalExtractions to be %d, got %d", numGoroutines, snapshot.TotalExtractions)
		}
		if snapshot.TotalDownloads != int64(numGoroutines) {
			t.Errorf("expected TotalDownloads to be %d, got %d", numGoroutines, snapshot.TotalDownloads)
		}
	})
}

func TestNewPublicStatsStore(t *testing.T) {
	t.Run("creates store without persistence", func(t *testing.T) {
		now := time.Now()
		store := NewPublicStatsStore(now)

		if store == nil {
			t.Fatal("expected store to be created")
		}

		snapshot := store.Snapshot(now)
		if snapshot.TotalVisits != 0 {
			t.Errorf("expected initial TotalVisits to be 0, got %d", snapshot.TotalVisits)
		}
	})

	t.Run("creates store with persistence options", func(t *testing.T) {
		tempDir := t.TempDir()
		tempFile := filepath.Join(tempDir, "stats.json")

		opts := PublicStatsPersistenceOptions{
			Enabled:       true,
			FilePath:      tempFile,
			FlushInterval: time.Second,
		}

		now := time.Now()
		store := NewPublicStatsStore(now, opts)
		defer func() { _ = store.Close() }()

		if store == nil {
			t.Fatal("expected store to be created")
		}

		if !store.persistenceEnabled {
			t.Error("expected persistence to be enabled")
		}
		if store.persistFilePath != tempFile {
			t.Errorf("expected persistFilePath to be %s, got %s", tempFile, store.persistFilePath)
		}
	})
}

func TestRecordVisitorAnonymous(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "stats.json")

	opts := PublicStatsPersistenceOptions{
		Enabled:  false,
		FilePath: tempFile,
	}

	now := time.Now()
	store := NewPublicStatsStore(now, opts)
	defer func() { _ = store.Close() }()

	t.Run("empty visitor key treated as anonymous", func(t *testing.T) {
		store.RecordVisitor("", now)
		store.RecordVisitor("   ", now)
		snapshot := store.Snapshot(now)

		if snapshot.TotalVisits != 1 {
			t.Errorf("expected TotalVisits to be 1 (anonymous counted once), got %d", snapshot.TotalVisits)
		}
	})
}

func TestLoadFromDisk(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "stats.json")

	state := publicStatsPersistedState{
		DayKey:           time.Now().UTC().Format("2006-01-02"),
		TodayVisits:      5,
		TotalVisits:      100,
		TotalExtractions: 50,
		TotalDownloads:   25,
		SeenVisitorKeys:  []string{"v1", "v2", "v3", "v4", "v5"},
	}

	data := []byte(`{
		"dayKey": "` + state.DayKey + `",
		"todayVisits": 5,
		"totalVisits": 100,
		"totalExtractions": 50,
		"totalDownloads": 25,
		"seenVisitorKeys": ["v1", "v2", "v3", "v4", "v5"]
	}`)

	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	opts := PublicStatsPersistenceOptions{
		Enabled:  true,
		FilePath: tempFile,
	}

	now := time.Now()
	store := NewPublicStatsStore(now, opts)

	snapshot := store.Snapshot(now)

	if snapshot.TodayVisits != 5 {
		t.Errorf("expected TodayVisits to be 5 from disk, got %d", snapshot.TodayVisits)
	}
	if snapshot.TotalVisits != 100 {
		t.Errorf("expected TotalVisits to be 100 from disk, got %d", snapshot.TotalVisits)
	}
	if snapshot.TotalExtractions != 50 {
		t.Errorf("expected TotalExtractions to be 50 from disk, got %d", snapshot.TotalExtractions)
	}
	if snapshot.TotalDownloads != 25 {
		t.Errorf("expected TotalDownloads to be 25 from disk, got %d", snapshot.TotalDownloads)
	}

	store.RecordVisitor("v1", now)
	snapshot = store.Snapshot(now)
	if snapshot.TodayVisits != 5 {
		t.Errorf("expected TodayVisits to remain 5 (v1 already seen), got %d", snapshot.TodayVisits)
	}
}

func TestPersistenceFlushByThreshold(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "stats.json")

	store := NewPublicStatsStore(time.Now(), PublicStatsPersistenceOptions{
		Enabled:        true,
		FilePath:       tempFile,
		FlushInterval:  10 * time.Second,
		FlushThreshold: 3,
	})
	defer func() { _ = store.Close() }()

	now := time.Now()
	store.RecordExtraction(now)
	store.RecordExtraction(now)
	store.RecordExtraction(now)

	state := waitForPersistedState(t, tempFile, 2*time.Second, func(s publicStatsPersistedState) bool {
		return s.TotalExtractions == 3
	})

	if state.TotalExtractions != 3 {
		t.Fatalf("expected TotalExtractions 3, got %d", state.TotalExtractions)
	}
}

func TestPersistenceFlushByInterval(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "stats.json")

	store := NewPublicStatsStore(time.Now(), PublicStatsPersistenceOptions{
		Enabled:        true,
		FilePath:       tempFile,
		FlushInterval:  time.Second,
		FlushThreshold: 100,
	})
	defer func() { _ = store.Close() }()

	store.RecordDownload(time.Now())

	state := waitForPersistedState(t, tempFile, 3*time.Second, func(s publicStatsPersistedState) bool {
		return s.TotalDownloads == 1
	})

	if state.TotalDownloads != 1 {
		t.Fatalf("expected TotalDownloads 1, got %d", state.TotalDownloads)
	}
}

func TestPersistenceFlushOnClose(t *testing.T) {
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "stats.json")

	store := NewPublicStatsStore(time.Now(), PublicStatsPersistenceOptions{
		Enabled:        true,
		FilePath:       tempFile,
		FlushInterval:  10 * time.Second,
		FlushThreshold: 100,
	})

	store.RecordVisitor("visitor-close", time.Now())

	if err := store.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	raw, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("expected persisted file after close: %v", err)
	}

	var state publicStatsPersistedState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("failed to parse persisted state: %v", err)
	}

	if state.TotalVisits != 1 {
		t.Fatalf("expected TotalVisits 1, got %d", state.TotalVisits)
	}
}

func waitForPersistedState(t *testing.T, path string, timeout time.Duration, match func(publicStatsPersistedState) bool) publicStatsPersistedState {
	t.Helper()

	deadline := time.Now().Add(timeout)
	lastErr := fmt.Errorf("state did not match before timeout")

	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			var state publicStatsPersistedState
			if unmarshalErr := json.Unmarshal(raw, &state); unmarshalErr == nil {
				if match(state) {
					return state
				}
				lastErr = fmt.Errorf("persisted state not matched yet")
			} else {
				lastErr = unmarshalErr
			}
		} else {
			lastErr = err
		}

		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for persisted state in %s: %v", path, lastErr)
	return publicStatsPersistedState{}
}
