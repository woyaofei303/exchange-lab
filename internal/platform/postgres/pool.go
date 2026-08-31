// Package postgres owns SQL persistence, transaction and idempotency boundaries.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig defines bounded PostgreSQL connection-pool behavior.
type PoolConfig struct {
	MinConns       int32
	MaxConns       int32
	ConnectTimeout time.Duration
}

// OpenPool parses a PostgreSQL DSN without changing it and returns a bounded
// pgx pool. Connectivity remains a readiness concern so a temporary database
// outage does not turn process startup into a crash loop.
func OpenPool(ctx context.Context, dsn string, settings PoolConfig) (*pgxpool.Pool, error) {
	if settings.MinConns < 0 || settings.MaxConns <= 0 || settings.MinConns > settings.MaxConns {
		return nil, fmt.Errorf("open postgres pool: invalid connection limits")
	}
	if settings.ConnectTimeout <= 0 {
		return nil, fmt.Errorf("open postgres pool: invalid connect timeout")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool: invalid configuration")
	}
	config.MinConns = settings.MinConns
	config.MaxConns = settings.MaxConns
	config.ConnConfig.ConnectTimeout = settings.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open postgres pool")
	}
	return pool, nil
}
