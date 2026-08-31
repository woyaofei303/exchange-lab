package market_test

import (
	"context"
	"testing"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/market"
	"github.com/woyaofei303/exchange-lab/internal/trading"
)

type instrumentReaderFunc func(context.Context, string) ([]trading.Instrument, error)

func (fn instrumentReaderFunc) Instruments(ctx context.Context, symbol string) ([]trading.Instrument, error) {
	return fn(ctx, symbol)
}

type tickerReaderFunc func(context.Context, string) ([]market.Ticker, error)

func (fn tickerReaderFunc) Tickers(ctx context.Context, symbol string) ([]market.Ticker, error) {
	return fn(ctx, symbol)
}

func TestCompositeReaderDelegatesEachDataOwnershipBoundary(t *testing.T) {
	t.Parallel()

	instrument := mustInstrument(t, "BTC")
	updatedAt := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	reader, err := market.NewCompositeReader(
		instrumentReaderFunc(func(_ context.Context, symbol string) ([]trading.Instrument, error) {
			if symbol != "BTCUSDT-PERP" {
				t.Fatalf("instrument symbol = %q", symbol)
			}
			return []trading.Instrument{instrument}, nil
		}),
		tickerReaderFunc(func(_ context.Context, symbol string) ([]market.Ticker, error) {
			if symbol != "BTCUSDT-PERP" {
				t.Fatalf("ticker symbol = %q", symbol)
			}
			return []market.Ticker{{Symbol: symbol, LastPrice: "50000.00", UpdatedAt: updatedAt}}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewCompositeReader() error = %v", err)
	}

	instruments, err := reader.Instruments(context.Background(), "BTCUSDT-PERP")
	if err != nil || len(instruments) != 1 || instruments[0].Symbol() != "BTCUSDT-PERP" {
		t.Fatalf("Instruments() = %#v, %v", instruments, err)
	}
	tickers, err := reader.Tickers(context.Background(), "BTCUSDT-PERP")
	if err != nil || len(tickers) != 1 || !tickers[0].UpdatedAt.Equal(updatedAt) {
		t.Fatalf("Tickers() = %#v, %v", tickers, err)
	}
}

func TestCompositeReaderRejectsMissingDependency(t *testing.T) {
	t.Parallel()

	reader := market.NewStaticReader(nil, nil)
	if _, err := market.NewCompositeReader(nil, reader); err == nil {
		t.Fatal("NewCompositeReader(nil, reader) error = nil")
	}
	if _, err := market.NewCompositeReader(reader, nil); err == nil {
		t.Fatal("NewCompositeReader(reader, nil) error = nil")
	}
}
