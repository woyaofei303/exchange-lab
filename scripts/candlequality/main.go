// Command candlequality checks persisted candle continuity and invariants.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("candle quality: OHLC, volume, uniqueness, and continuity verified")
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dsn := environmentOr("POSTGRES_DSN", "postgres://exchange:exchange_dev_only@127.0.0.1:5432/exchange?sslmode=disable")
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return errors.New("candle quality: create database client")
	}
	defer pool.Close()
	checks := []struct {
		name  string
		query string
	}{
		{name: "OHLC and volume", query: `
			SELECT count(*) FROM candles
			WHERE open_atoms <= 0 OR high_atoms < open_atoms OR high_atoms < close_atoms
			   OR low_atoms > open_atoms OR low_atoms > close_atoms
			   OR volume_atoms < 0 OR trade_count < 0
		`},
		{name: "event uniqueness", query: `
			SELECT count(*) FROM (
				SELECT stream_id, trade_id FROM market_trades
				GROUP BY stream_id, trade_id HAVING count(*) > 1
			) duplicates
		`},
		{name: "final candle continuity", query: `
			SELECT count(*) FROM (
				SELECT open_time, interval_seconds,
				       lag(open_time) OVER (PARTITION BY symbol, interval_seconds ORDER BY open_time) AS previous_time
				FROM candles WHERE is_final = true
			) ordered
			WHERE previous_time IS NOT NULL
			  AND open_time <> previous_time + make_interval(secs => interval_seconds)
		`},
	}
	for _, check := range checks {
		var violations int64
		if err := pool.QueryRow(ctx, check.query).Scan(&violations); err != nil {
			return fmt.Errorf("candle quality: %s check failed", check.name)
		}
		if violations != 0 {
			return fmt.Errorf("candle quality: %s violations=%d", check.name, violations)
		}
	}
	return nil
}

func environmentOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
