package schema

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache provides persistent storage for schema version tracking.
// Redis was chosen over alternatives because:
// - Fast key-value lookups for quick schema comparisons
// - Built-in TTL support to prevent stale schema accumulation
// - Atomic operations to handle concurrent schema checks
// - Minimal memory footprint by storing only hashes
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis cache instance
func NewRedisCache(addr string) *RedisCache {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	return &RedisCache{client: client}
}

// Get retrieves a value from Redis
func (c *RedisCache) Get(key string) (string, error) {
	ctx := context.Background()
	return c.client.Get(ctx, key).Result()
}

// Set stores a value in Redis with 30-day expiration.
// The 30-day retention period balances:
// - Keeping enough history for trend analysis
// - Preventing unbounded storage growth
// - Maintaining reasonable backup sizes
func (c *RedisCache) Set(key string, value string) error {
	ctx := context.Background()
	return c.client.Set(ctx, key, value, 30*24*time.Hour).Err()
}

// Close closes the Redis connection
func (c *RedisCache) Close() error {
	return c.client.Close()
}
