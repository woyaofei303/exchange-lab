package health

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresChecker struct{ pool *pgxpool.Pool }

// NewPostgresChecker adapts a context-aware PostgreSQL pool ping.
func NewPostgresChecker(pool *pgxpool.Pool) Checker { return postgresChecker{pool: pool} }

// Name returns the stable readiness dependency key.
func (postgresChecker) Name() string { return "postgres" }

// Check performs one context-bounded PostgreSQL ping.
func (c postgresChecker) Check(ctx context.Context) error { return c.pool.Ping(ctx) }
