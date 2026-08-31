// Command risk-worker evaluates liquidations and conditional orders.
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
		logger.Error("risk worker stopped")
		os.Exit(1)
	}
}
func run(ctx context.Context, logger *slog.Logger) error {
	dsn := env("POSTGRES_DSN", "postgres://exchange:exchange_dev_only@postgres:5432/exchange?sslmode=disable")
	addr := env("RISK_HTTP_ADDR", ":8094")
	pool, err := store.OpenPool(ctx, dsn, store.PoolConfig{MinConns: 1, MaxConns: 4, ConnectTimeout: 5 * time.Second})
	if err != nil {
		return err
	}
	defer pool.Close()
	repo := store.NewRiskRepository(pool, 3*time.Second)
	metrics := observability.New("risk-worker")
	scan := func(ctx context.Context) (map[string]int, error) {
		jobs, err := repo.ScanLiquidations(ctx, time.Now())
		if err != nil {
			return nil, err
		}
		completed, err := repo.ProcessLiquidations(ctx, time.Now())
		if err != nil {
			return nil, err
		}
		conditionals, err := repo.TriggerConditionals(ctx, time.Now())
		if err != nil {
			return nil, err
		}
		metrics.Liquidations.Add(float64(completed))
		metrics.LiquidationBacklog.Set(float64(jobs - completed))
		return map[string]int{"liquidation_jobs": jobs, "liquidations_completed": completed, "conditional_orders": conditionals}, nil
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
	mux.HandleFunc("POST /scan", func(w http.ResponseWriter, r *http.Request) {
		result, err := scan(r.Context())
		if err != nil {
			http.Error(w, "scan failed", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()
	ticker := time.NewTicker(time.Second)
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
		case <-ticker.C:
			if _, err := scan(ctx); err != nil {
				logger.Warn("risk scan failed")
			}
		}
	}
}
func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
