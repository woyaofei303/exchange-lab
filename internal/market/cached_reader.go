package market

import (
	"context"
	"fmt"
	"log/slog"

	"exchange-lab/internal/trading"
)

// Cache 仅存储可重建的行情读取模型，缓存错误绝不能替代权威 Reader 的结果。
type Cache interface {
	GetInstruments(context.Context, string) ([]trading.Instrument, bool, error)
	PutInstruments(context.Context, string, []trading.Instrument) error
	GetTickers(context.Context, string) ([]Ticker, bool, error)
	PutTickers(context.Context, string, []Ticker) error
}

// CachedReader 在权威行情源之上实现旁路缓存读取。
// Redis 故障时降级到权威源；权威源故障仍会向上返回。
type CachedReader struct {
	source Reader
	cache  Cache
	logger *slog.Logger
}

// NewCachedReader 校验权威读取器和可重建缓存。
func NewCachedReader(source Reader, cache Cache, logger *slog.Logger) (*CachedReader, error) {
	if source == nil || cache == nil {
		return nil, fmt.Errorf("create cached market reader: source and cache are required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CachedReader{source: source, cache: cache, logger: logger}, nil
}

// Instruments 返回缓存命中结果，缓存失败时回退到权威源。
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

// Tickers 返回缓存命中结果，并将缓存写入视为尽力而为的投影。
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
