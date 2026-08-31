package health

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type redisChecker struct{ client *redis.Client }

// NewRedisChecker adapts a context-aware Redis client ping.
func NewRedisChecker(client *redis.Client) Checker { return redisChecker{client: client} }

// Name returns the stable readiness dependency key.
func (redisChecker) Name() string { return "redis" }

// Check performs one context-bounded Redis PING.
func (c redisChecker) Check(ctx context.Context) error { return c.client.Ping(ctx).Err() }
