package order

import (
	"errors"
	"testing"

	"exchange-lab/internal/trading"
)

func testInstrument(t *testing.T) trading.Instrument {
	t.Helper()
	base, _ := trading.NewAsset("BTC", 8)
	quote, _ := trading.NewAsset("USDT", 8)
	instrument, err := trading.NewInstrument(trading.InstrumentSpec{
		Base: base, Quote: quote, PriceScale: 2, QuantityScale: 3,
		PriceStep: "0.10", QuantityStep: "0.001", MinNotional: "5.00000000",
	})
	if err != nil {
		t.Fatal(err)
	}
	return instrument
}

func TestNewLimitOrderValidatesFiltersAndReservesInitialMargin(t *testing.T) {
	instrument := testInstrument(t)
	price, _ := instrument.ParsePrice("50000.00")
	quantity, _ := instrument.ParseQuantity("0.010")
	placed, err := NewLimit("ord_1", 7, "client-0001", instrument, trading.SideBuy, price, quantity, 10)
	if err != nil {
		t.Fatal(err)
	}
	if placed.Status != trading.OrderStatusPending || placed.RemainingAtoms != 10 || placed.ReservedMarginAtoms != 5_000_000_000 {
		t.Fatalf("order = %#v", placed)
	}
	if placed.PriceScale != 2 || placed.QuantityScale != 3 || placed.QuoteScale != 8 {
		t.Fatalf("scales = price %d quantity %d quote %d", placed.PriceScale, placed.QuantityScale, placed.QuoteScale)
	}
}

func TestNewLimitOrderRoundsMarginUpAndRejectsInvalidInput(t *testing.T) {
	instrument := testInstrument(t)
	price, _ := instrument.ParsePrice("50000.10")
	quantity, _ := instrument.ParseQuantity("0.001")
	placed, err := NewLimit("ord_2", 7, "client-0002", instrument, trading.SideSell, price, quantity, 7)
	if err != nil {
		t.Fatal(err)
	}
	if placed.ReservedMarginAtoms != 714_287_143 {
		t.Fatalf("reserved margin = %d", placed.ReservedMarginAtoms)
	}
	badPrice, _ := instrument.ParsePrice("50000.01")
	if _, err := NewLimit("ord_3", 7, "client-0003", instrument, trading.SideBuy, badPrice, quantity, 10); err == nil {
		t.Fatal("invalid tick accepted")
	}
	if _, err := NewLimit("ord_3", 0, "client-0003", instrument, trading.SideBuy, price, quantity, 10); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid identity error = %v", err)
	}
}

func TestAcceptAndCancelReleaseExactlyOnce(t *testing.T) {
	instrument := testInstrument(t)
	price, _ := instrument.ParsePrice("50000.00")
	quantity, _ := instrument.ParseQuantity("0.010")
	placed, _ := NewLimit("ord_4", 7, "client-0004", instrument, trading.SideBuy, price, quantity, 10)
	open, err := placed.Accept()
	if err != nil || open.Status != trading.OrderStatusOpen {
		t.Fatalf("Accept() = %#v, %v", open, err)
	}
	canceled, released, err := open.Cancel()
	if err != nil || canceled.Status != trading.OrderStatusCanceled || released != 5_000_000_000 || canceled.ReservedMarginAtoms != 0 {
		t.Fatalf("Cancel() = %#v, release=%d, %v", canceled, released, err)
	}
	if _, _, err := canceled.Cancel(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("second Cancel() error = %v", err)
	}
}
