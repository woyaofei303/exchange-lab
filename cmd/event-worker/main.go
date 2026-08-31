// Command event-worker 发布 Outbox 事件并运行持久消费者。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"exchange-lab/internal/eventing"
	store "exchange-lab/internal/platform/postgres"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type workerConfig struct {
	HTTPAddr     string
	PostgresDSN  string
	NATSURL      string
	Timeout      time.Duration
	Shutdown     time.Duration
	BatchSize    int
	PollInterval time.Duration
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	config, err := loadConfig()
	if err != nil {
		logger.Error("invalid event worker configuration")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, config, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("event worker stopped")
		os.Exit(1)
	}
}

func run(ctx context.Context, config workerConfig, logger *slog.Logger) error {
	pool, err := store.OpenPool(ctx, config.PostgresDSN, store.PoolConfig{MinConns: 1, MaxConns: 4, ConnectTimeout: config.Timeout})
	if err != nil {
		return errors.New("create event database client")
	}
	defer pool.Close()
	connection, err := nats.Connect(config.NATSURL, nats.Name("exchange-lab-event-worker"), nats.Timeout(config.Timeout), nats.MaxReconnects(-1), nats.ReconnectWait(500*time.Millisecond))
	if err != nil {
		return errors.New("connect event NATS")
	}
	defer connection.Close()
	js, err := jetstream.New(connection)
	if err != nil {
		return errors.New("create event JetStream client")
	}
	setupContext, setupCancel := context.WithTimeout(ctx, config.Timeout)
	consumer, err := eventing.Configure(setupContext, js)
	setupCancel()
	if err != nil {
		return errors.New("configure event delivery")
	}
	setupContext, setupCancel = context.WithTimeout(ctx, config.Timeout)
	matchingConsumer, err := eventing.ConfigureMatching(setupContext, js)
	setupCancel()
	if err != nil {
		return errors.New("configure matching delivery")
	}
	setupContext, setupCancel = context.WithTimeout(ctx, config.Timeout)
	positionConsumer, err := eventing.ConfigurePosition(setupContext, js)
	setupCancel()
	if err != nil {
		return errors.New("configure position delivery")
	}
	repository := store.NewEventRepository(pool, config.Timeout)
	matchingRepository := store.NewMatchingRepository(pool, config.Timeout)
	positionRepository := store.NewPositionRepository(pool, config.Timeout)
	transport, err := eventing.NewNATSTransport(js)
	if err != nil {
		return errors.New("create event transport")
	}
	publisher, err := eventing.NewPublisher(repository, transport, eventing.PublisherConfig{BatchSize: config.BatchSize, Lease: 30 * time.Second, OperationTimeout: config.Timeout, Now: time.Now})
	if err != nil {
		return errors.New("create outbox publisher")
	}
	consumeErrors := make(chan error, 1)
	consumeContext, err := consumer.Consume(func(message jetstream.Msg) {
		handleMessage(ctx, repository, message, logger)
	}, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, consumeErr error) {
		select {
		case consumeErrors <- consumeErr:
		default:
		}
	}))
	if err != nil {
		return errors.New("start order event consumer")
	}
	defer consumeContext.Stop()
	matchingConsumeContext, err := matchingConsumer.Consume(func(message jetstream.Msg) {
		handleMatchingMessage(ctx, repository, matchingRepository, message, logger)
	}, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, consumeErr error) {
		select {
		case consumeErrors <- consumeErr:
		default:
		}
	}))
	if err != nil {
		return errors.New("start matching event consumer")
	}
	defer matchingConsumeContext.Stop()
	positionConsumeContext, err := positionConsumer.Consume(func(message jetstream.Msg) {
		handlePositionMessage(ctx, repository, positionRepository, message, logger)
	}, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, consumeErr error) {
		select {
		case consumeErrors <- consumeErr:
		default:
		}
	}))
	if err != nil {
		return errors.New("start position event consumer")
	}
	defer positionConsumeContext.Stop()

	listener, err := net.Listen("tcp", config.HTTPAddr)
	if err != nil {
		return errors.New("listen event health server")
	}
	server := healthServer(config.HTTPAddr, pool, connection, config.Timeout)
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(listener) }()
	go publishLoop(ctx, publisher, config.PollInterval, logger)

	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), config.Shutdown)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return errors.New("shutdown event health server")
		}
		return ctx.Err()
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("serve event health")
	case <-consumeErrors:
		return errors.New("consume order event")
	}
}

func handlePositionMessage(ctx context.Context, events *store.EventRepository, positions *store.PositionRepository, message jetstream.Msg, logger *slog.Logger) {
	metadata, err := message.Metadata()
	if err != nil {
		_ = message.NakWithDelay(time.Second)
		return
	}
	envelope, err := eventing.Decode(message.Data())
	if err != nil || envelope.Type != message.Subject() || message.Headers().Get(eventing.EventIDHeader) != envelope.ID {
		id := message.Headers().Get(eventing.EventIDHeader)
		if id == "" {
			id = fmt.Sprintf("malformed_%d", metadata.Sequence.Stream)
		}
		if events.Park(ctx, store.PositionConsumer, eventing.Envelope{ID: id, Type: message.Subject()}, []byte(`{"redacted":true}`), metadata.NumDelivered, "invalid_envelope", time.Now()) == nil {
			_ = message.Term()
			return
		}
		_ = message.NakWithDelay(time.Second)
		return
	}
	_, err = positions.Process(ctx, envelope, time.Now())
	if err == nil {
		_ = message.Ack()
		return
	}
	if metadata.NumDelivered >= 5 {
		if events.Park(ctx, store.PositionConsumer, envelope, message.Data(), metadata.NumDelivered, "position_failed", time.Now()) == nil {
			logger.Warn("parked exhausted position event", "event_id", envelope.ID)
			_ = message.Term()
			return
		}
	}
	_ = message.NakWithDelay(time.Second)
}

