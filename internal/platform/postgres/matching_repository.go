package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"exchange-lab/internal/eventing"
	"exchange-lab/internal/matching"
	"exchange-lab/internal/trading"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MatchingConsumer 是订单撮合已处理事件的稳定标识。
const MatchingConsumer = "matching-v1"

// MatchingRepository 原子推进订单簿、成交、订单和 Outbox 事实。
type MatchingRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewMatchingRepository 创建受上下文限制的撮合仓储。
func NewMatchingRepository(pool *pgxpool.Pool, timeout time.Duration) *MatchingRepository {
	return &MatchingRepository{pool: pool, timeout: timeout}
}

// Process 在 Serializable 幂等事务中应用一个有序事件。
func (r *MatchingRepository) Process(ctx context.Context, envelope eventing.Envelope, streamSequence uint64, receivedAt time.Time) (int, bool, error) {
	if r == nil || r.pool == nil || r.timeout <= 0 || streamSequence == 0 || streamSequence > math.MaxInt64 {
		return 0, false, matching.ErrInvalidOrder
	}
	orderEvent, err := matching.DecodeOrderEvent(envelope)
	if err != nil {
		return 0, false, err
	}
	operation, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	tx, err := r.pool.BeginTx(operation, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return 0, false, fmt.Errorf("match order event: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	inserted, err := insertProcessedEvent(operation, tx, MatchingConsumer, envelope, receivedAt)
	if err != nil {
		return 0, false, err
	}
	if !inserted {
		if err := tx.Commit(operation); err != nil {
			return 0, false, fmt.Errorf("match order replay: commit: %w", err)
		}
		return 0, false, nil
	}
	lastStream, lastTrade, err := lockMatchingOffset(operation, tx, receivedAt)
	if err != nil {
		return 0, false, err
	}
	if int64(streamSequence) <= lastStream {
		return 0, false, fmt.Errorf("match order event: out-of-order stream sequence")
	}
	snapshot, err := loadMatchingBook(operation, tx, orderEvent.Symbol)
	if err != nil {
		return 0, false, err
	}
	engine, err := matching.NewEngineFromSnapshot(snapshot, lastStream, lastTrade)
	if err != nil {
		return 0, false, err
	}
	var result matching.Result
	var canceled *matching.SnapshotOrder
	if envelope.Type == "orders.accepted" {
		var priceScale, quantityScale uint8
		if err := tx.QueryRow(operation, `SELECT price_scale,quantity_scale FROM instruments WHERE symbol=$1 AND status='active'`, orderEvent.Symbol).Scan(&priceScale, &quantityScale); err != nil {
			return 0, false, fmt.Errorf("match order event: instrument: %w", err)
		}
		price, err := trading.ParsePrice(orderEvent.Price, priceScale)
		if err != nil {
			return 0, false, matching.ErrInvalidOrder
		}
		quantity, err := trading.ParseQuantity(orderEvent.Quantity, quantityScale)
		if err != nil {
			return 0, false, matching.ErrInvalidOrder
		}
		result, err = engine.Add(matching.LimitOrder{ID: orderEvent.OrderID, UserID: orderEvent.UserID, Side: orderEvent.Side, PriceAtoms: price.Atoms(), QuantityAtoms: quantity.Atoms(), Sequence: int64(streamSequence)})
		if err != nil {
			return 0, false, err
		}
	} else {
		item, cancelErr := engine.Cancel(orderEvent.OrderID)
		if cancelErr == nil {
			canceled = &item
		} else if !errors.Is(cancelErr, matching.ErrNotFound) {
			return 0, false, cancelErr
		}
	}
	if err := replaceMatchingBook(operation, tx, orderEvent.Symbol, engine.Snapshot(), receivedAt); err != nil {
		return 0, false, err
	}
	if canceled != nil {
		if err := appendMatchingEvent(operation, tx, envelope.ID, canceled.ID, "canceled", canceled.RemainingAtoms, "", receivedAt); err != nil {
			return 0, false, err
		}
	}
	for _, event := range result.Events {
		if _, err := tx.Exec(operation, `UPDATE orders SET status=$2,remaining_atoms=$3,updated_at=$4 WHERE id=$1`, event.OrderID, event.Status, event.RemainingAtoms, receivedAt.UTC()); err != nil {
			return 0, false, fmt.Errorf("match order event: update order: %w", err)
		}
		if err := appendMatchingEvent(operation, tx, envelope.ID, event.OrderID, event.Type, event.RemainingAtoms, event.TradeID, receivedAt); err != nil {
			return 0, false, err
		}
	}
	for _, trade := range result.Trades {
		if err := insertTrade(operation, tx, envelope, orderEvent.Symbol, trade, receivedAt); err != nil {
			return 0, false, err
		}
	}
	if _, err := tx.Exec(operation, `UPDATE matching_offsets SET last_stream_sequence=$2,last_trade_sequence=$3,updated_at=$4 WHERE consumer_name=$1`, MatchingConsumer, int64(streamSequence), engine.TradeSequence(), receivedAt.UTC()); err != nil {
		return 0, false, fmt.Errorf("match order event: update offset: %w", err)
	}
	if _, err := tx.Exec(operation, `INSERT INTO event_receipts(consumer_name,event_id,aggregate_id,correlation_id,received_at) VALUES ($1,$2,$3,$4,$5)`, MatchingConsumer, envelope.ID, envelope.AggregateID, envelope.CorrelationID, receivedAt.UTC()); err != nil {
		return 0, false, fmt.Errorf("match order event: receipt: %w", err)
	}
	if err := tx.Commit(operation); err != nil {
		return 0, false, fmt.Errorf("match order event: commit: %w", err)
	}
	return len(result.Trades), true, nil
}

func insertProcessedEvent(ctx context.Context, tx pgx.Tx, consumer string, envelope eventing.Envelope, at time.Time) (bool, error) {
	result, err := tx.Exec(ctx, `INSERT INTO processed_events(consumer_name,event_id,subject,processed_at) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, consumer, envelope.ID, envelope.Type, at.UTC())
	if err != nil {
		return false, fmt.Errorf("match order event: dedupe: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func lockMatchingOffset(ctx context.Context, tx pgx.Tx, at time.Time) (int64, int64, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO matching_offsets(consumer_name,updated_at) VALUES ($1,$2) ON CONFLICT DO NOTHING`, MatchingConsumer, at.UTC()); err != nil {
		return 0, 0, fmt.Errorf("match order event: create offset: %w", err)
	}
	var stream, trade int64
	if err := tx.QueryRow(ctx, `SELECT last_stream_sequence,last_trade_sequence FROM matching_offsets WHERE consumer_name=$1 FOR UPDATE`, MatchingConsumer).Scan(&stream, &trade); err != nil {
		return 0, 0, fmt.Errorf("match order event: lock offset: %w", err)
	}
	return stream, trade, nil
}

func loadMatchingBook(ctx context.Context, tx pgx.Tx, symbol string) (matching.Snapshot, error) {
	rows, err := tx.Query(ctx, `SELECT order_id,user_id,side,price_atoms,quantity_atoms,remaining_atoms,order_sequence FROM matching_book_orders WHERE symbol=$1 ORDER BY order_sequence`, symbol)
	if err != nil {
		return matching.Snapshot{}, fmt.Errorf("load matching book: %w", err)
	}
	defer rows.Close()
	var snapshot matching.Snapshot
	for rows.Next() {
		var item matching.SnapshotOrder
		if err := rows.Scan(&item.ID, &item.UserID, &item.Side, &item.PriceAtoms, &item.QuantityAtoms, &item.RemainingAtoms, &item.Sequence); err != nil {
			return matching.Snapshot{}, fmt.Errorf("load matching book: scan: %w", err)
		}
		item.Status = trading.OrderStatusOpen
		if item.Side == trading.SideBuy {
			snapshot.Bids = append(snapshot.Bids, item)
		} else {
			snapshot.Asks = append(snapshot.Asks, item)
		}
	}
	return snapshot, rows.Err()
}

func replaceMatchingBook(ctx context.Context, tx pgx.Tx, symbol string, snapshot matching.Snapshot, at time.Time) error {
	if _, err := tx.Exec(ctx, `DELETE FROM matching_book_orders WHERE symbol=$1`, symbol); err != nil {
		return fmt.Errorf("replace matching book: clear: %w", err)
	}
	items := append(append([]matching.SnapshotOrder(nil), snapshot.Bids...), snapshot.Asks...)
	for _, item := range items {
		if _, err := tx.Exec(ctx, `INSERT INTO matching_book_orders(order_id,symbol,user_id,side,price_atoms,quantity_atoms,remaining_atoms,order_sequence,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, item.ID, symbol, item.UserID, item.Side, item.PriceAtoms, item.QuantityAtoms, item.RemainingAtoms, item.Sequence, at.UTC()); err != nil {
			return fmt.Errorf("replace matching book: insert: %w", err)
		}
	}
	return nil
}

func appendMatchingEvent(ctx context.Context, tx pgx.Tx, sourceID, orderID, eventType string, remaining int64, tradeID string, at time.Time) error {
	var optionalTrade any
	if tradeID != "" {
		optionalTrade = tradeID
	}
	_, err := tx.Exec(ctx, `INSERT INTO matching_events(source_event_id,order_id,event_type,remaining_atoms,trade_id,occurred_at) VALUES ($1,$2,$3,$4,$5,$6)`, sourceID, orderID, eventType, remaining, optionalTrade, at.UTC())
	if err != nil {
		return fmt.Errorf("append matching event: %w", err)
	}
	return nil
}

func insertTrade(ctx context.Context, tx pgx.Tx, source eventing.Envelope, symbol string, trade matching.Trade, at time.Time) error {
	var priceScale, quantityScale uint8
	if err := tx.QueryRow(ctx, `SELECT price_scale,quantity_scale FROM instruments WHERE symbol=$1`, symbol).Scan(&priceScale, &quantityScale); err != nil {
		return fmt.Errorf("insert trade: scales: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO contract_trades(id,symbol,maker_order_id,taker_order_id,maker_user_id,taker_user_id,taker_side,price_atoms,price_scale,quantity_atoms,quantity_scale,matching_sequence,occurred_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, trade.ID, symbol, trade.MakerOrderID, trade.TakerOrderID, trade.MakerUserID, trade.TakerUserID, trade.TakerSide, trade.PriceAtoms, priceScale, trade.QuantityAtoms, quantityScale, trade.Sequence, at.UTC()); err != nil {
		return fmt.Errorf("insert trade: %w", err)
	}
	price, _ := trading.FormatDecimal(trade.PriceAtoms, priceScale)
	quantity, _ := trading.FormatDecimal(trade.QuantityAtoms, quantityScale)
	payload, _ := json.Marshal(map[string]any{"version": 1, "trade_id": trade.ID, "symbol": symbol, "maker_order_id": trade.MakerOrderID, "taker_order_id": trade.TakerOrderID, "maker_user_id": trade.MakerUserID, "taker_user_id": trade.TakerUserID, "taker_side": trade.TakerSide, "price": price, "quantity": quantity, "occurred_at": at.UTC().Format(time.RFC3339Nano)})
	outboxID, err := randomBusinessID("out_")
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,subject,correlation_id,payload,available_at,created_at) VALUES ($1,'trade',$2,'trades.executed',$3,$4,$5,$5)`, outboxID, trade.ID, source.CorrelationID, payload, at.UTC()); err != nil {
		return fmt.Errorf("insert trade outbox: %w", err)
	}
	return nil
}
