package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// anthropicProxyCache is the Redis-backed implementation of
// service.AnthropicProxyCache. A cache miss (redis.Nil) is reported as
// ("", nil) so callers treat it as a miss, not an error.
type anthropicProxyCache struct {
	rdb *redis.Client
}

// NewAnthropicProxyCache creates a Redis-backed AnthropicProxyCache.
func NewAnthropicProxyCache(rdb *redis.Client) service.AnthropicProxyCache {
	return &anthropicProxyCache{rdb: rdb}
}

func (c *anthropicProxyCache) Get(ctx context.Context, key string) (string, error) {
	v, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return v, err
}

func (c *anthropicProxyCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}
