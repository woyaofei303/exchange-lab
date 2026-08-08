package health

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type redisChecker struct{ client *redis.Client }

func NewRedisChecker(client *redis.Client) Checker { return redisChecker{client: client} }

func (redisChecker) Name() string { return "redis" }

func (c redisChecker) Check(ctx context.Context) error { return c.client.Ping(ctx).Err() }
