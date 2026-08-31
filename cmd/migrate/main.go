// Command migrate 应用、回滚或报告内嵌的数据库结构迁移。
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"exchange-lab/internal/config"
	store "exchange-lab/internal/platform/postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type migrationDirection string

const (
	directionUp      migrationDirection = "up"
	directionDown    migrationDirection = "down"
	directionVersion migrationDirection = "version"
)

func main() {
	var directionText string
	var operationTimeout time.Duration
	flag.StringVar(&directionText, "direction", "up", "migration operation: up, down, or version")
	flag.DurationVar(&operationTimeout, "timeout", 30*time.Second, "maximum migration operation duration")
	flag.Parse()
	if flag.NArg() != 0 || operationTimeout <= 0 {
		fmt.Fprintln(os.Stderr, "migration failed: invalid arguments")
		os.Exit(2)
	}
	direction, err := parseDirection(directionText)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := run(context.Background(), direction, operationTimeout, os.LookupEnv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseDirection(value string) (migrationDirection, error) {
	direction := migrationDirection(value)
	switch direction {
	case directionUp, directionDown, directionVersion:
		return direction, nil
	default:
		return "", errors.New("migration failed: direction must be up, down, or version")
	}
}

func run(
	parent context.Context,
	direction migrationDirection,
	operationTimeout time.Duration,
	lookup func(string) (string, bool),
	output io.Writer,
) error {
	databaseConfig, err := config.LoadDatabase(lookup)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}
	db, err := sql.Open("pgx", databaseConfig.PostgresDSN)
	if err != nil {
		return errors.New("migration failed: open database")
	}
	defer db.Close()
	db.SetMaxOpenConns(int(databaseConfig.MaxConns))
	db.SetMaxIdleConns(int(databaseConfig.MinConns))

	pingContext, cancelPing := context.WithTimeout(parent, databaseConfig.Timeout)
	if err := db.PingContext(pingContext); err != nil {
		cancelPing()
		return errors.New("migration failed: connect database")
	}
	cancelPing()

	migrator, err := store.NewMigrator(db)
	if err != nil {
		return errors.New("migration failed: initialize migrator")
	}
	operationContext, cancelOperation := context.WithTimeout(parent, operationTimeout)
	defer cancelOperation()
	switch direction {
	case directionUp:
		if err := migrator.Up(operationContext); err != nil {
			return errors.New("migration failed: apply up")
		}
	case directionDown:
		if err := migrator.Down(operationContext); err != nil {
			return errors.New("migration failed: apply down")
		}
	case directionVersion:
		version, err := migrator.Version(operationContext)
		if err != nil {
			return errors.New("migration failed: read version")
		}
		if _, err := fmt.Fprintln(output, version); err != nil {
			return errors.New("migration failed: write version")
		}
	default:
		return errors.New("migration failed: unsupported direction")
	}
	return nil
}
