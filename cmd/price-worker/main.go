// Command price-worker derives index and mark prices from normalized tickers.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/woyaofei303/exchange-lab/internal/contractprice"
	"github.com/woyaofei303/exchange-lab/internal/observability"
	"github.com/woyaofei303/exchange-lab/internal/platform/natspublisher"
	store "github.com/woyaofei303/exchange-lab/internal/platform/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("price worker stopped")
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	dsn := env("POSTGRES_DSN", "postgres://exchange:exchange_dev_only@postgres:5432/exchange?sslmode=disable")
	natsURL := env("NATS_URL", "nats://nats:4222")
	addr := env("PRICE_HTTP_ADDR", ":8093")
	pool, err := store.OpenPool(ctx, dsn, store.PoolConfig{MinConns: 1, MaxConns: 4, ConnectTimeout: 5 * time.Second})
	if err != nil {
		return errors.New("create price database client")
	}
	defer pool.Close()
	nc, err := nats.Connect(natsURL, nats.Name("exchange-lab-price-worker"), nats.Timeout(2*time.Second), nats.MaxReconnects(-1))
	if err != nil {
		return errors.New("connect price NATS")
	}
	defer nc.Close()
	publisher, _ := natspublisher.New(nc)
	worker, _ := contractprice.NewWorker(store.NewPriceRepository(pool, 2*time.Second), publisher, time.Now)
	metrics := observability.New("price-worker")
	sub, err := nc.SubscribeSync("market.*.ticker")
	if err != nil {
		return errors.New("subscribe market ticker")
	}
	defer sub.Unsubscribe()
	if err := nc.FlushTimeout(2 * time.Second); err != nil {
		return errors.New("activate price subscription")
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		checkCtx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if pool.Ping(checkCtx) != nil || !nc.IsConnected() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	errs := make(chan error, 2)
	go func() { errs <- server.ListenAndServe() }()
	go func() {
		for {
			msg, readErr := sub.NextMsgWithContext(ctx)
			if readErr != nil {
				errs <- readErr
				return
			}
			if handleErr := worker.Handle(ctx, msg.Data); handleErr != nil {
				logger.Error("process risk price failed")
				metrics.PriceFresh.Set(0)
			} else {
				metrics.PriceFresh.Set(1)
			}
		}
	}()
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(closeCtx)
		return ctx.Err()
	}
}

func env(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok && value != "" {
		return value
	}
	return fallback
}
