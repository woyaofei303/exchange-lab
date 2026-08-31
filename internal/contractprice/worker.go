package contractprice

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"exchange-lab/internal/marketdata"
	"exchange-lab/internal/trading"
)

// Store 持久化价格结果及产生该结果的观测值。
type Store interface {
	Save(context.Context, string, uint8, []Source, IndexResult, int64, int64) error
}

// Publisher 在持久化成功后发布计算出的标记价格更新。
type Publisher interface {
	Publish(context.Context, string, []byte) error
}

// Worker 将规范化行情事件转换为持久化的指数价格和标记价格。
type Worker struct {
	store     Store
	publisher Publisher
	now       func() time.Time
}

// Update 是发送给下游读取方的带版本标记价格事件。
type Update struct {
	Version    int       `json:"version"`
	Symbol     string    `json:"symbol"`
	IndexPrice string    `json:"index_price"`
	MarkPrice  string    `json:"mark_price"`
	PremiumBPS int64     `json:"premium_bps"`
	Fresh      bool      `json:"fresh"`
	AsOf       time.Time `json:"as_of"`
}

// NewWorker 校验价格工作进程的依赖项。
func NewWorker(store Store, publisher Publisher, now func() time.Time) (*Worker, error) {
	if store == nil || publisher == nil {
		return nil, fmt.Errorf("create price worker: invalid dependencies")
	}
	if now == nil {
		now = time.Now
	}
	return &Worker{store: store, publisher: publisher, now: now}, nil
}

// Handle 处理一条规范化行情，并且只在可靠保存后发布。
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
