package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SchemaChecker 同时检查 PostgreSQL 连通性和运行中应用所需的迁移版本。
type SchemaChecker struct {
	pool            *pgxpool.Pool
	requiredVersion int64
}

// NewSchemaChecker 将就绪状态绑定到应用所需的最低数据库结构版本。
func NewSchemaChecker(pool *pgxpool.Pool, requiredVersion int64) *SchemaChecker {
	return &SchemaChecker{pool: pool, requiredVersion: requiredVersion}
}

// Name 返回稳定的就绪检查依赖键。
func (*SchemaChecker) Name() string { return "postgres" }

// Check 验证连通性，并确认所有必需迁移均已应用。
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
