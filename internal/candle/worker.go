package candle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"exchange-lab/internal/marketdata"
	"exchange-lab/internal/trading"
)

// Query 定义有界的左闭右开 K 线历史范围。
type Query struct {
	Symbol   string
	Interval Interval
	Start    time.Time
	End      time.Time
	Limit    int
}

// Store 以原子方式对成交去重，并持久化基础和聚合 K 线。
type Store interface {
	ApplyTrade(context.Context, Trade) (Candle, bool, error)
	Upsert(context.Context, Candle) error
	List(context.Context, Query) ([]Candle, error)
	FinalizeBefore(context.Context, string, time.Time) ([]Candle, error)
}

// Publisher 在持久化后发布 K 线更新。
type Publisher interface {
	Publish(context.Context, string, []byte) error
}

// WorkerConfig 定义精确的来源精度和发布超时时间。
type WorkerConfig struct {
	PriceScale       uint8
	QuantityScale    uint8
	OperationTimeout time.Duration
}

// Worker 消费规范化成交并维护持久化 K 线投影。
type Worker struct {
	store     Store
	publisher Publisher
	config    WorkerConfig
}

// Update 是带版本的实时 K 线信封。
type Update struct {
	Version int    `json:"version"`
	Type    string `json:"type"`
	Data    View   `json:"data"`
}

// NewWorker 校验 K 线持久化、发布和精度配置。
func NewWorker(store Store, publisher Publisher, config WorkerConfig) (*Worker, error) {
	if store == nil || publisher == nil || config.PriceScale > 18 || config.QuantityScale > 18 || config.OperationTimeout <= 0 {
		return nil, fmt.Errorf("create candle worker: invalid configuration")
	}
	return &Worker{store: store, publisher: publisher, config: config}, nil
}

// Handle 幂等应用一笔成交、结束旧时间桶并重建聚合 K 线。
func (w *Worker) Handle(ctx context.Context, payload []byte) error {
	event, err := marketdata.Decode(payload)
	if err != nil || event.Type != marketdata.EventTrade || event.Trade == nil {
		return fmt.Errorf("handle candle event: invalid trade event")
	}
	price, err := trading.ParsePrice(event.Trade.Price, w.config.PriceScale)
	if err != nil {
		return fmt.Errorf("handle candle event: invalid price")
	}
	quantity, err := trading.ParseQuantity(event.Trade.Quantity, w.config.QuantityScale)
	if err != nil {
		return fmt.Errorf("handle candle event: invalid quantity")
	}
	trade := Trade{
		StreamID: event.StreamID, TradeID: event.Trade.TradeID, Symbol: event.Symbol,
		Sequence: event.Sequence, EventTime: event.EmittedAt,
		PriceAtoms: price.Atoms(), PriceScale: price.Scale(), QuantityAtoms: quantity.Atoms(), QuantityScale: quantity.Scale(),
	}
	oneMinute, accepted, err := w.store.ApplyTrade(ctx, trade)
	if err != nil {
		return fmt.Errorf("handle candle event: store trade")
	}
	if !accepted {
		return nil
	}
	finalized, err := w.store.FinalizeBefore(ctx, event.Symbol, oneMinute.OpenTime)
	if err != nil {
		return fmt.Errorf("handle candle event: finalize candles")
	}
	for _, item := range finalized {
		if err := w.publish(ctx, item); err != nil {
			return err
		}
	}
	if err := w.publish(ctx, oneMinute); err != nil {
		return err
	}
	for _, target := range []Interval{FiveMinutes, OneHour} {
		if _, err := w.rebuildRollup(ctx, event.Symbol, oneMinute.OpenTime, target, true); err != nil {
			return err
		}
	}
	return nil
}

