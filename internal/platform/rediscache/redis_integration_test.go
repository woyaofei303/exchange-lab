//go:build integration

package rediscache_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"exchange-lab/internal/market"
	"exchange-lab/internal/platform/rediscache"
	"exchange-lab/internal/trading"
	redisclient "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func TestRedisCacheRateLimitDenylistAndFailureFallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := tcredis.Run(ctx, "redis:8.4.1-alpine")
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start Redis container: %v", err)
	}
	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("Redis connection string: %v", err)
	}
	options, err := redisclient.ParseURL(uri)
	if err != nil {
		t.Fatalf("parse Redis connection string: %v", err)
	}
	client := redisclient.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })

	cache, err := rediscache.NewMarketCache(client, rediscache.MarketCacheConfig{
		OperationTimeout: 200 * time.Millisecond,
		InstrumentTTL:    time.Minute,
		TickerTTL:        5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewMarketCache() error = %v", err)
	}
	instrument := mustRedisInstrument(t)
	if _, hit, err := cache.GetInstruments(ctx, instrument.Symbol()); err != nil || hit {
		t.Fatalf("initial GetInstruments() hit = %t, error = %v", hit, err)
	}
	if err := cache.PutInstruments(ctx, instrument.Symbol(), []trading.Instrument{instrument}); err != nil {
		t.Fatalf("PutInstruments() error = %v", err)
	}
	got, hit, err := cache.GetInstruments(ctx, instrument.Symbol())
	if err != nil || !hit || len(got) != 1 || got[0].PriceStep().String() != "0.10" || got[0].MinNotional().Decimal() != "5.00000000" {
		t.Fatalf("cached instruments = %#v, hit = %t, error = %v", got, hit, err)
	}
	if err := cache.InvalidateInstruments(ctx, instrument.Symbol()); err != nil {
		t.Fatalf("InvalidateInstruments() error = %v", err)
	}
	if _, hit, err := cache.GetInstruments(ctx, instrument.Symbol()); err != nil || hit {
		t.Fatalf("GetInstruments() after invalidation hit = %t, error = %v", hit, err)
	}

	ticker := market.Ticker{Symbol: instrument.Symbol(), LastPrice: "50000.00", MarkPrice: "50001.00", IndexPrice: "49999.00", UpdatedAt: time.Now().UTC()}
	if err := cache.PutTickers(ctx, ticker.Symbol, []market.Ticker{ticker}); err != nil {
		t.Fatalf("PutTickers() error = %v", err)
	}
	tickers, hit, err := cache.GetTickers(ctx, ticker.Symbol)
	if err != nil || !hit || len(tickers) != 1 || tickers[0].LastPrice != ticker.LastPrice {
		t.Fatalf("cached tickers = %#v, hit = %t, error = %v", tickers, hit, err)
	}
	if err := cache.PutTicker(ctx, ticker); err != nil {
		t.Fatalf("PutTicker() error = %v", err)
	}
	allTickers, hit, err := cache.GetTickers(ctx, "")
	if err != nil || !hit || len(allTickers) != 1 || allTickers[0].LastPrice != ticker.LastPrice {
		t.Fatalf("all cached tickers = %#v, hit = %t, error = %v", allTickers, hit, err)
	}

	limiter, err := rediscache.NewLoginRateLimiter(client, 200*time.Millisecond, 2, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginRateLimiter() error = %v", err)
	}
	first, err := limiter.Allow(ctx, "User@Example.Test")
	if err != nil || !first.Allowed || first.Remaining != 1 {
		t.Fatalf("first Allow() = %#v, %v", first, err)
	}
	second, err := limiter.Allow(ctx, "user@example.test")
	if err != nil || !second.Allowed || second.Remaining != 0 {
		t.Fatalf("second Allow() = %#v, %v", second, err)
	}
	third, err := limiter.Allow(ctx, "USER@example.test")
	if err != nil || third.Allowed || third.RetryAfter <= 0 {
		t.Fatalf("third Allow() = %#v, %v", third, err)
	}

	denylist, err := rediscache.NewTokenDenylist(client, 200*time.Millisecond, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewTokenDenylist() error = %v", err)
	}
	const tokenID = "session-token-secret"
	denied, err := denylist.IsDenied(ctx, tokenID)
	if err != nil || denied {
		t.Fatalf("initial IsDenied() = %t, %v", denied, err)
	}
	if err := denylist.Deny(ctx, tokenID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Deny() error = %v", err)
	}
	denied, err = denylist.IsDenied(ctx, tokenID)
	if err != nil || !denied {
		t.Fatalf("IsDenied() = %t, %v", denied, err)
	}

	keys, err := client.Keys(ctx, "exchange-lab:v1:*").Result()
	if err != nil {
		t.Fatalf("list test keys: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("Redis integration created no namespaced keys")
	}
	for _, key := range keys {
		if strings.Contains(strings.ToLower(key), "user@example.test") || strings.Contains(key, tokenID) {
			t.Fatalf("Redis key leaked private input: %s", key)
		}
		ttl, err := client.TTL(ctx, key).Result()
		if err != nil || ttl <= 0 {
			t.Fatalf("key %s TTL = %s, error = %v", key, ttl, err)
		}
	}
	authority := market.NewStaticReader([]trading.Instrument{instrument}, nil)
	cachedReader, err := market.NewCachedReader(authority, cache, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewCachedReader() error = %v", err)
	}
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush rebuildable Redis data: %v", err)
	}
	fromAuthority, err := cachedReader.Instruments(ctx, instrument.Symbol())
	if err != nil || len(fromAuthority) != 1 || fromAuthority[0].Symbol() != instrument.Symbol() {
		t.Fatalf("fallback after Redis flush = %#v, error = %v", fromAuthority, err)
	}

	stopTimeout := 5 * time.Second
	if err := container.Stop(ctx, &stopTimeout); err != nil {
		t.Fatalf("stop Redis container: %v", err)
	}
	fromAuthority, err = cachedReader.Instruments(ctx, instrument.Symbol())
	if err != nil || len(fromAuthority) != 1 || fromAuthority[0].Symbol() != instrument.Symbol() {
		t.Fatalf("fallback instruments = %#v, error = %v", fromAuthority, err)
	}
	if _, err := limiter.Allow(ctx, "user@example.test"); err == nil {
		t.Fatal("Allow() error = nil while Redis is unavailable")
	}
	if _, err := denylist.IsDenied(ctx, tokenID); err == nil {
		t.Fatal("IsDenied() error = nil while Redis is unavailable")
	}
}

func mustRedisInstrument(t *testing.T) trading.Instrument {
	t.Helper()

	base, err := trading.NewAsset("BTC", 8)
	if err != nil {
		t.Fatal(err)
	}
	quote, err := trading.NewAsset("USDT", 8)
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := trading.NewInstrument(trading.InstrumentSpec{
		Base: base, Quote: quote, PriceScale: 2, QuantityScale: 3,
		PriceStep: "0.10", QuantityStep: "0.001", MinNotional: "5",
	})
	if err != nil {
		t.Fatal(err)
	}
	return instrument
}
