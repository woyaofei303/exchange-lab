package marketdata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/market"
)

// ErrReconnectExhausted reports that the configured retry budget is spent.
var ErrReconnectExhausted = errors.New("market reconnect limit exhausted")

// Connection is the single-reader feed session used by Worker.
type Connection interface {
	Read(context.Context) ([]byte, error)
	Ping(context.Context) error
	Close() error
}

// Connector opens one market-data session.
type Connector interface {
	Connect(context.Context) (Connection, error)
}

// Publisher distributes accepted normalized events.
type Publisher interface {
	Publish(context.Context, string, []byte) error
}

// TickerCache stores only the rebuildable latest-ticker projection.
type TickerCache interface {
	PutTicker(context.Context, market.Ticker) error
}

// GapReporter records sequence evidence before a session reconnects.
type GapReporter interface {
	ReportGap(context.Context, GapObservation)
}

// WorkerConfig bounds reconnect, heartbeat and downstream operations.
type WorkerConfig struct {
	MaxReconnects     uint
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	HeartbeatInterval time.Duration
	OperationTimeout  time.Duration
	Sleep             func(context.Context, time.Duration) error
}

// Worker validates, sequences and publishes one feed session at a time.
type Worker struct {
	connector Connector
	publisher Publisher
	cache     TickerCache
	reporter  GapReporter
	config    WorkerConfig
	sequence  *Sequencer
}

// NewWorker validates all I/O dependencies and retry bounds.
func NewWorker(connector Connector, publisher Publisher, cache TickerCache, reporter GapReporter, config WorkerConfig) (*Worker, error) {
	if connector == nil || publisher == nil || cache == nil || reporter == nil ||
		config.InitialBackoff <= 0 || config.MaxBackoff < config.InitialBackoff ||
		config.HeartbeatInterval <= 0 || config.OperationTimeout <= 0 {
		return nil, fmt.Errorf("create market worker: invalid configuration")
	}
	if config.Sleep == nil {
		config.Sleep = sleepContext
	}
	return &Worker{connector: connector, publisher: publisher, cache: cache, reporter: reporter, config: config, sequence: NewSequencer()}, nil
}

// Run reconnects with capped exponential backoff until cancellation or exhaustion.
func (w *Worker) Run(ctx context.Context) error {
	for attempt := uint(0); ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		connection, err := w.connector.Connect(ctx)
		if err == nil {
			err = w.consume(ctx, connection)
			_ = connection.Close()
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt >= w.config.MaxReconnects {
			return ErrReconnectExhausted
		}
		if err := w.config.Sleep(ctx, backoff(w.config.InitialBackoff, w.config.MaxBackoff, attempt)); err != nil {
			return err
		}
	}
}

func (w *Worker) consume(ctx context.Context, connection Connection) error {
	sessionContext, cancel := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	go w.heartbeat(sessionContext, connection, cancel, heartbeatDone)
	defer func() {
		cancel()
		<-heartbeatDone
	}()

	for {
		payload, err := connection.Read(sessionContext)
		if err != nil {
			select {
			case heartbeatErr := <-heartbeatDone:
				heartbeatDone <- heartbeatErr
				if heartbeatErr != nil {
					return heartbeatErr
				}
			default:
			}
			return err
		}
		event, err := Decode(payload)
		if err != nil {
			continue
		}
		observation := w.sequence.Observe(event)
		switch observation.Disposition {
		case Duplicate, OutOfOrder:
			continue
		case Gap:
			w.reporter.ReportGap(ctx, observation.Gap)
			return fmt.Errorf("market sequence gap")
		}
		if err := w.deliver(ctx, event); err != nil {
			return err
		}
	}
}

func (w *Worker) heartbeat(ctx context.Context, connection Connection, cancel context.CancelFunc, done chan<- error) {
	ticker := time.NewTicker(w.config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			done <- nil
			return
		case <-ticker.C:
			operationContext, operationCancel := context.WithTimeout(ctx, w.config.OperationTimeout)
			err := connection.Ping(operationContext)
			operationCancel()
			if err != nil {
				done <- err
				cancel()
				return
			}
		}
	}
}

func (w *Worker) deliver(ctx context.Context, event Event) error {
	payload, err := Encode(event)
	if err != nil {
		return err
	}
	subject, err := Subject(event)
	if err != nil {
		return err
	}
	operationContext, cancel := context.WithTimeout(ctx, w.config.OperationTimeout)
	err = w.publisher.Publish(operationContext, subject, payload)
	cancel()
	if err != nil {
		return fmt.Errorf("publish market event")
	}
	if event.Ticker != nil {
		ticker := market.Ticker{
			Symbol: event.Symbol, LastPrice: event.Ticker.LastPrice, MarkPrice: event.Ticker.MarkPrice,
			IndexPrice: event.Ticker.IndexPrice, PriceChange: event.Ticker.PriceChange,
			Volume: event.Ticker.Volume, UpdatedAt: event.EmittedAt,
		}
		cacheContext, cacheCancel := context.WithTimeout(ctx, w.config.OperationTimeout)
		_ = w.cache.PutTicker(cacheContext, ticker)
		cacheCancel()
	}
	return nil
}

func backoff(initial, maximum time.Duration, attempt uint) time.Duration {
	delay := initial
	for index := uint(0); index < attempt; index++ {
		if delay >= maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
