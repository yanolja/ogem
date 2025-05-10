package schema

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(addr string) *RedisCache {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})
	return &RedisCache{client: client}
}

func (c *RedisCache) Get(key string) (string, error) {
	ctx := context.Background()
	return c.client.Get(ctx, key).Result()
}

func (c *RedisCache) Set(key string, value string) error {
	ctx := context.Background()
	return c.client.Set(ctx, key, value, 30*24*time.Hour).Err()
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}
