package contractprice

import (
	"context"
	"testing"
	"time"

	"exchange-lab/internal/marketdata"
)

type workerStore struct {
	calls   int
	index   IndexResult
	mark    int64
	sources []Source
}

func (s *workerStore) Save(_ context.Context, _ string, _ uint8, sources []Source, index IndexResult, mark, _ int64) error {
	s.calls++
	s.sources = sources
	s.index = index
	s.mark = mark
	return nil
}

type workerPublisher struct {
	subject string
	payload []byte
}

func (p *workerPublisher) Publish(_ context.Context, subject string, payload []byte) error {
	p.subject = subject
	p.payload = payload
	return nil
}

func TestWorkerBuildsIndependentThreeSourceMarkPrice(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	store, publisher := &workerStore{}, &workerPublisher{}
	worker, err := NewWorker(store, publisher, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	payload, err := marketdata.Encode(marketdata.Event{Version: 1, StreamID: "s", Type: marketdata.EventTicker, Symbol: "BTCUSDT-PERP", Sequence: 1, EmittedAt: now, Ticker: &marketdata.Ticker{LastPrice: "50000.00", MarkPrice: "50000.00", IndexPrice: "50000.00", PriceChange: "0.00", Volume: "1.000"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Handle(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || len(store.sources) != 3 || store.index.PriceAtoms != 5_000_000 || store.mark != 5_000_000 {
		t.Fatalf("store = %#v", store)
	}
	if publisher.subject != "prices.BTCUSDT-PERP.mark" || len(publisher.payload) == 0 {
		t.Fatalf("publisher = %#v", publisher)
	}
}
