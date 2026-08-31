// Package market composes authoritative and cache-backed market read models.
package market

import (
	"context"
	"fmt"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/trading"
)

// Ticker is a read-only market-data view. Decimal fields remain strings so the
// transport layer cannot accidentally treat them as binary floating point.
type Ticker struct {
	Symbol      string
	LastPrice   string
	MarkPrice   string
	IndexPrice  string
	PriceChange string
	Volume      string
	UpdatedAt   time.Time
}

// Reader is the business seam used by read-only market-data transports.
type Reader interface {
	InstrumentReader
	TickerReader
}

// InstrumentReader owns contract metadata reads.
type InstrumentReader interface {
	Instruments(context.Context, string) ([]trading.Instrument, error)
}

// TickerReader owns latest market snapshot reads.
type TickerReader interface {
	Tickers(context.Context, string) ([]Ticker, error)
}

// CompositeReader keeps instrument persistence and ticker delivery replaceable
// independently while exposing one transport-facing Reader.
type CompositeReader struct {
	instruments InstrumentReader
	tickers     TickerReader
}

// NewCompositeReader combines separate authoritative instrument and ticker readers.
func NewCompositeReader(instruments InstrumentReader, tickers TickerReader) (*CompositeReader, error) {
	if instruments == nil || tickers == nil {
		return nil, fmt.Errorf("create market reader: instrument and ticker readers are required")
	}
	return &CompositeReader{instruments: instruments, tickers: tickers}, nil
}

// Instruments delegates to the authoritative instrument reader.
func (r *CompositeReader) Instruments(ctx context.Context, symbol string) ([]trading.Instrument, error) {
	return r.instruments.Instruments(ctx, symbol)
}

// Tickers delegates to the authoritative ticker reader.
func (r *CompositeReader) Tickers(ctx context.Context, symbol string) ([]Ticker, error) {
	return r.tickers.Tickers(ctx, symbol)
}

// StaticReader is the Week 3 in-memory adapter. Later market-data work can
// replace it without changing HTTP handlers.
type StaticReader struct {
	instruments []trading.Instrument
	tickers     []Ticker
}

// NewStaticReader copies its inputs so callers cannot mutate reader state.
func NewStaticReader(instruments []trading.Instrument, tickers []Ticker) *StaticReader {
	return &StaticReader{
		instruments: append([]trading.Instrument(nil), instruments...),
		tickers:     append([]Ticker(nil), tickers...),
	}
}

// Instruments returns every instrument, or only the exact symbol requested.
func (r *StaticReader) Instruments(ctx context.Context, symbol string) ([]trading.Instrument, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]trading.Instrument, 0, len(r.instruments))
	for _, instrument := range r.instruments {
		if symbol == "" || instrument.Symbol() == symbol {
			result = append(result, instrument)
		}
	}
	return result, nil
}

// Tickers returns every ticker, or only the exact symbol requested.
func (r *StaticReader) Tickers(ctx context.Context, symbol string) ([]Ticker, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]Ticker, 0, len(r.tickers))
	for _, ticker := range r.tickers {
		if symbol == "" || ticker.Symbol == symbol {
			result = append(result, ticker)
		}
	}
	return result, nil
}
