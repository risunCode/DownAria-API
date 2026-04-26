package extract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	runtime "downaria-api/internal/runtime"
)

type Cache interface {
	Get(key string) (*Result, bool, error)
	Set(key string, value *Result) error
}

type CacheStats struct {
	Entries   int
	BytesUsed int64
}

type FileCache struct {
	dir         string
	ttl         time.Duration
	cleanupTTL  time.Duration
	mu          sync.Mutex
	lastCleanup time.Time
}

type cacheEntry struct {
	ExpiresAt time.Time `json:"expires_at"`
	Result    *Result   `json:"result"`
}

var ephemeralQueryHints = []string{"token", "sig", "signature", "expires", "expire", "exp", "auth", "acctoken", "download_filename"}

func NewFileCache(dir string, ttl time.Duration) (*FileCache, error) {
	if ttl <= 0 {
		return nil, fmt.Errorf("cache ttl must be greater than zero")
	}
	if strings.TrimSpace(dir) == "" {
		dir = runtime.Subdir("cache")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	cache := &FileCache{dir: dir, ttl: ttl, cleanupTTL: time.Minute}
	go cache.startCleanupLoop()
	return cache, nil
}

func CacheKey(rawURL string, opts ExtractOptions) string {
	hash := sha256.Sum256([]byte(rawURL + "\n" + opts.CookieHeader + "\n" + fmt.Sprintf("%t", opts.UseAuth)))
	return hex.EncodeToString(hash[:])
}

func (c *FileCache) Get(key string) (*Result, bool, error) {
	if c == nil {
		return nil, false, nil
	}
	path := c.entryPath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read cache entry: %w", err)
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		_ = os.Remove(path)
		return nil, false, fmt.Errorf("decode cache entry: %w", err)
	}
	if time.Now().After(entry.ExpiresAt) || entry.Result == nil {
		_ = os.Remove(path)
		return nil, false, nil
	}
	return entry.Result, true, nil
}

func (c *FileCache) Set(key string, value *Result) error {
	if c == nil || value == nil {
		return nil
	}
	entry := cacheEntry{ExpiresAt: time.Now().Add(cacheTTLForResult(c.ttl, value)), Result: value}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode cache entry: %w", err)
	}
	tmp, err := os.CreateTemp(c.dir, key+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp cache entry: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write cache entry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close cache entry: %w", err)
	}
	if err := os.Rename(tmp.Name(), c.entryPath(key)); err != nil {
		return fmt.Errorf("commit cache entry: %w", err)
	}
	return nil
}

func (c *FileCache) entryPath(key string) string {
	return filepath.Join(c.dir, key+".json")
}

func (c *FileCache) cleanupExpired() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.lastCleanup) < c.cleanupTTL {
		return nil
	}
	c.lastCleanup = time.Now()
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(c.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var decoded cacheEntry
		if err := json.Unmarshal(data, &decoded); err != nil || decoded.Result == nil || now.After(decoded.ExpiresAt) {
			_ = os.Remove(path)
		}
	}
	return nil
}

func (c *FileCache) startCleanupLoop() {
	ticker := time.NewTicker(c.cleanupTTL)
	defer ticker.Stop()
	for range ticker.C {
		_ = c.cleanupExpired()
	}
}

func (c *FileCache) Stats() CacheStats {
	if c == nil {
		return CacheStats{}
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return CacheStats{}
	}
	stats := CacheStats{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		stats.Entries++
		stats.BytesUsed += info.Size()
	}
	return stats
}

func cacheTTLForResult(defaultTTL time.Duration, result *Result) time.Duration {
	if defaultTTL <= 0 {
		defaultTTL = time.Minute
	}
	if result == nil || !resultContainsEphemeralURLs(result) {
		return defaultTTL
	}
	ephemeralTTL := 30 * time.Second
	if defaultTTL < ephemeralTTL {
		return defaultTTL
	}
	return ephemeralTTL
}

func resultContainsEphemeralURLs(result *Result) bool {
	if result == nil {
		return false
	}
	for _, item := range result.Media {
		for _, source := range item.Sources {
			if mediaURLLooksEphemeral(source.URL) {
				return true
			}
		}
	}
	return false
}

func mediaURLLooksEphemeral(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	query := parsed.Query()
	for _, key := range ephemeralQueryHints {
		if query.Get(key) != "" {
			return true
		}
	}
	return false
}
