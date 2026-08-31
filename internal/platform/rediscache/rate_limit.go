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

// RateDecision reports allowance, remaining attempts and retry delay.
type RateDecision struct {
	Allowed    bool
	Remaining  uint64
	RetryAfter time.Duration
}

// LoginRateLimiter uses one atomic Redis script per normalized identity.
type LoginRateLimiter struct {
	client  redisclient.Cmdable
	timeout time.Duration
	limit   uint64
	window  time.Duration
}

// NewLoginRateLimiter validates fail-closed operation and window bounds.
func NewLoginRateLimiter(client redisclient.Cmdable, timeout time.Duration, limit uint64, window time.Duration) (*LoginRateLimiter, error) {
	if client == nil || timeout <= 0 || limit == 0 || window <= 0 {
		return nil, fmt.Errorf("create login rate limiter: invalid configuration")
	}
	return &LoginRateLimiter{client: client, timeout: timeout, limit: limit, window: window}, nil
}

// Allow atomically counts a normalized identity. Redis errors are returned so
// the future authentication layer can fail closed instead of bypassing limits.
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
