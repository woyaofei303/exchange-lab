// Command candle-worker aggregates normalized trades into durable candles.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/woyaofei303/exchange-lab/internal/candle"
	"github.com/woyaofei303/exchange-lab/internal/config"
	"github.com/woyaofei303/exchange-lab/internal/platform/natspublisher"
	store "github.com/woyaofei303/exchange-lab/internal/platform/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.LoadCandleWorker(os.LookupEnv)
	if err != nil {
		logger.Error("invalid candle worker configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("candle worker stopped")
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.CandleWorkerConfig, logger *slog.Logger) error {
	pool, err := store.OpenPool(ctx, cfg.Database.PostgresDSN, store.PoolConfig{
		MinConns: cfg.Database.MinConns, MaxConns: cfg.Database.MaxConns, ConnectTimeout: cfg.Database.Timeout,
	})
	if err != nil {
		return errors.New("create candle database client")
	}
	defer pool.Close()
	natsConnection, err := nats.Connect(cfg.NATSURL,
		nats.Name("exchange-lab-candle-worker"), nats.Timeout(cfg.OperationTimeout),
		nats.MaxReconnects(-1), nats.ReconnectWait(500*time.Millisecond),
	)
	if err != nil {
		return errors.New("connect candle NATS")
	}
	defer natsConnection.Close()
	publisher, err := natspublisher.New(natsConnection)
	if err != nil {
		return errors.New("create candle publisher")
	}
	repository := store.NewCandleRepository(pool, cfg.Database.Timeout)
	worker, err := candle.NewWorker(repository, publisher, candle.WorkerConfig{
		PriceScale: cfg.PriceScale, QuantityScale: cfg.QuantityScale, OperationTimeout: cfg.OperationTimeout,
	})
	if err != nil {
		return errors.New("create candle worker")
	}
	subscription, err := natsConnection.SubscribeSync("market.*.trade")
	if err != nil {
		return errors.New("subscribe market trades")
	}
	defer subscription.Unsubscribe()
	flushContext, flushCancel := context.WithTimeout(ctx, cfg.OperationTimeout)
	err = natsConnection.FlushWithContext(flushContext)
	flushCancel()
	if err != nil {
		return errors.New("activate market trade subscription")
	}
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return errors.New("listen candle health server")
	}
	server := candleHealthServer(cfg.HTTPAddr, pool, natsConnection, cfg.OperationTimeout)
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(listener) }()
	workerErrors := make(chan error, 1)
	go func() { workerErrors <- consume(ctx, subscription, worker, logger) }()

	select {
	case err := <-workerErrors:
		_ = server.Close()
		return err
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("serve candle health")
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return errors.New("shutdown candle health")
		}
		return ctx.Err()
	}
}

type databasePinger interface {
	Ping(context.Context) error
}

func candleHealthServer(address string, database databasePinger, connection *nats.Conn, timeout time.Duration) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, request *http.Request) {
		checkContext, cancel := context.WithTimeout(request.Context(), timeout)
		defer cancel()
		if database.Ping(checkContext) != nil || !connection.IsConnected() {
			http.Error(writer, "not ready", http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"ready"}`))
	})
	return &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
}

func consume(ctx context.Context, subscription *nats.Subscription, worker *candle.Worker, logger *slog.Logger) error {
	for {
		message, err := subscription.NextMsgWithContext(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("read market trade")
		}
		if err := worker.Handle(ctx, message.Data); err != nil {
			logger.Error("process market trade failed")
			return errors.New("process market trade")
		}
	}
}
