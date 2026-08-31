// Package app 组装 API 依赖并协调优雅启停生命周期。
package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	contractai "exchange-lab/internal/ai"
	"exchange-lab/internal/candle"
	"exchange-lab/internal/config"
	"exchange-lab/internal/health"
	"exchange-lab/internal/httpserver"
	"exchange-lab/internal/identity"
	"exchange-lab/internal/market"
	"exchange-lab/internal/observability"
	domainorder "exchange-lab/internal/order"
	store "exchange-lab/internal/platform/postgres"
	"exchange-lab/internal/platform/rediscache"
	"exchange-lab/internal/realtime"
	"exchange-lab/migrations"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

const (
	natsConnectTimeout = time.Second
	natsReconnectWait  = 500 * time.Millisecond
)

// Run 组装并启动 API，持续运行直至收到取消信号或发生进程级故障。
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) (runErr error) {
	traceShutdown, err := observability.SetupTracing(ctx, "exchange-api", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if err != nil {
		return err
	}
	defer func() { _ = traceShutdown(context.Background()) }()
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return errors.New("listen http server")
	}
	defer listener.Close()

	pool, err := store.OpenPool(ctx, cfg.PostgresDSN, store.PoolConfig{
		MinConns:       cfg.PostgresMinConns,
		MaxConns:       cfg.PostgresMaxConns,
		ConnectTimeout: cfg.DatabaseTimeout,
	})
	if err != nil {
		return errors.New("create postgres client")
	}
	defer pool.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer func() {
		runErr = redisCloseResult(runErr, redisClient.Close())
	}()

	natsConnection, err := connectNATS(cfg.NATSURL)
	if err != nil {
		return err
	}
	defer natsConnection.Close()

	jetStream, err := natsConnection.JetStream()
	if err != nil {
		return errors.New("create jetstream context")
	}

	readiness := health.NewService(cfg.DependencyTimeout,
		store.NewSchemaChecker(pool, migrations.CurrentVersion),
		health.NewRedisChecker(redisClient),
		health.NewNATSChecker(natsConnection, jetStream),
	)
	instruments := store.NewInstrumentRepository(pool, cfg.DatabaseTimeout)
	authoritativeMarkets, err := market.NewCompositeReader(instruments, defaultTickerReader(time.Now().UTC()))
	if err != nil {
		return errors.New("create market reader")
	}
	marketCache, err := rediscache.NewMarketCache(redisClient, rediscache.MarketCacheConfig{
		OperationTimeout: cfg.CacheTimeout,
		InstrumentTTL:    cfg.InstrumentCacheTTL,
		TickerTTL:        cfg.TickerCacheTTL,
	})
	if err != nil {
		return errors.New("create market cache")
	}
	markets, err := market.NewCachedReader(authoritativeMarkets, marketCache, logger)
	if err != nil {
		return errors.New("create cached market reader")
	}
	candles := store.NewCandleRepository(pool, cfg.DatabaseTimeout)
	identityRepository := store.NewIdentityRepository(pool, cfg.DatabaseTimeout)
	identityService, err := identity.NewService(identityRepository, identity.DefaultPasswordHasher(), identity.TokenIssuer{}, cfg.AuthSessionTTL, time.Now)
	if err != nil {
		return errors.New("create identity service")
	}
	loginLimiter, err := rediscache.NewLoginRateLimiter(redisClient, cfg.CacheTimeout, cfg.LoginRateLimit, cfg.LoginRateWindow)
	if err != nil {
		return errors.New("create login limiter")
	}
	ledgerRepository := store.NewLedgerRepository(pool, cfg.DatabaseTimeout)
	orderRepository := store.NewOrderRepository(pool, cfg.DatabaseTimeout)
	priceRepository := store.NewPriceRepository(pool, cfg.DatabaseTimeout)
	positionRepository := store.NewPositionRepository(pool, cfg.DatabaseTimeout)
	riskRepository := store.NewRiskRepository(pool, cfg.DatabaseTimeout)
	aiAuditRepository := store.NewAIRepository(pool, cfg.DatabaseTimeout)
	var riskModel contractai.Model = contractai.DeterministicModel{}
	aiModelName := "deterministic-local"
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		aiModelName = os.Getenv("OPENAI_MODEL")
		if aiModelName == "" {
			aiModelName = "gpt-5-mini"
		}
		openAIModel, modelErr := contractai.NewOpenAIModel(apiKey, aiModelName)
		if modelErr != nil {
			return errors.New("create AI model")
		}
		riskModel = openAIModel
	}
	aiService, err := contractai.NewService(riskRepository, riskModel, aiAuditRepository, aiModelName, "contract-risk-v1", time.Now)
	if err != nil {
		return errors.New("create AI risk service")
	}
	metrics := observability.New("api")
	orderService, err := domainorder.NewService(instruments, orderRepository, time.Now)
	if err != nil {
		return errors.New("create order service")
	}
	candleHub, err := realtime.NewHub(64)
	if err != nil {
		return errors.New("create candle stream hub")
	}
	candleSubscription, err := natsConnection.Subscribe("candles.>", func(message *nats.Msg) {
		update, decodeErr := candle.DecodeUpdate(message.Data)
		if decodeErr != nil {
			return
		}
		subject, subjectErr := candle.UpdateSubject(update)
		if subjectErr == nil && subject == message.Subject {
			candleHub.Publish(message.Data)
		}
	})
	if err != nil {
		return errors.New("subscribe candle updates")
	}
	defer candleSubscription.Unsubscribe()
	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpserver.NewHandlerWithFeatures(readiness, markets, httpserver.Features{
			Candles: candles, CandleHub: candleHub, Identity: identityService, LoginLimiter: loginLimiter,
			AuthCookieSecure: cfg.AuthCookieSecure, AuthSessionTTL: cfg.AuthSessionTTL, Ledger: ledgerRepository, Orders: orderService,
			Prices:    priceRepository,
			Positions: positionRepository, Derivatives: riskRepository,
			AI:      aiService,
			Metrics: metrics,
		}, logger, cfg.RequestTimeout),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return serve(ctx, server, listener, cfg.ShutdownTimeout)
}

func defaultTickerReader(updatedAt time.Time) market.TickerReader {
	return market.NewStaticReader(
		nil,
		[]market.Ticker{{
			Symbol:      "BTCUSDT-PERP",
			LastPrice:   "50000.00",
			MarkPrice:   "50000.00",
			IndexPrice:  "50000.00",
			PriceChange: "0.00",
			Volume:      "0.000",
			UpdatedAt:   updatedAt,
		}},
	)
}

func connectNATS(natsURL string) (*nats.Conn, error) {
	connection, err := nats.Connect(natsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(natsReconnectWait),
		nats.Timeout(natsConnectTimeout),
	)
	if err != nil {
		return nil, errors.New("create nats client")
	}
	return connection, nil
}

func redisCloseResult(primaryErr, closeErr error) error {
	if primaryErr != nil {
		return primaryErr
	}
	if closeErr != nil {
		return errors.New("close redis client")
	}
	return nil
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
