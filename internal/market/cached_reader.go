package market

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/woyaofei303/exchange-lab/internal/trading"
)

// Cache stores only rebuildable market read models. A cache error must never
// replace the authoritative Reader result.
type Cache interface {
	GetInstruments(context.Context, string) ([]trading.Instrument, bool, error)
	PutInstruments(context.Context, string, []trading.Instrument) error
	GetTickers(context.Context, string) ([]Ticker, bool, error)
	PutTickers(context.Context, string, []Ticker) error
}

// CachedReader implements cache-aside reads over an authoritative market
// source. Redis failures degrade to the source; source failures remain visible.
type CachedReader struct {
	source Reader
	cache  Cache
	logger *slog.Logger
}

// NewCachedReader validates an authoritative reader and rebuildable cache.
func NewCachedReader(source Reader, cache Cache, logger *slog.Logger) (*CachedReader, error) {
	if source == nil || cache == nil {
		return nil, fmt.Errorf("create cached market reader: source and cache are required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CachedReader{source: source, cache: cache, logger: logger}, nil
}

// Instruments serves cache hits and falls back to the source on cache failure.
func (r *CachedReader) Instruments(ctx context.Context, symbol string) ([]trading.Instrument, error) {
	items, hit, err := r.cache.GetInstruments(ctx, symbol)
	if err == nil && hit {
		return items, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		r.logger.Warn("market cache read failed", "kind", "instruments")
	}
	items, err = r.source.Instruments(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if err := r.cache.PutInstruments(ctx, symbol, items); err != nil && ctx.Err() == nil {
		r.logger.Warn("market cache write failed", "kind", "instruments")
	}
	return items, nil
}

// Tickers serves cache hits and treats cache writes as best-effort projections.
func (r *CachedReader) Tickers(ctx context.Context, symbol string) ([]Ticker, error) {
	items, hit, err := r.cache.GetTickers(ctx, symbol)
	if err == nil && hit {
		return items, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		r.logger.Warn("market cache read failed", "kind", "tickers")
	}
	items, err = r.source.Tickers(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if err := r.cache.PutTickers(ctx, symbol, items); err != nil && ctx.Err() == nil {
		r.logger.Warn("market cache write failed", "kind", "tickers")
	}
	return items, nil
}
