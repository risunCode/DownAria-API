package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootDefaultsToTempRoot(t *testing.T) {
	t.Setenv("DOWNARIA_API_TEMP_DIR", "")

	if got, want := Root(), filepath.Join(os.TempDir(), "downaria-api"); got != want {
		t.Fatalf("Root() = %q, want %q", got, want)
	}
}

func TestTempDirOverrideSetsRoot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "temp")
	t.Setenv("DOWNARIA_API_TEMP_DIR", dir)

	if got, want := Root(), dir; got != want {
		t.Fatalf("Root() = %q, want %q", got, want)
	}
}

func TestStatsDirUsesDedicatedOverride(t *testing.T) {
	t.Setenv("DOWNARIA_API_TEMP_DIR", filepath.Join(t.TempDir(), "temp"))
	t.Setenv("DOWNARIA_API_STATS_DIR", filepath.Join(t.TempDir(), "stats"))

	if got, want := StatsDir(), os.Getenv("DOWNARIA_API_STATS_DIR"); got != want {
		t.Fatalf("StatsDir() = %q, want %q", got, want)
	}
}

func TestEnsureStatsDirCreatesPersistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "stats")
	t.Setenv("DOWNARIA_API_STATS_DIR", dir)

	got, err := EnsureStatsDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("EnsureStatsDir() = %q, want %q", got, dir)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("stats dir not created: info=%v err=%v", info, err)
	}
}
