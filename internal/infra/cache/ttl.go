package cache

import (
	"sort"
	"sync"
	"time"
)

type entry struct {
	value     any
	expiresAt time.Time
}

type TTLCache struct {
	mu         sync.RWMutex
	items      map[string]entry
	maxEntries int
}

func NewTTLCache() *TTLCache {
	return NewTTLCacheWithMaxEntries(10000)
}

func NewTTLCacheWithMaxEntries(maxEntries int) *TTLCache {
	if maxEntries < 1 {
		maxEntries = 10000
	}
	return &TTLCache{
		items:      make(map[string]entry),
		maxEntries: maxEntries,
	}
}

func (c *TTLCache) Get(key string) (any, bool) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}

	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		c.Delete(key)
		return nil, false
	}

	return item.value, true
}

func (c *TTLCache) Set(key string, value any, ttl time.Duration) {
	if ttl <= 0 {
		c.Delete(key)
		return
	}

	c.mu.Lock()
	now := time.Now()
	c.cleanupExpiredLocked(now)
	if _, exists := c.items[key]; !exists && len(c.items) >= c.maxEntries {
		c.evictOneLocked()
	}
	c.items[key] = entry{
		value:     value,
		expiresAt: now.Add(ttl),
	}
	c.mu.Unlock()
}

func (c *TTLCache) Delete(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

func (c *TTLCache) Cleanup() {
	c.mu.Lock()
	c.cleanupExpiredLocked(time.Now())
	c.mu.Unlock()
}

func (c *TTLCache) cleanupExpiredLocked(now time.Time) {
	for key, item := range c.items {
		if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
			delete(c.items, key)
		}
	}
}

func (c *TTLCache) evictOneLocked() {
	if len(c.items) == 0 {
		return
	}

	keys := make([]string, 0, len(c.items))
	for key := range c.items {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		left := c.items[keys[i]].expiresAt
		right := c.items[keys[j]].expiresAt

		if left.Equal(right) {
			return keys[i] < keys[j]
		}
		if left.IsZero() {
			return false
		}
		if right.IsZero() {
			return true
		}
		return left.Before(right)
	})

	delete(c.items, keys[0])
}
