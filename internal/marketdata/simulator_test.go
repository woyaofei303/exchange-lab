package marketdata_test

import (
	"testing"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/marketdata"
)

func TestSimulatorGeneratesSnapshotTickerTradeAndDelta(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	simulator, err := marketdata.NewSimulator(marketdata.SimulatorConfig{
		StreamID: "sim-test", Symbol: "BTCUSDT-PERP", PriceScale: 2, QuantityScale: 3,
		InitialPriceAtoms: 5_000_000, TickAtoms: 10, QuantityAtoms: 1, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}

	wantTypes := []marketdata.EventType{
		marketdata.EventBookSnapshot,
		marketdata.EventTicker,
		marketdata.EventTrade,
		marketdata.EventBookDelta,
	}
	for index, wantType := range wantTypes {
		event := simulator.Next()
		if event.Type != wantType || event.Sequence != uint64(index+1) || event.StreamID != "sim-test" {
			t.Fatalf("Next(%d) = %#v, want type %s sequence %d", index, event, wantType, index+1)
		}
		if _, err := marketdata.Encode(event); err != nil {
			t.Fatalf("generated event invalid: %v", err)
		}
	}
}

func TestSimulatorRejectsInvalidOrOverflowingConfiguration(t *testing.T) {
	t.Parallel()

	tests := []marketdata.SimulatorConfig{
		{},
		{StreamID: "sim", Symbol: "BTCUSDT-PERP", PriceScale: 19, QuantityScale: 3, InitialPriceAtoms: 1, TickAtoms: 1, QuantityAtoms: 1, Now: time.Now},
		{StreamID: "sim", Symbol: "BTCUSDT-PERP", PriceScale: 2, QuantityScale: 3, InitialPriceAtoms: int64(^uint64(0) >> 1), TickAtoms: 1, QuantityAtoms: 1, Now: time.Now},
	}
	for _, config := range tests {
		if _, err := marketdata.NewSimulator(config); err == nil {
			t.Fatalf("NewSimulator(%#v) error = nil", config)
		}
	}
}
