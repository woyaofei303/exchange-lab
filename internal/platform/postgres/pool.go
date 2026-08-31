// Package postgres 管理 SQL 持久化、事务和幂等边界。
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig 定义有界的 PostgreSQL 连接池行为。
type PoolConfig struct {
	MinConns       int32
	MaxConns       int32
	ConnectTimeout time.Duration
}

// OpenPool 在不修改 PostgreSQL DSN 的情况下解析它，并返回有界的 pgx 连接池。
// 连通性仍由就绪检查负责，避免临时数据库故障使进程启动陷入崩溃循环。
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
