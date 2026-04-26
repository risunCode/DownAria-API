package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"downaria-api/internal/extract"
	runtime "downaria-api/internal/runtime"
)

type Artifact struct {
	ID           string    `json:"id"`
	Path         string    `json:"path"`
	Filename     string    `json:"filename"`
	ContentType  string    `json:"content_type"`
	ContentBytes int64     `json:"content_bytes"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type ArtifactStats struct {
	Entries   int   `json:"entries"`
	BytesUsed int64 `json:"bytes_used"`
}

type ArtifactStore struct {
	root      string
	ttl       time.Duration
	maxBytes  int64
	mu        sync.RWMutex
	done      chan struct{}
	closeOnce sync.Once
}

func NewArtifactStore(root string, ttl time.Duration, maxBytes int64) (*ArtifactStore, error) {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if strings.TrimSpace(root) == "" {
		root = runtime.Subdir("artifacts")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact dir: %w", err)
	}
	store := &ArtifactStore{root: root, ttl: ttl, maxBytes: maxBytes, done: make(chan struct{})}
	go store.startCleanupLoop()
	return store, nil
}

// Close stops the background cleanup goroutine. Safe to call multiple times.
func (s *ArtifactStore) Close() error {
	s.closeOnce.Do(func() { close(s.done) })
	return nil
}

func (s *ArtifactStore) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func (s *ArtifactStore) SaveFile(srcPath, filename, contentType string, size int64) (*Artifact, error) {
	if s == nil {
		return nil, fmt.Errorf("artifact store is nil")
	}
	id, err := randomHexID("")
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(s.root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact workspace: %w", err)
	}
	name := sanitizeFilename(filename, srcPath)
	dstPath := filepath.Join(dir, name)
	if err := moveFile(srcPath, dstPath); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("move artifact: %w", err)
	}
	if size <= 0 {
		info, statErr := os.Stat(dstPath)
		if statErr != nil {
			_ = os.RemoveAll(dir)
			return nil, fmt.Errorf("stat artifact: %w", statErr)
		}
		size = info.Size()
	}
	artifact := &Artifact{ID: id, Path: dstPath, Filename: name, ContentType: strings.TrimSpace(contentType), ContentBytes: size, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(s.ttl)}
	s.mu.Lock()
	err = s.writeManifestLocked(artifact)
	s.mu.Unlock()
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}

	s.mu.Lock()
	err = s.cleanupForQuotaLocked()
	s.mu.Unlock()
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if _, err := os.Stat(artifact.Path); err != nil {
		return nil, fmt.Errorf("artifact exceeds storage quota")
	}
	return artifact, nil
}

func moveFile(srcPath, dstPath string) error {
	if err := os.Rename(srcPath, dstPath); err == nil {
		return nil
	} else if !isCrossDevice(err) {
		return err
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	tmpPath := dstPath + ".tmp"
	dst, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Remove(srcPath)
}

func (s *ArtifactStore) Get(id string) (*Artifact, error) {
	if s == nil {
		return nil, fmt.Errorf("artifact store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	artifact, err := s.readManifestLocked(id)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(artifact.Path); err != nil {
		_ = os.RemoveAll(filepath.Dir(artifact.Path))
		return nil, fmt.Errorf("artifact is missing")
	}
	return artifact, nil
}

func (s *ArtifactStore) Delete(id string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	artifact, err := s.readManifestLocked(id)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.RemoveAll(filepath.Dir(artifact.Path))
}

func (s *ArtifactStore) CleanupExpired() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanupExpiredLocked(time.Now())
}

func (s *ArtifactStore) Stats() ArtifactStats {
	if s == nil {
		return ArtifactStats{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	items, _ := s.listArtifactsLocked()
	stats := ArtifactStats{}
	for _, artifact := range items {
		stats.Entries++
		stats.BytesUsed += artifact.ContentBytes
	}
	return stats
}

func (s *ArtifactStore) writeManifestLocked(artifact *Artifact) error {
	path := filepath.Join(filepath.Dir(artifact.Path), "manifest.json")
	if err := writeJSONFile(path, artifact); err != nil {
		return fmt.Errorf("write artifact manifest: %w", err)
	}
	return nil
}

func (s *ArtifactStore) readManifestLocked(id string) (*Artifact, error) {
	data, err := os.ReadFile(filepath.Join(s.root, strings.TrimSpace(id), "manifest.json"))
	if err != nil {
		return nil, err
	}
	var artifact Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, fmt.Errorf("decode artifact manifest: %w", err)
	}
	if artifact.ID == "" {
		artifact.ID = strings.TrimSpace(id)
	}
	if time.Now().After(artifact.ExpiresAt) {
		_ = os.RemoveAll(filepath.Join(s.root, strings.TrimSpace(id)))
		return nil, os.ErrNotExist
	}
	return &artifact, nil
}

func (s *ArtifactStore) listArtifactsLocked() ([]Artifact, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	items := make([]Artifact, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		artifact, err := s.readManifestLocked(entry.Name())
		if err != nil {
			continue
		}
		items = append(items, *artifact)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items, nil
}

func (s *ArtifactStore) cleanupExpiredLocked(now time.Time) error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(s.root, entry.Name(), "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			_ = os.RemoveAll(filepath.Join(s.root, entry.Name()))
			continue
		}
		var artifact Artifact
		if err := json.Unmarshal(data, &artifact); err != nil || now.After(artifact.ExpiresAt) {
			_ = os.RemoveAll(filepath.Join(s.root, entry.Name()))
		}
	}
	return nil
}

func (s *ArtifactStore) cleanupForQuotaLocked() error {
	if s.maxBytes <= 0 {
		return nil
	}
	items, err := s.listArtifactsLocked()
	if err != nil {
		return nil
	}
	var used int64
	for _, item := range items {
		used += item.ContentBytes
	}
	for _, item := range items {
		if used <= s.maxBytes {
			break
		}
		used -= item.ContentBytes
		_ = os.RemoveAll(filepath.Dir(item.Path))
	}
	return nil
}

func (s *ArtifactStore) startCleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = s.CleanupExpired()
		case <-s.done:
			return
		}
	}
}

func sanitizeFilename(filename, fallbackPath string) string {
	return extract.SanitizeFilename(filename, filepath.Base(strings.TrimSpace(fallbackPath)))
}
