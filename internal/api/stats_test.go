package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatsStoreUsesPersistentStatsDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "stats")
	t.Setenv("DOWNARIA_API_STATS_DIR", dir)

	store := newStatsStore()
	if got, want := store.filePath, filepath.Join(dir, "stats.json"); got != want {
		t.Fatalf("filePath = %q, want %q", got, want)
	}

	store.recordDownload(2)
	if _, err := os.Stat(store.filePath); err != nil {
		t.Fatalf("stats file not persisted: %v", err)
	}
}