func publishLoop(ctx context.Context, publisher *eventing.Publisher, interval time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := publisher.Tick(ctx); err != nil && ctx.Err() == nil {
			logger.Warn("outbox publish attempt failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func handleMessage(ctx context.Context, repository *store.EventRepository, message jetstream.Msg, logger *slog.Logger) {
	metadata, err := message.Metadata()
	if err != nil {
		_ = message.NakWithDelay(time.Second)
		return
	}
	envelope, err := eventing.Decode(message.Data())
	if err != nil || envelope.Type != message.Subject() || message.Headers().Get(eventing.EventIDHeader) != envelope.ID {
		id := message.Headers().Get(eventing.EventIDHeader)
		if id == "" {
			id = fmt.Sprintf("malformed_%d", metadata.Sequence.Stream)
		}
		redacted := eventing.Envelope{ID: id, Type: message.Subject()}
		if parkErr := repository.Park(ctx, eventing.AuditConsumer, redacted, []byte(`{"redacted":true}`), metadata.NumDelivered, "invalid_envelope", time.Now()); parkErr != nil {
			_ = message.NakWithDelay(time.Second)
			return
		}
		logger.Warn("parked invalid order event", "event_id", id)
		_ = message.Term()
		return
	}
	_, err = repository.RecordReceipt(ctx, eventing.AuditConsumer, envelope, time.Now())
	if err == nil {
		_ = message.Ack()
		return
	}
	if metadata.NumDelivered >= 5 {
		if parkErr := repository.Park(ctx, eventing.AuditConsumer, envelope, message.Data(), metadata.NumDelivered, "processing_failed", time.Now()); parkErr == nil {
			logger.Warn("parked exhausted order event", "event_id", envelope.ID)
			_ = message.Term()
			return
		}
	}
	_ = message.NakWithDelay(time.Second)
}

func handleMatchingMessage(ctx context.Context, events *store.EventRepository, matching *store.MatchingRepository, message jetstream.Msg, logger *slog.Logger) {
	metadata, err := message.Metadata()
	if err != nil {
		_ = message.NakWithDelay(time.Second)
		return
	}
	envelope, err := eventing.Decode(message.Data())
	if err != nil || envelope.Type != message.Subject() || message.Headers().Get(eventing.EventIDHeader) != envelope.ID {
		id := message.Headers().Get(eventing.EventIDHeader)
		if id == "" {
			id = fmt.Sprintf("malformed_%d", metadata.Sequence.Stream)
		}
		redacted := eventing.Envelope{ID: id, Type: message.Subject()}
		if parkErr := events.Park(ctx, store.MatchingConsumer, redacted, []byte(`{"redacted":true}`), metadata.NumDelivered, "invalid_envelope", time.Now()); parkErr == nil {
			_ = message.Term()
			return
		}
		_ = message.NakWithDelay(time.Second)
		return
	}
	_, _, err = matching.Process(ctx, envelope, metadata.Sequence.Stream, time.Now())
	if err == nil {
		_ = message.Ack()
		return
	}
	if metadata.NumDelivered >= 5 {
		if parkErr := events.Park(ctx, store.MatchingConsumer, envelope, message.Data(), metadata.NumDelivered, "matching_failed", time.Now()); parkErr == nil {
			logger.Warn("parked exhausted matching event", "event_id", envelope.ID)
			_ = message.Term()
			return
		}
	}
	_ = message.NakWithDelay(time.Second)
}

type pinger interface{ Ping(context.Context) error }

func healthServer(address string, database pinger, connection *nats.Conn, timeout time.Duration) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, request *http.Request) {
		check, cancel := context.WithTimeout(request.Context(), timeout)
		defer cancel()
		if database.Ping(check) != nil || !connection.IsConnected() {
			http.Error(writer, "not ready", http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"ready"}`))
	})
	return &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
}

func loadConfig() (workerConfig, error) {
	config := workerConfig{
		HTTPAddr:    env("EVENT_HTTP_ADDR", ":8092"),
		PostgresDSN: env("POSTGRES_DSN", "postgres://exchange:exchange_dev_only@127.0.0.1:5432/exchange?sslmode=disable"),
		NATSURL:     env("NATS_URL", "nats://127.0.0.1:4222"),
		Timeout:     2 * time.Second, Shutdown: 10 * time.Second, BatchSize: 50, PollInterval: 200 * time.Millisecond,
	}
	var err error
	if config.Timeout, err = durationEnv("EVENT_OPERATION_TIMEOUT", config.Timeout); err != nil {
		return workerConfig{}, err
	}
	if config.Shutdown, err = durationEnv("SHUTDOWN_TIMEOUT", config.Shutdown); err != nil {
		return workerConfig{}, err
	}
	if config.PollInterval, err = durationEnv("EVENT_POLL_INTERVAL", config.PollInterval); err != nil {
		return workerConfig{}, err
	}
	if value := os.Getenv("EVENT_BATCH_SIZE"); value != "" {
		config.BatchSize, err = strconv.Atoi(value)
		if err != nil {
			return workerConfig{}, err
		}
	}
	if config.HTTPAddr == "" || config.PostgresDSN == "" || config.NATSURL == "" || config.Timeout <= 0 || config.Shutdown <= 0 || config.PollInterval < 10*time.Millisecond || config.BatchSize < 1 || config.BatchSize > 1000 {
		return workerConfig{}, errors.New("invalid event worker settings")
	}
	return config, nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	return time.ParseDuration(value)
}
