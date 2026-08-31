package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	"github.com/woyaofei303/exchange-lab/migrations"
)

// Migrator applies embedded, versioned PostgreSQL schema changes.
type Migrator struct {
	provider *goose.Provider
}

// NewMigrator configures goose to store migration history in
// schema_migrations. The caller retains ownership of db.
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

// Up applies every pending migration. Calling Up when the schema is current is
// a no-op.
func (m *Migrator) Up(ctx context.Context) error {
	if _, err := m.provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate database up: %w", err)
	}
	return nil
}

// Down rolls back exactly one migration.
func (m *Migrator) Down(ctx context.Context) error {
	if _, err := m.provider.Down(ctx); err != nil {
		return fmt.Errorf("migrate database down: %w", err)
	}
	return nil
}

// Version returns the latest applied migration version.
func (m *Migrator) Version(ctx context.Context) (int64, error) {
	version, err := m.provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("read database migration version: %w", err)
	}
	return version, nil
}
