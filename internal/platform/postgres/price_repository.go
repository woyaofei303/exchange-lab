package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/woyaofei303/exchange-lab/internal/contractprice"
)

// PriceRepository persists source evidence, index prices and mark prices together.
type PriceRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewPriceRepository constructs a context-bounded price repository.
func NewPriceRepository(pool *pgxpool.Pool, timeout time.Duration) *PriceRepository {
	return &PriceRepository{pool: pool, timeout: timeout}
}

// Save commits source status, index evidence and mark price in one transaction.
func (r *PriceRepository) Save(ctx context.Context, symbol string, scale uint8, sources []contractprice.Source, index contractprice.IndexResult, markAtoms, premiumBPS int64) (resultErr error) {
	if r == nil || r.pool == nil || r.timeout <= 0 || symbol == "" || scale > 18 || len(sources) == 0 || index.PriceAtoms <= 0 || markAtoms <= 0 || index.AsOf.IsZero() {
		return fmt.Errorf("save contract price: invalid input")
	}
	queryContext, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	tx, err := r.pool.Begin(queryContext)
	if err != nil {
		return fmt.Errorf("save contract price: begin: %w", err)
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback(queryContext)
		}
	}()
	included := make(map[string]struct{}, len(index.Included))
	for _, name := range index.Included {
		included[name] = struct{}{}
	}
	for _, source := range sources {
		status := "excluded"
		if _, ok := included[source.Name]; ok {
			status = "included"
		}
		if _, err := tx.Exec(queryContext, `
			INSERT INTO price_sources (symbol, source_name, price_atoms, price_scale, weight, updated_at, status)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (symbol, source_name) DO UPDATE SET price_atoms=EXCLUDED.price_atoms,
			price_scale=EXCLUDED.price_scale, weight=EXCLUDED.weight, updated_at=EXCLUDED.updated_at, status=EXCLUDED.status`,
			symbol, source.Name, source.PriceAtoms, scale, source.Weight, source.UpdatedAt.UTC(), status); err != nil {
			return fmt.Errorf("save contract price source: %w", err)
		}
	}
	if _, err := tx.Exec(queryContext, `INSERT INTO index_prices
		(symbol,price_atoms,price_scale,included_sources,excluded_sources,fresh,as_of)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, symbol, index.PriceAtoms, scale, index.Included, index.Excluded, index.Fresh, index.AsOf.UTC()); err != nil {
		return fmt.Errorf("save index price: %w", err)
	}
	if _, err := tx.Exec(queryContext, `INSERT INTO mark_prices
		(symbol,index_price_atoms,mark_price_atoms,price_scale,premium_bps,fresh,as_of)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, symbol, index.PriceAtoms, markAtoms, scale, premiumBPS, index.Fresh, index.AsOf.UTC()); err != nil {
		return fmt.Errorf("save mark price: %w", err)
	}
	if err := tx.Commit(queryContext); err != nil {
		return fmt.Errorf("save contract price: commit: %w", err)
	}
	return nil
}

// Latest returns the newest mark with matching index-source evidence.
func (r *PriceRepository) Latest(ctx context.Context, symbol string) (contractprice.Snapshot, error) {
	if r == nil || r.pool == nil || r.timeout <= 0 || symbol == "" {
		return contractprice.Snapshot{}, fmt.Errorf("latest contract price: invalid input")
	}
	queryContext, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	var result contractprice.Snapshot
	err := r.pool.QueryRow(queryContext, `
		SELECT m.symbol,m.index_price_atoms,m.mark_price_atoms,m.price_scale,m.premium_bps,
		       i.included_sources,i.excluded_sources,(m.fresh AND i.fresh),LEAST(m.as_of,i.as_of)
		FROM mark_prices m JOIN LATERAL (
			SELECT * FROM index_prices WHERE symbol=m.symbol ORDER BY created_at DESC,id DESC LIMIT 1
		) i ON true WHERE m.symbol=$1 ORDER BY m.created_at DESC,m.id DESC LIMIT 1`, symbol).Scan(
		&result.Symbol, &result.IndexAtoms, &result.MarkAtoms, &result.PriceScale, &result.PremiumBPS,
		&result.IncludedSources, &result.ExcludedSources, &result.Fresh, &result.AsOf)
	if err != nil {
		return contractprice.Snapshot{}, fmt.Errorf("latest contract price: %w", err)
	}
	return result, nil
}
