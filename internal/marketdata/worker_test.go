package marketdata_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"exchange-lab/internal/market"
	"exchange-lab/internal/marketdata"
)

func TestWorkerPublishesAcceptedEventsCachesTickerAndDropsDuplicates(t *testing.T) {
	t.Parallel()

	events := []marketdata.Event{
		bookSnapshot(1),
		tickerEvent(2),
		tickerEvent(2),
	}
	connection := &scriptedConnection{reads: encodeEvents(t, events), terminalErr: io.EOF}
	publisher := &recordingPublisher{}
	cache := &recordingTickerCache{}
	worker := newTestWorker(t, &scriptedConnector{connections: []marketdata.Connection{connection}}, publisher, cache, &recordingGapReporter{}, 0)

	err := worker.Run(context.Background())
	if !errors.Is(err, marketdata.ErrReconnectExhausted) {
		t.Fatalf("Run() error = %v, want ErrReconnectExhausted", err)
	}
	if publisher.Count() != 2 {
		t.Fatalf("published = %d, want 2 accepted events", publisher.Count())
	}
	if got := cache.Last(); got.Symbol != "BTCUSDT-PERP" || got.LastPrice != "50000.10" {
		t.Fatalf("cached ticker = %#v", got)
	}
}

func TestWorkerReportsGapAndReconnectsWithBoundedBackoff(t *testing.T) {
	t.Parallel()

	first := &scriptedConnection{reads: encodeEvents(t, []marketdata.Event{bookSnapshot(1), tickerEvent(3)})}
	second := &scriptedConnection{reads: encodeEvents(t, []marketdata.Event{bookSnapshotFor("stream-b", 1)}), terminalErr: io.EOF}
	connector := &scriptedConnector{connections: []marketdata.Connection{first, second}}
	reporter := &recordingGapReporter{}
	var slept []time.Duration
	worker, err := marketdata.NewWorker(connector, &recordingPublisher{}, &recordingTickerCache{}, reporter, marketdata.WorkerConfig{
		MaxReconnects: 1, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
		HeartbeatInterval: time.Hour, OperationTimeout: 10 * time.Millisecond,
		Sleep: func(_ context.Context, duration time.Duration) error { slept = append(slept, duration); return nil },
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}

	err = worker.Run(context.Background())
	if !errors.Is(err, marketdata.ErrReconnectExhausted) {
		t.Fatalf("Run() error = %v, want ErrReconnectExhausted", err)
	}
	if reporter.Count() != 1 {
		t.Fatalf("gap reports = %d, want 1", reporter.Count())
	}
	if connector.Count() != 2 || len(slept) != 1 || slept[0] != time.Millisecond {
		t.Fatalf("connects=%d slept=%v, want two connects and one bounded delay", connector.Count(), slept)
	}
}

func TestWorkerSkipsInvalidJSONAndStopsOnSlowDownstream(t *testing.T) {
	t.Parallel()

	connection := &scriptedConnection{reads: [][]byte{[]byte(`not-json`), mustEncode(t, bookSnapshot(1))}}
	publisher := &recordingPublisher{publish: func(ctx context.Context, _ string, _ []byte) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	worker := newTestWorker(t, &scriptedConnector{connections: []marketdata.Connection{connection}}, publisher, &recordingTickerCache{}, &recordingGapReporter{}, 0)

	started := time.Now()
	err := worker.Run(context.Background())
	if !errors.Is(err, marketdata.ErrReconnectExhausted) {
		t.Fatalf("Run() error = %v, want ErrReconnectExhausted", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("slow downstream blocked worker for %s", elapsed)
	}
}

func newTestWorker(t *testing.T, connector marketdata.Connector, publisher marketdata.Publisher, cache marketdata.TickerCache, reporter marketdata.GapReporter, reconnects uint) *marketdata.Worker {
	t.Helper()
	worker, err := marketdata.NewWorker(connector, publisher, cache, reporter, marketdata.WorkerConfig{
		MaxReconnects: reconnects, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
		HeartbeatInterval: time.Hour, OperationTimeout: 10 * time.Millisecond,
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	return worker
}

type scriptedConnection struct {
	reads       [][]byte
	terminalErr error
	index       int
}

func (c *scriptedConnection) Read(context.Context) ([]byte, error) {
	if c.index < len(c.reads) {
		result := c.reads[c.index]
		c.index++
		return result, nil
	}
	if c.terminalErr != nil {
		return nil, c.terminalErr
	}
	return nil, io.EOF
}

func (c *scriptedConnection) Ping(context.Context) error { return nil }
func (c *scriptedConnection) Close() error               { return nil }

type scriptedConnector struct {
	mu          sync.Mutex
	connections []marketdata.Connection
	count       int
}

func (c *scriptedConnector) Connect(context.Context) (marketdata.Connection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
	if len(c.connections) == 0 {
		return nil, errors.New("source unavailable")
	}
	connection := c.connections[0]
	c.connections = c.connections[1:]
	return connection, nil
}

func (c *scriptedConnector) Count() int { c.mu.Lock(); defer c.mu.Unlock(); return c.count }

type recordingPublisher struct {
	mu      sync.Mutex
	payload [][]byte
	publish func(context.Context, string, []byte) error
}

func (p *recordingPublisher) Publish(ctx context.Context, subject string, payload []byte) error {
	if p.publish != nil {
		return p.publish(ctx, subject, payload)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.payload = append(p.payload, append([]byte(subject+":"), payload...))
	return nil
}

func (p *recordingPublisher) Count() int { p.mu.Lock(); defer p.mu.Unlock(); return len(p.payload) }

type recordingTickerCache struct {
	mu     sync.Mutex
	ticker market.Ticker
}

func (c *recordingTickerCache) PutTicker(_ context.Context, ticker market.Ticker) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ticker = ticker
	return nil
}

func (c *recordingTickerCache) Last() market.Ticker {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ticker
}

type recordingGapReporter struct {
	mu   sync.Mutex
	gaps []marketdata.GapObservation
}

func (r *recordingGapReporter) ReportGap(_ context.Context, gap marketdata.GapObservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gaps = append(r.gaps, gap)
}

func (r *recordingGapReporter) Count() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.gaps) }

func encodeEvents(t *testing.T, events []marketdata.Event) [][]byte {
	t.Helper()
	result := make([][]byte, 0, len(events))
	for _, event := range events {
		result = append(result, mustEncode(t, event))
	}
	return result
}

func mustEncode(t *testing.T, event marketdata.Event) []byte {
	t.Helper()
	payload, err := marketdata.Encode(event)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	return payload
}

func bookSnapshot(sequence uint64) marketdata.Event { return bookSnapshotFor("stream-a", sequence) }

func bookSnapshotFor(stream string, sequence uint64) marketdata.Event {
	return marketdata.Event{
		Version: 1, StreamID: stream, Type: marketdata.EventBookSnapshot,
		Symbol: "BTCUSDT-PERP", Sequence: sequence, EmittedAt: time.Now().UTC(),
		OrderBook: &marketdata.OrderBook{Bids: []marketdata.Level{{Price: "50000.00", Quantity: "1.000"}}, Asks: []marketdata.Level{{Price: "50000.10", Quantity: "1.000"}}},
	}
}

func tickerEvent(sequence uint64) marketdata.Event {
	return marketdata.Event{
		Version: 1, StreamID: "stream-a", Type: marketdata.EventTicker,
		Symbol: "BTCUSDT-PERP", Sequence: sequence, EmittedAt: time.Now().UTC(),
		Ticker: &marketdata.Ticker{LastPrice: "50000.10", MarkPrice: "50000.10", IndexPrice: "50000.00", PriceChange: "0.10", Volume: "1.000"},
	}
}
