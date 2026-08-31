package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"exchange-lab/migrations"
	"github.com/pressly/goose/v3"
)

// Migrator 应用内嵌且带版本的 PostgreSQL 数据库结构变更。
type Migrator struct {
	provider *goose.Provider
}

// NewMigrator 配置 goose 将迁移历史存入 schema_migrations；db 仍由调用方管理。
func NewMigrator(db *sql.DB) (*Migrator, error) {
	if db == nil {
		return nil, fmt.Errorf("create migrator: database is required")
	}
	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		migrations.FS,
		goose.WithTableName("schema_migrations"),
	)
	if err != nil {
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	return &Migrator{provider: provider}, nil
}

// Up 应用所有待处理迁移；数据库结构已是最新时不执行任何操作。
func (m *Migrator) Up(ctx context.Context) error {
	if _, err := m.provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate database up: %w", err)
	}
	return nil
}

// Down 精确回滚一个迁移。
func (m *Migrator) Down(ctx context.Context) error {
	if _, err := m.provider.Down(ctx); err != nil {
		return fmt.Errorf("migrate database down: %w", err)
	}
	return nil
}

// Version 返回最近应用的迁移版本。
func (m *Migrator) Version(ctx context.Context) (int64, error) {
	version, err := m.provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("read database migration version: %w", err)
	}
	return version, nil
}
