package market_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"exchange-lab/internal/market"
	"exchange-lab/internal/trading"
)

type marketCache struct {
	getInstruments func(context.Context, string) ([]trading.Instrument, bool, error)
	putInstruments func(context.Context, string, []trading.Instrument) error
	getTickers     func(context.Context, string) ([]market.Ticker, bool, error)
	putTickers     func(context.Context, string, []market.Ticker) error
}

func (c marketCache) GetInstruments(ctx context.Context, symbol string) ([]trading.Instrument, bool, error) {
	return c.getInstruments(ctx, symbol)
}

func (c marketCache) PutInstruments(ctx context.Context, symbol string, instruments []trading.Instrument) error {
	return c.putInstruments(ctx, symbol, instruments)
}

func (c marketCache) GetTickers(ctx context.Context, symbol string) ([]market.Ticker, bool, error) {
	return c.getTickers(ctx, symbol)
}

func (c marketCache) PutTickers(ctx context.Context, symbol string, tickers []market.Ticker) error {
	return c.putTickers(ctx, symbol, tickers)
}

func TestCachedReaderReturnsCacheHitWithoutCallingSource(t *testing.T) {
	t.Parallel()

	want := mustInstrument(t, "BTC")
	cache := marketCache{
		getInstruments: func(context.Context, string) ([]trading.Instrument, bool, error) {
			return []trading.Instrument{want}, true, nil
		},
		putInstruments: func(context.Context, string, []trading.Instrument) error { return nil },
		getTickers:     func(context.Context, string) ([]market.Ticker, bool, error) { return nil, false, nil },
		putTickers:     func(context.Context, string, []market.Ticker) error { return nil },
	}
	source := market.NewStaticReader([]trading.Instrument{mustInstrument(t, "ETH")}, nil)
	reader, err := market.NewCachedReader(source, cache, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewCachedReader() error = %v", err)
	}

	got, err := reader.Instruments(context.Background(), "BTCUSDT-PERP")
	if err != nil || len(got) != 1 || got[0].Symbol() != want.Symbol() {
		t.Fatalf("Instruments() = %#v, %v", got, err)
	}
}

func TestCachedReaderFillsCacheAfterMiss(t *testing.T) {
	t.Parallel()

	want := []market.Ticker{{Symbol: "BTCUSDT-PERP", LastPrice: "50000.00"}}
	var cached []market.Ticker
	cache := marketCache{
		getInstruments: func(context.Context, string) ([]trading.Instrument, bool, error) { return nil, false, nil },
		putInstruments: func(context.Context, string, []trading.Instrument) error { return nil },
		getTickers:     func(context.Context, string) ([]market.Ticker, bool, error) { return nil, false, nil },
		putTickers: func(_ context.Context, _ string, tickers []market.Ticker) error {
			cached = append([]market.Ticker(nil), tickers...)
			return nil
		},
	}
	reader, err := market.NewCachedReader(market.NewStaticReader(nil, want), cache, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewCachedReader() error = %v", err)
	}

	got, err := reader.Tickers(context.Background(), "BTCUSDT-PERP")
	if err != nil || len(got) != 1 || len(cached) != 1 || cached[0].LastPrice != want[0].LastPrice {
		t.Fatalf("Tickers() = %#v, cached = %#v, err = %v", got, cached, err)
	}
}

func TestCachedReaderFallsBackToAuthorityWhenCacheUnavailable(t *testing.T) {
	t.Parallel()

	cacheFailure := errors.New("redis unavailable")
	cache := marketCache{
		getInstruments: func(context.Context, string) ([]trading.Instrument, bool, error) {
			return nil, false, cacheFailure
		},
		putInstruments: func(context.Context, string, []trading.Instrument) error { return cacheFailure },
		getTickers:     func(context.Context, string) ([]market.Ticker, bool, error) { return nil, false, cacheFailure },
		putTickers:     func(context.Context, string, []market.Ticker) error { return cacheFailure },
	}
	source := market.NewStaticReader([]trading.Instrument{mustInstrument(t, "BTC")}, nil)
	reader, err := market.NewCachedReader(source, cache, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewCachedReader() error = %v", err)
	}

	got, err := reader.Instruments(context.Background(), "BTCUSDT-PERP")
	if err != nil || len(got) != 1 || got[0].Symbol() != "BTCUSDT-PERP" {
		t.Fatalf("Instruments() = %#v, %v", got, err)
	}
}

func TestCachedReaderReturnsAuthorityError(t *testing.T) {
	t.Parallel()

	authorityFailure := errors.New("postgres unavailable")
	cache := marketCache{
		getInstruments: func(context.Context, string) ([]trading.Instrument, bool, error) { return nil, false, nil },
		putInstruments: func(context.Context, string, []trading.Instrument) error { return nil },
		getTickers:     func(context.Context, string) ([]market.Ticker, bool, error) { return nil, false, nil },
		putTickers:     func(context.Context, string, []market.Ticker) error { return nil },
	}
	source := &readerFailure{err: authorityFailure}
	reader, err := market.NewCachedReader(source, cache, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewCachedReader() error = %v", err)
	}

	if _, err := reader.Instruments(context.Background(), ""); !errors.Is(err, authorityFailure) {
		t.Fatalf("Instruments() error = %v, want authority failure", err)
	}
}

type readerFailure struct{ err error }

func (r *readerFailure) Instruments(context.Context, string) ([]trading.Instrument, error) {
	return nil, r.err
}

func (r *readerFailure) Tickers(context.Context, string) ([]market.Ticker, error) {
	return nil, r.err
}
