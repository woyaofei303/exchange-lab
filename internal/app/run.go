package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"github.com/woyaofei303/exchange-lab/internal/config"
	"github.com/woyaofei303/exchange-lab/internal/health"
	"github.com/woyaofei303/exchange-lab/internal/httpserver"
)

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return errors.New("listen http server")
	}
	defer listener.Close()

	pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
	if err != nil {
		return errors.New("create postgres client")
	}
	defer pool.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer redisClient.Close()

	natsConnection, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		return errors.New("create nats client")
	}
	defer natsConnection.Close()

	jetStream, err := natsConnection.JetStream()
	if err != nil {
		return errors.New("create jetstream context")
	}

	readiness := health.NewService(cfg.DependencyTimeout,
		health.NewPostgresChecker(pool),
		health.NewRedisChecker(redisClient),
		health.NewNATSChecker(natsConnection, jetStream),
	)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpserver.NewHandler(readiness, logger),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return serve(ctx, server, listener, cfg.ShutdownTimeout)
}

func serve(ctx context.Context, server *http.Server, listener net.Listener, shutdownTimeout time.Duration) error {
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(listener)
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		shutdownErr := server.Shutdown(shutdownContext)
		serverErr := <-serverErrors
		if shutdownErr != nil {
			return shutdownErr
		}
		if errors.Is(serverErr, http.ErrServerClosed) {
			return nil
		}
		return serverErr
	}
}
