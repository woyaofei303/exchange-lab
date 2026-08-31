package health

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresChecker struct{ pool *pgxpool.Pool }

// NewPostgresChecker 适配支持上下文的 PostgreSQL 连接池探测。
func NewPostgresChecker(pool *pgxpool.Pool) Checker { return postgresChecker{pool: pool} }

// Name 返回稳定的就绪检查依赖键。
func (postgresChecker) Name() string { return "postgres" }

// Check 执行一次受上下文限制的 PostgreSQL 探测。
func (c postgresChecker) Check(ctx context.Context) error { return c.pool.Ping(ctx) }
