//go:build integration

package health_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"github.com/woyaofei303/exchange-lab/internal/health"
)

func TestDependencyCheckersReachComposeServices(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, "postgres://exchange:exchange_dev_only@127.0.0.1:5432/exchange?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	defer redisClient.Close()

	natsConnection, err := nats.Connect("nats://127.0.0.1:4222")
	if err != nil {
		t.Fatal(err)
	}
	defer natsConnection.Close()

	jetStream, err := natsConnection.JetStream()
	if err != nil {
		t.Fatal(err)
	}

	checkers := []health.Checker{
		health.NewPostgresChecker(pool),
		health.NewRedisChecker(redisClient),
		health.NewNATSChecker(natsConnection, jetStream),
	}
	for _, current := range checkers {
		if err := current.Check(ctx); err != nil {
			t.Errorf("%s Check() error = %v", current.Name(), err)
		}
	}
}
