package cache

import (
	"sync"
	"testing"
	"time"
)

func TestSetAndGet(t *testing.T) {
	c := NewTTLCache()
	c.Set("key1", "value1", time.Minute)

	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected to find key1")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %v", val)
	}
}

func TestGetExpired(t *testing.T) {
	c := NewTTLCache()
	c.Set("key1", "value1", 10*time.Millisecond)

	time.Sleep(20 * time.Millisecond)

	val, ok := c.Get("key1")
	if ok {
		t.Error("expected key to be expired")
	}
	if val != nil {
		t.Errorf("expected nil, got %v", val)
	}
}

func TestGetNotFound(t *testing.T) {
	c := NewTTLCache()

	val, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected key to not be found")
	}
	if val != nil {
		t.Errorf("expected nil, got %v", val)
	}
}

func TestDelete(t *testing.T) {
	c := NewTTLCache()
	c.Set("key1", "value1", time.Minute)

	c.Delete("key1")

	val, ok := c.Get("key1")
	if ok {
		t.Error("expected key to be deleted")
	}
	if val != nil {
		t.Errorf("expected nil, got %v", val)
	}
}

func TestCleanup(t *testing.T) {
	c := NewTTLCache()

	c.Set("valid", "value1", time.Minute)
	c.Set("expired", "value2", 10*time.Millisecond)

	time.Sleep(20 * time.Millisecond)

	c.Cleanup()

	val, ok := c.Get("valid")
	if !ok {
		t.Error("expected valid key to remain")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %v", val)
	}

	_, ok = c.Get("expired")
	if ok {
		t.Error("expected expired key to be removed after cleanup")
	}
}

func TestOverwrite(t *testing.T) {
	c := NewTTLCache()
	c.Set("key1", "value1", time.Minute)
	c.Set("key1", "value2", time.Minute)

	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected to find key1")
	}
	if val != "value2" {
		t.Errorf("expected value2, got %v", val)
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := NewTTLCache()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "key"
			c.Set(key, n, time.Minute)
		}(i)
	}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Get("key")
		}()
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Delete("key")
		}()
	}

	wg.Wait()
}

func TestPlatformTTLConfig_UsesPlatformSpecificTTL(t *testing.T) {
	cfg := NewPlatformTTLConfig(1*time.Minute, map[string]time.Duration{
		"twitter": 2 * time.Minute,
	})

	if got := cfg.TTLForPlatform("twitter"); got != 2*time.Minute {
		t.Fatalf("expected twitter ttl=2m, got %s", got)
	}

	if got := cfg.TTLForPlatform("TWITTER"); got != 2*time.Minute {
		t.Fatalf("expected case-insensitive platform match, got %s", got)
	}
}

func TestPlatformTTLConfig_FallsBackToDefault(t *testing.T) {
	cfg := NewPlatformTTLConfig(1*time.Minute, map[string]time.Duration{
		"twitter": 2 * time.Minute,
	})

	if got := cfg.TTLForPlatform("unknown"); got != 1*time.Minute {
		t.Fatalf("expected fallback ttl=1m, got %s", got)
	}
}

func TestTTLCache_MaxEntriesEviction(t *testing.T) {
	c := NewTTLCacheWithMaxEntries(2)
	c.Set("a", "value-a", time.Minute)
	c.Set("b", "value-b", time.Minute)
	c.Set("c", "value-c", time.Minute)

	if _, ok := c.Get("c"); !ok {
		t.Fatalf("expected newest key c to exist")
	}

	if _, ok := c.Get("b"); !ok {
		t.Fatalf("expected key b to exist")
	}

	if _, ok := c.Get("a"); ok {
		t.Fatalf("expected lexicographically first key a to be evicted deterministically")
	}
}

func TestTTLCache_EvictionSkipsExpiredEntries(t *testing.T) {
	c := NewTTLCacheWithMaxEntries(2)
	c.Set("expired", "value-expired", 10*time.Millisecond)
	c.Set("alive", "value-alive", time.Minute)
	time.Sleep(20 * time.Millisecond)

	c.Set("new", "value-new", time.Minute)

	if _, ok := c.Get("alive"); !ok {
		t.Fatalf("expected alive key to remain")
	}
	if _, ok := c.Get("new"); !ok {
		t.Fatalf("expected new key to exist")
	}
	if _, ok := c.Get("expired"); ok {
		t.Fatalf("expected expired key removed")
	}
}
