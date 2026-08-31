package postgres

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/woyaofei303/exchange-lab/internal/candle"
)

// CandleRepository owns trade deduplication and durable candle projections.
type CandleRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// CandleQuery aliases the domain query accepted by candle storage.
type CandleQuery = candle.Query

var candleRepositorySymbolPattern = regexp.MustCompile(`^[A-Z0-9]{4,24}-PERP$`)

// NewCandleRepository constructs a context-bounded candle repository.
func NewCandleRepository(pool *pgxpool.Pool, timeout time.Duration) *CandleRepository {
	return &CandleRepository{pool: pool, timeout: timeout}
}

// ApplyTrade durably deduplicates a normalized trade and updates its 1m candle
// in the same transaction. accepted is false for a previously stored event.
func (r *CandleRepository) ApplyTrade(ctx context.Context, trade candle.Trade) (result candle.Candle, accepted bool, resultErr error) {
	if r == nil || r.pool == nil || r.timeout <= 0 {
		return candle.Candle{}, false, fmt.Errorf("apply candle trade: invalid repository configuration")
	}
	if _, err := candle.ApplyTrade(nil, trade, candle.OneMinute); err != nil {
		return candle.Candle{}, false, err
	}
	queryContext, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	tx, err := r.pool.BeginTx(queryContext, pgx.TxOptions{})
	if err != nil {
		return candle.Candle{}, false, fmt.Errorf("apply candle trade: begin transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback(queryContext)
		}
	}()

	tag, err := tx.Exec(queryContext, `
		INSERT INTO market_trades (
			stream_id, trade_id, symbol, sequence, event_time,
			price_scale, quantity_scale, price_atoms, quantity_atoms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT DO NOTHING
	`, trade.StreamID, trade.TradeID, trade.Symbol, trade.Sequence, trade.EventTime.UTC(),
		trade.PriceScale, trade.QuantityScale, trade.PriceAtoms, trade.QuantityAtoms)
	if err != nil {
		return candle.Candle{}, false, fmt.Errorf("apply candle trade: store event: %w", err)
	}
	bucket := candle.BucketStart(trade.EventTime, candle.OneMinute)
	if tag.RowsAffected() == 0 {
		result, err = queryOneCandle(queryContext, tx, trade.Symbol, candle.OneMinute, bucket)
		if err != nil {
			return candle.Candle{}, false, fmt.Errorf("apply candle trade: read duplicate candle: %w", err)
		}
		if err := tx.Commit(queryContext); err != nil {
			return candle.Candle{}, false, fmt.Errorf("apply candle trade: commit duplicate: %w", err)
		}
		return result, false, nil
	}

	row := tx.QueryRow(queryContext, `
		INSERT INTO candles (
			symbol, interval_seconds, open_time, price_scale, quantity_scale,
			open_atoms, high_atoms, low_atoms, close_atoms, volume_atoms,
			trade_count, is_final, first_trade_time, last_trade_time,
			first_sequence, last_sequence
		) VALUES ($1, 60, $2, $3, $4, $5, $5, $5, $5, $6, 1, false, $7, $7, $8, $8)
		ON CONFLICT (symbol, interval_seconds, open_time) DO UPDATE SET
			open_atoms = CASE
				WHEN EXCLUDED.first_trade_time < candles.first_trade_time
				  OR (EXCLUDED.first_trade_time = candles.first_trade_time AND EXCLUDED.first_sequence < candles.first_sequence)
				THEN EXCLUDED.open_atoms ELSE candles.open_atoms END,
			high_atoms = GREATEST(candles.high_atoms, EXCLUDED.high_atoms),
			low_atoms = LEAST(candles.low_atoms, EXCLUDED.low_atoms),
			close_atoms = CASE
				WHEN EXCLUDED.last_trade_time > candles.last_trade_time
				  OR (EXCLUDED.last_trade_time = candles.last_trade_time AND EXCLUDED.last_sequence > candles.last_sequence)
				THEN EXCLUDED.close_atoms ELSE candles.close_atoms END,
			volume_atoms = candles.volume_atoms + EXCLUDED.volume_atoms,
			trade_count = candles.trade_count + 1,
			first_trade_time = LEAST(candles.first_trade_time, EXCLUDED.first_trade_time),
			last_trade_time = GREATEST(candles.last_trade_time, EXCLUDED.last_trade_time),
			first_sequence = CASE
				WHEN EXCLUDED.first_trade_time < candles.first_trade_time
				  OR (EXCLUDED.first_trade_time = candles.first_trade_time AND EXCLUDED.first_sequence < candles.first_sequence)
				THEN EXCLUDED.first_sequence ELSE candles.first_sequence END,
			last_sequence = CASE
				WHEN EXCLUDED.last_trade_time > candles.last_trade_time
				  OR (EXCLUDED.last_trade_time = candles.last_trade_time AND EXCLUDED.last_sequence > candles.last_sequence)
				THEN EXCLUDED.last_sequence ELSE candles.last_sequence END,
			updated_at = now()
		WHERE candles.price_scale = EXCLUDED.price_scale
		  AND candles.quantity_scale = EXCLUDED.quantity_scale
		RETURNING `+candleColumns,
		trade.Symbol, bucket, trade.PriceScale, trade.QuantityScale,
		trade.PriceAtoms, trade.QuantityAtoms, trade.EventTime.UTC(), trade.Sequence)
	result, err = scanCandle(row)
	if err != nil {
		return candle.Candle{}, false, fmt.Errorf("apply candle trade: upsert candle: %w", err)
	}
	if err := tx.Commit(queryContext); err != nil {
		return candle.Candle{}, false, fmt.Errorf("apply candle trade: commit: %w", err)
	}
	return result, true, nil
}

