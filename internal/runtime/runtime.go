package runtime

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func Root() string {
	return TempRoot()
}

func TempRoot() string {
	if value := strings.TrimSpace(os.Getenv("DOWNARIA_API_TEMP_DIR")); value != "" {
		return value
	}
	return filepath.Join(os.TempDir(), "downaria-api")
}

func StatsDir() string {
	if value := strings.TrimSpace(os.Getenv("DOWNARIA_API_STATS_DIR")); value != "" {
		return value
	}
	base := userLocalDataDir()
	if strings.TrimSpace(base) == "" {
		base = filepath.Join(os.TempDir(), "downaria-api")
	}
	return filepath.Join(base, "DownAria-API", "stats")
}

func EnsureStatsDir() (string, error) {
	dir := StatsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func Subdir(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return Root()
	}
	return filepath.Join(Root(), name)
}

func EnsureSubdir(name string) (string, error) {
	dir := Subdir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func MinimalEnv() []string {
	keys := []string{"PATH", "SYSTEMROOT", "WINDIR", "HOME", "TMP", "TEMP"}
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func SetupCleanup(ctx context.Context) {
	_ = ctx
}

func userLocalDataDir() string {
	if runtime.GOOS == "windows" {
		if value := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); value != "" {
			return value
		}
	}
	if value := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); value != "" {
		return value
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Application Support")
		}
		return filepath.Join(home, ".local", "share")
	}
	return ""
}
