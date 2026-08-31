package market_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"exchange-lab/internal/market"
	"exchange-lab/internal/trading"
)

func TestStaticReaderFiltersInstrumentsBySymbol(t *testing.T) {
	t.Parallel()

	btc := mustInstrument(t, "BTC")
	eth := mustInstrument(t, "ETH")
	reader := market.NewStaticReader([]trading.Instrument{btc, eth}, nil)

	instruments, err := reader.Instruments(context.Background(), "ETHUSDT-PERP")
	if err != nil {
		t.Fatalf("Instruments() error = %v", err)
	}
	if got, want := len(instruments), 1; got != want {
		t.Fatalf("len(instruments) = %d, want %d", got, want)
	}
	if got, want := instruments[0].Symbol(), "ETHUSDT-PERP"; got != want {
		t.Fatalf("instrument symbol = %q, want %q", got, want)
	}
}

func TestStaticReaderFiltersTickersBySymbol(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	reader := market.NewStaticReader(nil, []market.Ticker{
		{Symbol: "BTCUSDT-PERP", LastPrice: "50000.00", UpdatedAt: updatedAt},
		{Symbol: "ETHUSDT-PERP", LastPrice: "3000.00", UpdatedAt: updatedAt},
	})

	tickers, err := reader.Tickers(context.Background(), "BTCUSDT-PERP")
	if err != nil {
		t.Fatalf("Tickers() error = %v", err)
	}
	if got, want := len(tickers), 1; got != want {
		t.Fatalf("len(tickers) = %d, want %d", got, want)
	}
	if got, want := tickers[0].LastPrice, "50000.00"; got != want {
		t.Fatalf("last price = %q, want %q", got, want)
	}
}

func TestStaticReaderHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := market.NewStaticReader(nil, nil)

	if _, err := reader.Instruments(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("Instruments() error = %v, want context.Canceled", err)
	}
	if _, err := reader.Tickers(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("Tickers() error = %v, want context.Canceled", err)
	}
}

func mustInstrument(t *testing.T, baseCode string) trading.Instrument {
	t.Helper()

	base, err := trading.NewAsset(baseCode, 8)
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	quote, err := trading.NewAsset("USDT", 8)
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	instrument, err := trading.NewInstrument(trading.InstrumentSpec{
		Base:          base,
		Quote:         quote,
		PriceScale:    2,
		QuantityScale: 3,
		PriceStep:     "0.10",
		QuantityStep:  "0.001",
		MinNotional:   "5",
	})
	if err != nil {
		t.Fatalf("NewInstrument() error = %v", err)
	}
	return instrument
}