// Upsert replaces one validated derived candle projection.
func (r *CandleRepository) Upsert(ctx context.Context, item candle.Candle) error {
	if r == nil || r.pool == nil || r.timeout <= 0 {
		return fmt.Errorf("upsert candle: invalid repository configuration")
	}
	if _, err := item.View(); err != nil {
		return err
	}
	queryContext, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	_, err := r.pool.Exec(queryContext, `
		INSERT INTO candles (
			symbol, interval_seconds, open_time, price_scale, quantity_scale,
			open_atoms, high_atoms, low_atoms, close_atoms, volume_atoms,
			trade_count, is_final, first_trade_time, last_trade_time,
			first_sequence, last_sequence
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (symbol, interval_seconds, open_time) DO UPDATE SET
			price_scale = EXCLUDED.price_scale,
			quantity_scale = EXCLUDED.quantity_scale,
			open_atoms = EXCLUDED.open_atoms,
			high_atoms = EXCLUDED.high_atoms,
			low_atoms = EXCLUDED.low_atoms,
			close_atoms = EXCLUDED.close_atoms,
			volume_atoms = EXCLUDED.volume_atoms,
			trade_count = EXCLUDED.trade_count,
			is_final = EXCLUDED.is_final,
			first_trade_time = EXCLUDED.first_trade_time,
			last_trade_time = EXCLUDED.last_trade_time,
			first_sequence = EXCLUDED.first_sequence,
			last_sequence = EXCLUDED.last_sequence,
			updated_at = now()
	`, item.Symbol, int32(item.Interval), item.OpenTime.UTC(), item.PriceScale, item.QuantityScale,
		item.OpenAtoms, item.HighAtoms, item.LowAtoms, item.CloseAtoms, item.VolumeAtoms,
		item.TradeCount, item.Final, item.FirstTradeTime.UTC(), item.LastTradeTime.UTC(),
		item.FirstSequence, item.LastSequence)
	if err != nil {
		return fmt.Errorf("upsert candle: %w", err)
	}
	return nil
}

