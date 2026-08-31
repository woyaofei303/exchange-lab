package health

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type redisChecker struct{ client *redis.Client }

// NewRedisChecker 适配支持上下文的 Redis 客户端 PING。
func NewRedisChecker(client *redis.Client) Checker { return redisChecker{client: client} }

// Name 返回稳定的就绪检查依赖键。
func (redisChecker) Name() string { return "redis" }

// Check 执行一次受上下文限制的 Redis PING。
func (c redisChecker) Check(ctx context.Context) error { return c.client.Ping(ctx).Err() }