// Repair 填补有界的一分钟周期范围，并重建所有受影响的聚合 K 线。
func (w *Worker) Repair(ctx context.Context, symbol string, start, end time.Time) error {
	source, err := w.store.List(ctx, Query{Symbol: symbol, Interval: OneMinute, Start: start, End: end, Limit: 1000})
	if err != nil {
		return fmt.Errorf("repair candles: list source")
	}
	repaired, err := RepairGaps(source, start, end, OneMinute)
	if err != nil {
		return err
	}
	for _, item := range repaired {
		if err := w.store.Upsert(ctx, item); err != nil {
			return fmt.Errorf("repair candles: upsert minute")
		}
		if err := w.publish(ctx, item); err != nil {
			return err
		}
	}
	for _, target := range []Interval{FiveMinutes, OneHour} {
		buckets := make(map[int64]time.Time)
		for _, item := range repaired {
			bucket := BucketStart(item.OpenTime, target)
			buckets[bucket.Unix()] = bucket
		}
		starts := make([]time.Time, 0, len(buckets))
		for _, bucket := range buckets {
			starts = append(starts, bucket)
		}
		sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
		for _, bucket := range starts {
			if _, err := w.rebuildRollup(ctx, symbol, bucket, target, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Worker) rebuildRollup(ctx context.Context, symbol string, eventTime time.Time, target Interval, publish bool) (Candle, error) {
	start := BucketStart(eventTime, target)
	source, err := w.store.List(ctx, Query{Symbol: symbol, Interval: OneMinute, Start: start, End: start.Add(target.Duration()), Limit: int(target/OneMinute) + 1})
	if err != nil {
		return Candle{}, fmt.Errorf("rebuild candle rollup: list source")
	}
	if len(source) == 0 {
		return Candle{}, fmt.Errorf("rebuild candle rollup: source unavailable")
	}
	rollup, err := Rollup(source, target)
	if err != nil {
		return Candle{}, err
	}
	if err := w.store.Upsert(ctx, rollup); err != nil {
		return Candle{}, fmt.Errorf("rebuild candle rollup: upsert")
	}
	if publish {
		if err := w.publish(ctx, rollup); err != nil {
			return Candle{}, err
		}
	}
	return rollup, nil
}

func (w *Worker) publish(ctx context.Context, item Candle) error {
	view, err := item.View()
	if err != nil {
		return fmt.Errorf("publish candle: invalid candle")
	}
	update := Update{Version: 1, Type: "candle", Data: view}
	payload, err := EncodeUpdate(update)
	if err != nil {
		return fmt.Errorf("publish candle: encode update")
	}
	operationContext, cancel := context.WithTimeout(ctx, w.config.OperationTimeout)
	defer cancel()
	subject, err := UpdateSubject(update)
	if err != nil {
		return err
	}
	if err := w.publisher.Publish(operationContext, subject, payload); err != nil {
		return fmt.Errorf("publish candle update")
	}
	return nil
}

// EncodeUpdate 校验并序列化一条实时 K 线更新。
func EncodeUpdate(update Update) ([]byte, error) {
	if err := validateUpdate(update); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(update)
	if err != nil {
		return nil, fmt.Errorf("encode candle update")
	}
	return payload, nil
}

// DecodeUpdate 严格解析一条有界的实时 K 线更新。
func DecodeUpdate(payload []byte) (Update, error) {
	if len(payload) == 0 || len(payload) > 1<<20 {
		return Update{}, fmt.Errorf("decode candle update: invalid payload size")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var update Update
	if err := decoder.Decode(&update); err != nil {
		return Update{}, fmt.Errorf("decode candle update: invalid JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Update{}, fmt.Errorf("decode candle update: trailing JSON")
	}
	if err := validateUpdate(update); err != nil {
		return Update{}, err
	}
	return update, nil
}

// UpdateSubject 生成稳定的交易对和周期 NATS 主题。
func UpdateSubject(update Update) (string, error) {
	if err := validateUpdate(update); err != nil {
		return "", err
	}
	return "candles." + update.Data.Symbol + "." + update.Data.Interval, nil
}

func validateUpdate(update Update) error {
	if update.Version != 1 || update.Type != "candle" || !candleSymbolPattern.MatchString(update.Data.Symbol) {
		return fmt.Errorf("validate candle update: invalid envelope")
	}
	interval, err := ParseInterval(update.Data.Interval)
	if err != nil || update.Data.OpenTime.IsZero() || update.Data.CloseTime.IsZero() || !update.Data.CloseTime.Equal(update.Data.OpenTime.Add(interval.Duration())) || update.Data.TradeCount < 0 {
		return fmt.Errorf("validate candle update: invalid data")
	}
	for _, value := range []string{update.Data.Open, update.Data.High, update.Data.Low, update.Data.Close, update.Data.Volume} {
		if value == "" {
			return fmt.Errorf("validate candle update: invalid decimal")
		}
	}
	return nil
}
