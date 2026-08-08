package health

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresChecker struct{ pool *pgxpool.Pool }

func NewPostgresChecker(pool *pgxpool.Pool) Checker { return postgresChecker{pool: pool} }

func (postgresChecker) Name() string { return "postgres" }

func (c postgresChecker) Check(ctx context.Context) error { return c.pool.Ping(ctx) }
