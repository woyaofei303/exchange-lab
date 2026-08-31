package trading

import "testing"

func mustAsset(t *testing.T, code string, scale uint8) Asset {
	t.Helper()

	asset, err := NewAsset(code, scale)
	if err != nil {
		t.Fatalf("NewAsset(%q, %d) error = %v", code, scale, err)
	}
	return asset
}

func mustBTCUSDTInstrument(t *testing.T) Instrument {
	t.Helper()

	instrument, err := NewInstrument(InstrumentSpec{
		Base:          mustAsset(t, "BTC", 8),
		Quote:         mustAsset(t, "USDT", 8),
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

func mustPrice(t *testing.T, instrument Instrument, text string) Price {
	t.Helper()

	price, err := instrument.ParsePrice(text)
	if err != nil {
		t.Fatalf("Instrument.ParsePrice(%q) error = %v", text, err)
	}
	return price
}

func mustQuantity(t *testing.T, instrument Instrument, text string) Quantity {
	t.Helper()

	quantity, err := instrument.ParseQuantity(text)
	if err != nil {
		t.Fatalf("Instrument.ParseQuantity(%q) error = %v", text, err)
	}
	return quantity
}
