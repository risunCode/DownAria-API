// Package redis provides Redis-backed cache implementation.
// This is a stub for future distributed caching support.
package redis

import (
	"context"
	"time"
)

// Config holds Redis connection configuration
type Config struct {
	Addr     string
	Password string
	DB       int
	Prefix   string
}

// Cache implements distributed caching using Redis
type Cache struct {
	config Config
	// client *redis.Client // Uncomment when redis client is added
}

// NewCache creates a new Redis cache instance
func NewCache(config Config) *Cache {
	return &Cache{config: config}
}

// Get retrieves a value from cache
func (c *Cache) Get(ctx context.Context, key string, dest interface{}) error {
	// TODO: Implement when redis client is available
	// val, err := c.client.Get(ctx, c.key(key)).Result()
	// if err == redis.Nil {
	//     return cache.ErrNotFound
	// }
	// if err != nil {
	//     return err
	// }
	// return json.Unmarshal([]byte(val), dest)
	return nil
}

// Set stores a value in cache with TTL
func (c *Cache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// TODO: Implement when redis client is available
	// data, err := json.Marshal(value)
	// if err != nil {
	//     return err
	// }
	// return c.client.Set(ctx, c.key(key), data, ttl).Err()
	return nil
}

// Delete removes a key from cache
func (c *Cache) Delete(ctx context.Context, key string) error {
	// TODO: Implement when redis client is available
	// return c.client.Del(ctx, c.key(key)).Err()
	return nil
}

// key prepends prefix to the key
func (c *Cache) key(k string) string {
	if c.config.Prefix != "" {
		return c.config.Prefix + ":" + k
	}
	return k
}