// List returns a bounded, time-ordered half-open candle range.
func (r *CandleRepository) List(ctx context.Context, query candle.Query) ([]candle.Candle, error) {
	if r == nil || r.pool == nil || r.timeout <= 0 {
		return nil, fmt.Errorf("list candles: invalid repository configuration")
	}
	if !candleRepositorySymbolPattern.MatchString(query.Symbol) || query.Interval.String() == "" || query.Start.IsZero() || !query.Start.Before(query.End) || query.Limit < 1 || query.Limit > 1000 {
		return nil, fmt.Errorf("list candles: invalid query")
	}
	queryContext, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	rows, err := r.pool.Query(queryContext, `
		SELECT `+candleColumns+`
		FROM candles
		WHERE symbol = $1 AND interval_seconds = $2
		  AND open_time >= $3 AND open_time < $4
		ORDER BY open_time
		LIMIT $5
	`, query.Symbol, int32(query.Interval), query.Start.UTC(), query.End.UTC(), query.Limit)
	if err != nil {
		return nil, fmt.Errorf("list candles: %w", err)
	}
	defer rows.Close()
	result := make([]candle.Candle, 0)
	for rows.Next() {
		item, err := scanCandle(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list candle rows: %w", err)
	}
	return result, nil
}

// FinalizeBefore marks complete buckets and returns every changed candle.
func (r *CandleRepository) FinalizeBefore(ctx context.Context, symbol string, watermark time.Time) ([]candle.Candle, error) {
	if r == nil || r.pool == nil || r.timeout <= 0 || !candleRepositorySymbolPattern.MatchString(symbol) || watermark.IsZero() {
		return nil, fmt.Errorf("finalize candles: invalid input")
	}
	queryContext, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	rows, err := r.pool.Query(queryContext, `
		UPDATE candles
		SET is_final = true, updated_at = now()
		WHERE symbol = $1 AND is_final = false
		  AND open_time + make_interval(secs => interval_seconds) <= $2
		RETURNING `+candleColumns,
		symbol, watermark.UTC())
	if err != nil {
		return nil, fmt.Errorf("finalize candles: %w", err)
	}
	defer rows.Close()
	result := make([]candle.Candle, 0)
	for rows.Next() {
		item, err := scanCandle(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("finalize candle rows: %w", err)
	}
	return result, nil
}

const candleColumns = `
	symbol, interval_seconds, open_time, price_scale, quantity_scale,
	open_atoms, high_atoms, low_atoms, close_atoms, volume_atoms,
	trade_count, is_final, first_trade_time, last_trade_time,
	first_sequence, last_sequence
`

func queryOneCandle(ctx context.Context, queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, symbol string, interval candle.Interval, openTime time.Time) (candle.Candle, error) {
	return scanCandle(queryer.QueryRow(ctx, `
		SELECT `+candleColumns+`
		FROM candles WHERE symbol = $1 AND interval_seconds = $2 AND open_time = $3
	`, symbol, int32(interval), openTime.UTC()))
}

func scanCandle(row rowScanner) (candle.Candle, error) {
	var (
		item                        candle.Candle
		interval                    int32
		firstSequence, lastSequence int64
	)
	if err := row.Scan(
		&item.Symbol, &interval, &item.OpenTime, &item.PriceScale, &item.QuantityScale,
		&item.OpenAtoms, &item.HighAtoms, &item.LowAtoms, &item.CloseAtoms, &item.VolumeAtoms,
		&item.TradeCount, &item.Final, &item.FirstTradeTime, &item.LastTradeTime,
		&firstSequence, &lastSequence,
	); err != nil {
		return candle.Candle{}, fmt.Errorf("scan candle: %w", err)
	}
	item.Interval = candle.Interval(interval)
	if firstSequence < 0 || lastSequence < 0 {
		return candle.Candle{}, fmt.Errorf("scan candle: invalid sequence")
	}
	item.FirstSequence = uint64(firstSequence)
	item.LastSequence = uint64(lastSequence)
	if _, err := item.View(); err != nil {
		return candle.Candle{}, fmt.Errorf("scan candle: %w", err)
	}
	return item, nil
}
