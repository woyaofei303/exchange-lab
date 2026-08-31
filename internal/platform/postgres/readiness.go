package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SchemaChecker verifies both PostgreSQL connectivity and the migration level
// required by the running application.
type SchemaChecker struct {
	pool            *pgxpool.Pool
	requiredVersion int64
}

// NewSchemaChecker binds readiness to the minimum application schema version.
func NewSchemaChecker(pool *pgxpool.Pool, requiredVersion int64) *SchemaChecker {
	return &SchemaChecker{pool: pool, requiredVersion: requiredVersion}
}

// Name returns the stable readiness dependency key.
func (*SchemaChecker) Name() string { return "postgres" }

// Check verifies connectivity and that every required migration is applied.
func (c *SchemaChecker) Check(ctx context.Context) error {
	if c == nil || c.pool == nil || c.requiredVersion <= 0 {
		return fmt.Errorf("check postgres schema: invalid configuration")
	}
	var currentVersion int64
	if err := c.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(version_id) FILTER (WHERE is_applied), 0)
		FROM schema_migrations
	`).Scan(&currentVersion); err != nil {
		return fmt.Errorf("check postgres schema")
	}
	if currentVersion < c.requiredVersion {
		return fmt.Errorf("check postgres schema: migration required")
	}
	return nil
}
