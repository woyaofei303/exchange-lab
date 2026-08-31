// Command marketcheck 验证实时规范化行情流水线。
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"exchange-lab/internal/marketdata"
	"exchange-lab/internal/platform/rediscache"
	"github.com/nats-io/nats.go"
	redisclient "github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("market pipeline: WebSocket -> worker -> NATS + Redis verified")
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	natsURL := environmentOr("NATS_URL", "nats://127.0.0.1:4222")
	connection, err := nats.Connect(natsURL, nats.Name("exchange-lab-market-check"), nats.Timeout(2*time.Second))
	if err != nil {
		return errors.New("market check: connect NATS")
	}
	defer connection.Close()
	subscription, err := connection.SubscribeSync("market.>")
	if err != nil {
		return errors.New("market check: subscribe NATS")
	}
	defer subscription.Unsubscribe()
	if err := connection.FlushWithContext(ctx); err != nil {
		return errors.New("market check: flush subscription")
	}
	message, err := subscription.NextMsgWithContext(ctx)
	if err != nil {
		return errors.New("market check: wait for normalized event")
	}
	if err := validateMessage(message.Subject, message.Data); err != nil {
		return err
	}

	redis := redisclient.NewClient(&redisclient.Options{Addr: environmentOr("REDIS_ADDR", "127.0.0.1:6379")})
	defer redis.Close()
	cache, err := rediscache.NewMarketCache(redis, rediscache.MarketCacheConfig{
		OperationTimeout: time.Second, InstrumentTTL: time.Minute, TickerTTL: 5 * time.Second,
	})
	if err != nil {
		return errors.New("market check: create Redis cache reader")
	}
	tickers, hit, err := cache.GetTickers(ctx, "BTCUSDT-PERP")
	if err != nil || !hit || len(tickers) != 1 || tickers[0].Symbol != "BTCUSDT-PERP" {
		return errors.New("market check: latest ticker cache unavailable")
	}
	return nil
}

func validateMessage(subject string, payload []byte) error {
	event, err := marketdata.Decode(payload)
	if err != nil {
		return errors.New("market check: invalid normalized event")
	}
	wantSubject, err := marketdata.Subject(event)
	if err != nil || wantSubject != subject {
		return errors.New("market check: subject and event mismatch")
	}
	return nil
}

func environmentOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
