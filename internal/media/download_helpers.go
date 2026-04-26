package media

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func sanitizeHeaderValue(value string) string {
	clean := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == 0 {
			return -1
		}
		return r
	}, value)
	return strings.TrimSpace(clean)
}

func sanitizeHeaderToken(value string) string {
	clean := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == 0 || r == ':' {
			return -1
		}
		return r
	}, value)
	return strings.TrimSpace(clean)
}

func resolveDownloadedFile(dir string, expected string) (string, os.FileInfo, error) {
	if info, err := os.Stat(expected); err == nil {
		return expected, info, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		return path, info, nil
	}
	return "", nil, fmt.Errorf("yt-dlp output missing")
}
