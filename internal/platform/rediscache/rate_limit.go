package rediscache

import (
	"context"
	"fmt"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

var loginRateScript = redisclient.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('PTTL', KEYS[1])
return {count, ttl}
`)

// RateDecision 报告是否允许、剩余尝试次数和重试延迟。
type RateDecision struct {
	Allowed    bool
	Remaining  uint64
	RetryAfter time.Duration
}

// LoginRateLimiter 为每个规范化身份使用一个原子 Redis 脚本。
type LoginRateLimiter struct {
	client  redisclient.Cmdable
	timeout time.Duration
	limit   uint64
	window  time.Duration
}

// NewLoginRateLimiter 校验失败关闭行为和时间窗口边界。
func NewLoginRateLimiter(client redisclient.Cmdable, timeout time.Duration, limit uint64, window time.Duration) (*LoginRateLimiter, error) {
	if client == nil || timeout <= 0 || limit == 0 || window <= 0 {
		return nil, fmt.Errorf("create login rate limiter: invalid configuration")
	}
	return &LoginRateLimiter{client: client, timeout: timeout, limit: limit, window: window}, nil
}

// Allow 原子统计一个规范化身份。Redis 错误会向上返回，
// 使后续身份验证层失败关闭，而不是绕过限制。
func (l *LoginRateLimiter) Allow(ctx context.Context, identity string) (RateDecision, error) {
	key, err := privateKey("security:login", identity)
	if err != nil {
		return RateDecision{}, err
	}
	operationContext, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()
	result, err := loginRateScript.Run(operationContext, l.client, []string{key}, l.window.Milliseconds()).Result()
	if err != nil {
		return RateDecision{}, fmt.Errorf("apply login rate limit: %w", err)
	}
	values, ok := result.([]any)
	if !ok || len(values) != 2 {
		return RateDecision{}, fmt.Errorf("apply login rate limit: invalid Redis response")
	}
	count, ok := values[0].(int64)
	if !ok || count <= 0 {
		return RateDecision{}, fmt.Errorf("apply login rate limit: invalid count")
	}
	ttlMilliseconds, ok := values[1].(int64)
	if !ok || ttlMilliseconds <= 0 {
		return RateDecision{}, fmt.Errorf("apply login rate limit: invalid TTL")
	}
	decision := RateDecision{RetryAfter: time.Duration(ttlMilliseconds) * time.Millisecond}
	decision.Allowed = uint64(count) <= l.limit
	if uint64(count) < l.limit {
		decision.Remaining = l.limit - uint64(count)
	}
	return decision, nil
}
