// Command market-worker 校验、排序并分发行情数据。
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"exchange-lab/internal/config"
	"exchange-lab/internal/marketdata"
	"exchange-lab/internal/platform/natspublisher"
	"exchange-lab/internal/platform/rediscache"
	"exchange-lab/internal/platform/websocketfeed"
	"github.com/nats-io/nats.go"
	redisclient "github.com/redis/go-redis/v9"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.LoadMarketWorker(os.LookupEnv)
	if err != nil {
		logger.Error("invalid market worker configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("market worker stopped")
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.MarketWorkerConfig, logger *slog.Logger) error {
	connector, err := websocketfeed.NewConnector(cfg.WebSocketURL, cfg.MaxMessageBytes)
	if err != nil {
		return errors.New("create market connector")
	}
	natsConnection, err := nats.Connect(cfg.NATSURL,
		nats.Name("exchange-lab-market-worker"),
		nats.Timeout(cfg.OperationTimeout),
		nats.MaxReconnects(int(cfg.MaxReconnects)),
		nats.ReconnectWait(cfg.InitialBackoff),
	)
	if err != nil {
		return errors.New("connect market NATS")
	}
	defer natsConnection.Close()
	publisher, err := natspublisher.New(natsConnection)
	if err != nil {
		return errors.New("create market publisher")
	}
	redis := redisclient.NewClient(&redisclient.Options{Addr: cfg.RedisAddr})
	defer redis.Close()
	cache, err := rediscache.NewMarketCache(redis, rediscache.MarketCacheConfig{
		OperationTimeout: cfg.CacheTimeout, InstrumentTTL: cfg.InstrumentCacheTTL, TickerTTL: cfg.TickerCacheTTL,
	})
	if err != nil {
		return errors.New("create market ticker cache")
	}
	worker, err := marketdata.NewWorker(connector, publisher, cache, slogGapReporter{logger: logger}, marketdata.WorkerConfig{
		MaxReconnects: cfg.MaxReconnects, InitialBackoff: cfg.InitialBackoff, MaxBackoff: cfg.MaxBackoff,
		HeartbeatInterval: cfg.HeartbeatInterval, OperationTimeout: cfg.OperationTimeout,
	})
	if err != nil {
		return errors.New("create market worker")
	}
	return worker.Run(ctx)
}

type slogGapReporter struct {
	logger *slog.Logger
}

// ReportGap 记录有界的序列证据，不暴露载荷数据。
func (r slogGapReporter) ReportGap(_ context.Context, gap marketdata.GapObservation) {
	r.logger.Warn("market sequence gap",
		"stream_id", gap.StreamID, "symbol", gap.Symbol,
		"expected_sequence", gap.Expected, "received_sequence", gap.Received,
	)
}
