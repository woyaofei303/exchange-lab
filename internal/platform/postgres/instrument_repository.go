package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/woyaofei303/exchange-lab/internal/trading"
)

// InstrumentRepository stores contract filters as integer atoms plus explicit
// scales. PostgreSQL is the source of truth for this data.
type InstrumentRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewInstrumentRepository constructs a context-bounded instrument repository.
func NewInstrumentRepository(pool *pgxpool.Pool, timeout time.Duration) *InstrumentRepository {
	return &InstrumentRepository{pool: pool, timeout: timeout}
}

// Instruments implements market.InstrumentReader without coupling this
// persistence package to the transport-facing composite implementation.
func (r *InstrumentRepository) Instruments(ctx context.Context, symbol string) ([]trading.Instrument, error) {
	return r.List(ctx, symbol)
}

// List returns active contracts, optionally filtered by an exact symbol.
func (r *InstrumentRepository) List(ctx context.Context, symbol string) ([]trading.Instrument, error) {
	if r == nil || r.pool == nil || r.timeout <= 0 {
		return nil, fmt.Errorf("list instruments: invalid repository configuration")
	}
	queryContext, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	query := `
		SELECT symbol, base_asset, quote_asset, base_scale, quote_scale,
		       price_scale, quantity_scale, price_step_atoms,
		       quantity_step_atoms, min_notional_atoms
		FROM instruments
		WHERE status = 'active'
		ORDER BY symbol
	`
	arguments := []any(nil)
	if symbol != "" {
		query = `
			SELECT symbol, base_asset, quote_asset, base_scale, quote_scale,
			       price_scale, quantity_scale, price_step_atoms,
			       quantity_step_atoms, min_notional_atoms
			FROM instruments
			WHERE status = 'active' AND symbol = $1
			ORDER BY symbol
		`
		arguments = []any{symbol}
	}
	rows, err := r.pool.Query(queryContext, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list instruments: %w", err)
	}
	defer rows.Close()

	result := make([]trading.Instrument, 0)
	for rows.Next() {
		instrument, err := scanInstrument(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, instrument)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list instruments rows: %w", err)
	}
	return result, nil
}

// Save inserts or updates one validated contract configuration.
func (r *InstrumentRepository) Save(ctx context.Context, instrument trading.Instrument) error {
	if r == nil || r.pool == nil || r.timeout <= 0 {
		return fmt.Errorf("save instrument: invalid repository configuration")
	}
	if instrument.Symbol() == "" || instrument.PriceStep().Atoms() <= 0 || instrument.QuantityStep().Atoms() <= 0 || instrument.MinNotional().Atoms() <= 0 {
		return fmt.Errorf("save instrument: invalid instrument")
	}
	queryContext, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	_, err := r.pool.Exec(queryContext, `
		INSERT INTO instruments (
			symbol, base_asset, quote_asset, base_scale, quote_scale,
			price_scale, quantity_scale, price_step_atoms,
			quantity_step_atoms, min_notional_atoms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (symbol) DO UPDATE SET
			base_asset = EXCLUDED.base_asset,
			quote_asset = EXCLUDED.quote_asset,
			base_scale = EXCLUDED.base_scale,
			quote_scale = EXCLUDED.quote_scale,
			price_scale = EXCLUDED.price_scale,
			quantity_scale = EXCLUDED.quantity_scale,
			price_step_atoms = EXCLUDED.price_step_atoms,
			quantity_step_atoms = EXCLUDED.quantity_step_atoms,
			min_notional_atoms = EXCLUDED.min_notional_atoms,
			status = 'active',
			updated_at = now()
	`,
		instrument.Symbol(),
		instrument.Base().Code(),
		instrument.Quote().Code(),
		instrument.Base().Scale(),
		instrument.Quote().Scale(),
		instrument.PriceScale(),
		instrument.QuantityScale(),
		instrument.PriceStep().Atoms(),
		instrument.QuantityStep().Atoms(),
		instrument.MinNotional().Atoms(),
	)
	if err != nil {
		return fmt.Errorf("save instrument: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanInstrument(row rowScanner) (trading.Instrument, error) {
	var (
		symbol                            string
		baseCode, quoteCode               string
		baseScale, quoteScale             uint8
		priceScale, quantityScale         uint8
		priceStepAtoms, quantityStepAtoms int64
		minNotionalAtoms                  int64
	)
	if err := row.Scan(
		&symbol,
		&baseCode,
		&quoteCode,
		&baseScale,
		&quoteScale,
		&priceScale,
		&quantityScale,
		&priceStepAtoms,
		&quantityStepAtoms,
		&minNotionalAtoms,
	); err != nil {
		if err == pgx.ErrNoRows {
			return trading.Instrument{}, err
		}
		return trading.Instrument{}, fmt.Errorf("scan instrument: %w", err)
	}
	base, err := trading.NewAsset(baseCode, baseScale)
	if err != nil {
		return trading.Instrument{}, fmt.Errorf("scan instrument base: %w", err)
	}
	quote, err := trading.NewAsset(quoteCode, quoteScale)
	if err != nil {
		return trading.Instrument{}, fmt.Errorf("scan instrument quote: %w", err)
	}
	priceStep, err := trading.FormatDecimal(priceStepAtoms, priceScale)
	if err != nil {
		return trading.Instrument{}, fmt.Errorf("scan instrument price step: %w", err)
	}
	quantityStep, err := trading.FormatDecimal(quantityStepAtoms, quantityScale)
	if err != nil {
		return trading.Instrument{}, fmt.Errorf("scan instrument quantity step: %w", err)
	}
	minNotional, err := trading.FormatDecimal(minNotionalAtoms, quoteScale)
	if err != nil {
		return trading.Instrument{}, fmt.Errorf("scan instrument minimum notional: %w", err)
	}
	instrument, err := trading.NewInstrument(trading.InstrumentSpec{
		Base:          base,
		Quote:         quote,
		PriceScale:    priceScale,
		QuantityScale: quantityScale,
		PriceStep:     priceStep,
		QuantityStep:  quantityStep,
		MinNotional:   minNotional,
	})
	if err != nil {
		return trading.Instrument{}, fmt.Errorf("scan instrument filters: %w", err)
	}
	if instrument.Symbol() != symbol {
		return trading.Instrument{}, fmt.Errorf("scan instrument: symbol mismatch")
	}
	return instrument, nil
}
