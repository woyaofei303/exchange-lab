package rediscache

import (
	"context"
	"errors"
	"fmt"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

// TokenDenylist stores hashed revocations with bounded lifetimes.
type TokenDenylist struct {
	client  redisclient.Cmdable
	timeout time.Duration
	maxTTL  time.Duration
}

// NewTokenDenylist validates Redis access and maximum revocation lifetime.
func NewTokenDenylist(client redisclient.Cmdable, timeout, maxTTL time.Duration) (*TokenDenylist, error) {
	if client == nil || timeout <= 0 || maxTTL <= 0 {
		return nil, fmt.Errorf("create token denylist: invalid configuration")
	}
	return &TokenDenylist{client: client, timeout: timeout, maxTTL: maxTTL}, nil
}

// Deny stores only a hash of tokenID and expires it with the token. A failure
// is returned so callers do not pretend revocation succeeded.
func (d *TokenDenylist) Deny(ctx context.Context, tokenID string, expiresAt time.Time) error {
	key, err := privateKey("security:denylist", tokenID)
	if err != nil {
		return err
	}
	ttl := time.Until(expiresAt)
	if ttl <= 0 || ttl > d.maxTTL {
		return fmt.Errorf("deny token: invalid expiry")
	}
	operationContext, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	if err := d.client.Set(operationContext, key, "1", ttl).Err(); err != nil {
		return fmt.Errorf("deny token: %w", err)
	}
	return nil
}

// IsDenied returns Redis errors so authentication can fail closed when
// revocation state is unavailable.
func (d *TokenDenylist) IsDenied(ctx context.Context, tokenID string) (bool, error) {
	key, err := privateKey("security:denylist", tokenID)
	if err != nil {
		return false, err
	}
	operationContext, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	_, err = d.client.Get(operationContext, key).Result()
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, redisclient.Nil):
		return false, nil
	default:
		return false, fmt.Errorf("check token denylist: %w", err)
	}
}
