package candle_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/candle"
	"github.com/woyaofei303/exchange-lab/internal/marketdata"
)

func TestWorkerStoresOneMinuteRollsUpAndPublishesExactViews(t *testing.T) {
	t.Parallel()

	minute := time.Date(2026, 9, 21, 12, 3, 0, 0, time.UTC)
	oneMinute := minuteCandle(minute, 5_000_000, 5_001_000, 4_999_000, 5_000_500, 10, false)
	store := &candleStore{applyResult: oneMinute, accepted: true, listResult: []candle.Candle{oneMinute}}
	publisher := &candlePublisher{}
	worker := newCandleWorker(t, store, publisher)

	if err := worker.Handle(context.Background(), marketTradePayload(t, minute.Add(10*time.Second))); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if store.upsertCount != 2 {
		t.Fatalf("high-period upserts = %d, want 2", store.upsertCount)
	}
	if publisher.Count() != 3 {
		t.Fatalf("published candle updates = %d, want 1m+5m+1h", publisher.Count())
	}
	for _, message := range publisher.Messages() {
		var update candle.Update
		if err := json.Unmarshal(message.payload, &update); err != nil || update.Version != 1 || update.Type != "candle" || update.Data.Symbol != "BTCUSDT-PERP" {
			t.Fatalf("published update = %#v, decode error = %v", update, err)
		}
		if message.subject != "candles.BTCUSDT-PERP."+update.Data.Interval {
			t.Fatalf("subject = %q, interval = %q", message.subject, update.Data.Interval)
		}
	}
}

func TestWorkerDuplicateIsIdempotentAndInvalidInputIsRejected(t *testing.T) {
	t.Parallel()

	store := &candleStore{applyResult: minuteCandle(time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC), 1, 1, 1, 1, 1, false), accepted: false}
	publisher := &candlePublisher{}
	worker := newCandleWorker(t, store, publisher)
	if err := worker.Handle(context.Background(), marketTradePayload(t, time.Date(2026, 9, 21, 12, 0, 10, 0, time.UTC))); err != nil {
		t.Fatalf("Handle(duplicate) error = %v", err)
	}
	if publisher.Count() != 0 || store.upsertCount != 0 {
		t.Fatalf("duplicate side effects: published=%d upserts=%d", publisher.Count(), store.upsertCount)
	}
	if err := worker.Handle(context.Background(), []byte(`not-json`)); err == nil {
		t.Fatal("Handle(invalid JSON) error = nil")
	}
}

func TestWorkerBoundsSlowRealtimePublisher(t *testing.T) {
	t.Parallel()

	minute := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	oneMinute := minuteCandle(minute, 1, 1, 1, 1, 1, false)
	store := &candleStore{applyResult: oneMinute, accepted: true, listResult: []candle.Candle{oneMinute}}
	publisher := &candlePublisher{publish: func(ctx context.Context, _ string, _ []byte) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	worker := newCandleWorker(t, store, publisher)
	started := time.Now()
	if err := worker.Handle(context.Background(), marketTradePayload(t, minute)); err == nil {
		t.Fatal("Handle(slow publisher) error = nil")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Handle() blocked for %s", elapsed)
	}
}

func TestWorkerRepairFillsMinuteGapAndRebuildsRollups(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	store := &candleStore{listResult: []candle.Candle{
		minuteCandle(start, 100, 110, 90, 105, 1, true),
		minuteCandle(start.Add(2*time.Minute), 106, 112, 101, 108, 1, true),
	}}
	worker := newCandleWorker(t, store, &candlePublisher{})
	if err := worker.Repair(context.Background(), "BTCUSDT-PERP", start, start.Add(3*time.Minute)); err != nil {
		t.Fatalf("Repair() error = %v", err)
	}
	if store.upsertCount < 3 {
		t.Fatalf("repair upserts = %d, want missing minute plus rollups", store.upsertCount)
	}
	foundFlat := false
	for _, item := range store.upserted {
		if item.Interval == candle.OneMinute && item.OpenTime.Equal(start.Add(time.Minute)) && item.VolumeAtoms == 0 && item.Final {
			foundFlat = true
		}
	}
	if !foundFlat {
		t.Fatal("Repair() did not persist flat gap candle")
	}
}

func newCandleWorker(t *testing.T, store candle.Store, publisher candle.Publisher) *candle.Worker {
	t.Helper()
	worker, err := candle.NewWorker(store, publisher, candle.WorkerConfig{PriceScale: 2, QuantityScale: 3, OperationTimeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	return worker
}

func marketTradePayload(t *testing.T, eventTime time.Time) []byte {
	t.Helper()
	event := marketdata.Event{
		Version: 1, StreamID: "stream-a", Type: marketdata.EventTrade,
		Symbol: "BTCUSDT-PERP", Sequence: 3, EmittedAt: eventTime,
		Trade: &marketdata.Trade{TradeID: "trade-3", Side: "buy", Price: "50000.10", Quantity: "0.001"},
	}
	payload, err := marketdata.Encode(event)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

type candleStore struct {
	mu          sync.Mutex
	applyResult candle.Candle
	accepted    bool
	listResult  []candle.Candle
	upserted    []candle.Candle
	upsertCount int
}

func (s *candleStore) ApplyTrade(context.Context, candle.Trade) (candle.Candle, bool, error) {
	return s.applyResult, s.accepted, nil
}

func (s *candleStore) Upsert(_ context.Context, item candle.Candle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upserted = append(s.upserted, item)
	s.upsertCount++
	return nil
}

func (s *candleStore) List(context.Context, candle.Query) ([]candle.Candle, error) {
	return append([]candle.Candle(nil), s.listResult...), nil
}

func (s *candleStore) FinalizeBefore(context.Context, string, time.Time) ([]candle.Candle, error) {
	return nil, nil
}

type publishedCandle struct {
	subject string
	payload []byte
}

type candlePublisher struct {
	mu       sync.Mutex
	messages []publishedCandle
	publish  func(context.Context, string, []byte) error
}

func (p *candlePublisher) Publish(ctx context.Context, subject string, payload []byte) error {
	if p.publish != nil {
		return p.publish(ctx, subject, payload)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, publishedCandle{subject: subject, payload: append([]byte(nil), payload...)})
	return nil
}

func (p *candlePublisher) Count() int { p.mu.Lock(); defer p.mu.Unlock(); return len(p.messages) }
func (p *candlePublisher) Messages() []publishedCandle {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]publishedCandle(nil), p.messages...)
}
