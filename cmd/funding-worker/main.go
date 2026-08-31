// Command funding-worker periodically settles contract funding payments.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/woyaofei303/exchange-lab/internal/observability"
	store "github.com/woyaofei303/exchange-lab/internal/platform/postgres"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("funding worker stopped")
		os.Exit(1)
	}
}
func run(ctx context.Context, logger *slog.Logger) error {
	dsn := env("POSTGRES_DSN", "postgres://exchange:exchange_dev_only@postgres:5432/exchange?sslmode=disable")
	addr := env("FUNDING_HTTP_ADDR", ":8095")
	interval := 8 * time.Hour
	if value := os.Getenv("FUNDING_INTERVAL"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed < time.Minute {
			return errors.New("invalid funding interval")
		}
		interval = parsed
	}
	pool, err := store.OpenPool(ctx, dsn, store.PoolConfig{MinConns: 1, MaxConns: 4, ConnectTimeout: 5 * time.Second})
	if err != nil {
		return err
	}
	defer pool.Close()
	repo := store.NewRiskRepository(pool, 5*time.Second)
	metrics := observability.New("funding-worker")
	runFunding := func(ctx context.Context, at time.Time) (map[string]any, error) {
		id, count, err := repo.RunFunding(ctx, "BTCUSDT-PERP", at.UTC().Truncate(interval))
		if err == nil {
			metrics.FundingLag.Set(time.Since(at.UTC()).Seconds())
		}
		return map[string]any{"funding_rate_id": id, "settlements": count, "funding_time": at.UTC().Truncate(interval)}, err
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		check, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if pool.Ping(check) != nil {
			http.Error(w, "not ready", 503)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	mux.HandleFunc("POST /run", func(w http.ResponseWriter, r *http.Request) {
		at, err := requestedFundingTime(r, time.Now())
		if err != nil {
			http.Error(w, "invalid funding time", http.StatusBadRequest)
			return
		}
		result, err := runFunding(r.Context(), at)
		if err != nil {
			http.Error(w, "funding failed", 409)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = server.Shutdown(closeCtx)
			return ctx.Err()
		case err := <-serverErr:
			return err
		case at := <-ticker.C:
			if _, err := runFunding(ctx, at); err != nil {
				logger.Warn("funding settlement failed")
			}
		}
	}
}

func requestedFundingTime(request *http.Request, now time.Time) (time.Time, error) {
	value := request.URL.Query().Get("at")
	if value == "" {
		return now.UTC(), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New("invalid funding time")
	}
	return parsed.UTC(), nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
