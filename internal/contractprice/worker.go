package contractprice

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/marketdata"
	"github.com/woyaofei303/exchange-lab/internal/trading"
)

// Store persists a price result together with the observations that produced it.
type Store interface {
	Save(context.Context, string, uint8, []Source, IndexResult, int64, int64) error
}

// Publisher emits a derived mark-price update after persistence succeeds.
type Publisher interface {
	Publish(context.Context, string, []byte) error
}

// Worker converts normalized ticker events into persisted index and mark prices.
type Worker struct {
	store     Store
	publisher Publisher
	now       func() time.Time
}

// Update is the versioned mark-price event sent to downstream readers.
type Update struct {
	Version    int       `json:"version"`
	Symbol     string    `json:"symbol"`
	IndexPrice string    `json:"index_price"`
	MarkPrice  string    `json:"mark_price"`
	PremiumBPS int64     `json:"premium_bps"`
	Fresh      bool      `json:"fresh"`
	AsOf       time.Time `json:"as_of"`
}

// NewWorker validates the price worker dependencies.
func NewWorker(store Store, publisher Publisher, now func() time.Time) (*Worker, error) {
	if store == nil || publisher == nil {
		return nil, fmt.Errorf("create price worker: invalid dependencies")
	}
	if now == nil {
		now = time.Now
	}
	return &Worker{store: store, publisher: publisher, now: now}, nil
}

// Handle processes one normalized ticker and publishes only after durable save.
func (w *Worker) Handle(ctx context.Context, payload []byte) error {
	event, err := marketdata.Decode(payload)
	if err != nil || event.Type != marketdata.EventTicker || event.Ticker == nil {
		return fmt.Errorf("process price ticker: invalid event")
	}
	last, err := trading.ParsePrice(event.Ticker.LastPrice, 2)
	if err != nil {
		return fmt.Errorf("process price ticker: invalid last price")
	}
	now := w.now().UTC()
	sources := []Source{
		{Name: "sim-coinbase", PriceAtoms: last.Atoms() - 10, Weight: 1, UpdatedAt: event.EmittedAt.UTC()},
		{Name: "sim-kraken", PriceAtoms: last.Atoms(), Weight: 1, UpdatedAt: event.EmittedAt.UTC()},
		{Name: "sim-okx", PriceAtoms: last.Atoms() + 10, Weight: 1, UpdatedAt: event.EmittedAt.UTC()},
	}
	index, err := ComputeIndex(sources, IndexConfig{Now: now, MaxAge: 10 * time.Second, MaxDeviationBPS: 100, MinimumSources: 3})
	if err != nil {
		return fmt.Errorf("process price ticker: compute index: %w", err)
	}
	mark, applied, err := ComputeMark(index.PriceAtoms, 0, 100)
	if err != nil {
		return fmt.Errorf("process price ticker: compute mark: %w", err)
	}
	if err := w.store.Save(ctx, event.Symbol, 2, sources, index, mark, applied); err != nil {
		return err
	}
	indexDecimal, _ := trading.FormatDecimal(index.PriceAtoms, 2)
	markDecimal, _ := trading.FormatDecimal(mark, 2)
	encoded, err := json.Marshal(Update{Version: 1, Symbol: event.Symbol, IndexPrice: indexDecimal, MarkPrice: markDecimal, PremiumBPS: applied, Fresh: index.Fresh, AsOf: index.AsOf})
	if err != nil {
		return fmt.Errorf("encode price update")
	}
	if err := w.publisher.Publish(ctx, "prices."+event.Symbol+".mark", encoded); err != nil {
		return fmt.Errorf("publish price update")
	}
	return nil
}
