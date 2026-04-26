package api

import (
	"sync"
	"time"
)

type cachedValue[T any] struct {
	mu        sync.Mutex
	expiresAt time.Time
	value     T
}

func (c *cachedValue[T]) get(ttl time.Duration, load func() T) T {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if ttl > 0 && now.Before(c.expiresAt) {
		return c.value
	}
	value := load()
	c.value = value
	if ttl > 0 {
		c.expiresAt = now.Add(ttl)
	}
	return value
}
