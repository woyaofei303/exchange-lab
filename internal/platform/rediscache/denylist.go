package rediscache

import (
	"context"
	"errors"
	"fmt"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

// TokenDenylist 存储生命周期受限的撤销令牌哈希。
type TokenDenylist struct {
	client  redisclient.Cmdable
	timeout time.Duration
	maxTTL  time.Duration
}

// NewTokenDenylist 校验 Redis 访问和最大撤销生命周期。
func NewTokenDenylist(client redisclient.Cmdable, timeout, maxTTL time.Duration) (*TokenDenylist, error) {
	if client == nil || timeout <= 0 || maxTTL <= 0 {
		return nil, fmt.Errorf("create token denylist: invalid configuration")
	}
	return &TokenDenylist{client: client, timeout: timeout, maxTTL: maxTTL}, nil
}

// Deny 仅存储 tokenID 的哈希，并使其随令牌一起过期。
// 失败时返回错误，防止调用方误认为撤销成功。
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

// IsDenied 返回 Redis 错误，使身份验证在撤销状态不可用时能够失败关闭。
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
