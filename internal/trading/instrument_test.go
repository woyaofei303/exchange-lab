package trading

import (
	"errors"
	"testing"
)

func TestNewInstrumentBuildsPerpetualContract(t *testing.T) {
	t.Parallel()

	btc := mustAsset(t, "BTC", 8)
	usdt := mustAsset(t, "USDT", 8)
	instrument, err := NewInstrument(InstrumentSpec{
		Base:          btc,
		Quote:         usdt,
		PriceScale:    2,
		QuantityScale: 3,
		PriceStep:     "0.10",
		QuantityStep:  "0.001",
		MinNotional:   "5",
	})
	if err != nil {
		t.Fatalf("NewInstrument() error = %v", err)
	}
	if got, want := instrument.Symbol(), "BTCUSDT-PERP"; got != want {
		t.Fatalf("Symbol() = %q, want %q", got, want)
	}
	if got, want := instrument.PriceScale(), uint8(2); got != want {
		t.Fatalf("PriceScale() = %d, want %d", got, want)
	}
	if got, want := instrument.QuantityScale(), uint8(3); got != want {
		t.Fatalf("QuantityScale() = %d, want %d", got, want)
	}
	if got, want := instrument.MinNotional().String(), "5.00000000 USDT"; got != want {
		t.Fatalf("MinNotional() = %q, want %q", got, want)
	}
}

func TestInstrumentExposesBaseAndQuoteAssets(t *testing.T) {
	t.Parallel()

	instrument := mustBTCUSDTInstrument(t)
	if got, want := instrument.Base().Code(), "BTC"; got != want {
		t.Fatalf("Base().Code() = %q, want %q", got, want)
	}
	if got, want := instrument.Quote().Code(), "USDT"; got != want {
		t.Fatalf("Quote().Code() = %q, want %q", got, want)
	}
}

func TestInstrumentExposesTradingFilters(t *testing.T) {
	t.Parallel()

	instrument := mustBTCUSDTInstrument(t)
	if got, want := instrument.PriceStep().String(), "0.10"; got != want {
		t.Fatalf("PriceStep() = %q, want %q", got, want)
	}
	if got, want := instrument.QuantityStep().String(), "0.001"; got != want {
		t.Fatalf("QuantityStep() = %q, want %q", got, want)
	}
}

func TestInstrumentAcceptsValidLimitOrder(t *testing.T) {
	t.Parallel()

	instrument := mustBTCUSDTInstrument(t)
	price := mustPrice(t, instrument, "50000.10")
	quantity := mustQuantity(t, instrument, "0.001")

	if err := instrument.ValidateLimitOrder(price, quantity); err != nil {
		t.Fatalf("ValidateLimitOrder() error = %v", err)
	}
}

func TestInstrumentRejectsPriceWithDifferentScale(t *testing.T) {
	t.Parallel()

	instrument := mustBTCUSDTInstrument(t)
	price, err := ParsePrice("50000.1", 1)
	if err != nil {
		t.Fatalf("ParsePrice() error = %v", err)
	}
	quantity := mustQuantity(t, instrument, "0.001")

	err = instrument.ValidateLimitOrder(price, quantity)
	if !errors.Is(err, ErrScaleMismatch) {
		t.Fatalf("ValidateLimitOrder() error = %v, want ErrScaleMismatch", err)
	}
}

func TestInstrumentRejectsQuantityWithDifferentScale(t *testing.T) {
	t.Parallel()

	instrument := mustBTCUSDTInstrument(t)
	price := mustPrice(t, instrument, "50000.10")
	quantity, err := ParseQuantity("0.0010", 4)
	if err != nil {
		t.Fatalf("ParseQuantity() error = %v", err)
	}

	err = instrument.ValidateLimitOrder(price, quantity)
	if !errors.Is(err, ErrScaleMismatch) {
		t.Fatalf("ValidateLimitOrder() error = %v, want ErrScaleMismatch", err)
	}
}

func TestInstrumentRejectsPriceOutsideTickSize(t *testing.T) {
	t.Parallel()

	instrument := mustBTCUSDTInstrument(t)
	price := mustPrice(t, instrument, "50000.15")
	quantity := mustQuantity(t, instrument, "0.001")

	err := instrument.ValidateLimitOrder(price, quantity)
	if !errors.Is(err, ErrStepMismatch) {
		t.Fatalf("ValidateLimitOrder() error = %v, want ErrStepMismatch", err)
	}
}

func TestInstrumentRejectsQuantityOutsideStepSize(t *testing.T) {
	t.Parallel()

	instrument, err := NewInstrument(InstrumentSpec{
		Base:          mustAsset(t, "BTC", 8),
		Quote:         mustAsset(t, "USDT", 8),
		PriceScale:    2,
		QuantityScale: 3,
		PriceStep:     "0.10",
		QuantityStep:  "0.005",
		MinNotional:   "5",
	})
	if err != nil {
		t.Fatalf("NewInstrument() error = %v", err)
	}
	price := mustPrice(t, instrument, "50000.10")
	quantity := mustQuantity(t, instrument, "0.006")

	err = instrument.ValidateLimitOrder(price, quantity)
	if !errors.Is(err, ErrStepMismatch) {
		t.Fatalf("ValidateLimitOrder() error = %v, want ErrStepMismatch", err)
	}
}

func TestInstrumentRejectsOrderBelowMinimumNotional(t *testing.T) {
	t.Parallel()

	instrument := mustBTCUSDTInstrument(t)
	price := mustPrice(t, instrument, "1000.00")
	quantity := mustQuantity(t, instrument, "0.001")

	err := instrument.ValidateLimitOrder(price, quantity)
	if !errors.Is(err, ErrBelowMinimumNotional) {
		t.Fatalf("ValidateLimitOrder() error = %v, want ErrBelowMinimumNotional", err)
	}
}

func TestNewInstrumentRejectsSameBaseAndQuoteCode(t *testing.T) {
	t.Parallel()

	_, err := NewInstrument(InstrumentSpec{
		Base:          mustAsset(t, "BTC", 8),
		Quote:         mustAsset(t, "BTC", 6),
		PriceScale:    2,
		QuantityScale: 3,
		PriceStep:     "0.10",
		QuantityStep:  "0.001",
		MinNotional:   "5",
	})
	if !errors.Is(err, ErrInvalidInstrument) {
		t.Fatalf("NewInstrument() error = %v, want ErrInvalidInstrument", err)
	}
}

func TestInstrumentAcceptsOrderAtMinimumNotional(t *testing.T) {
	t.Parallel()

	instrument := mustBTCUSDTInstrument(t)
	price := mustPrice(t, instrument, "5000.00")
	quantity := mustQuantity(t, instrument, "0.001")

	if err := instrument.ValidateLimitOrder(price, quantity); err != nil {
		t.Fatalf("ValidateLimitOrder() error = %v", err)
	}
}

func TestInstrumentParsingEnforcesConfiguredPrecision(t *testing.T) {
	t.Parallel()

	instrument := mustBTCUSDTInstrument(t)
	if _, err := instrument.ParsePrice("1.001"); !errors.Is(err, ErrPrecisionExceeded) {
		t.Fatalf("ParsePrice() error = %v, want ErrPrecisionExceeded", err)
	}
	if _, err := instrument.ParseQuantity("0.0001"); !errors.Is(err, ErrPrecisionExceeded) {
		t.Fatalf("ParseQuantity() error = %v, want ErrPrecisionExceeded", err)
	}
}

func TestNewInstrumentRejectsInvalidFilters(t *testing.T) {
	t.Parallel()

	btc := mustAsset(t, "BTC", 8)
	usdt := mustAsset(t, "USDT", 8)
	usdtScaleFour := mustAsset(t, "USDT", 4)
	valid := InstrumentSpec{
		Base:          btc,
		Quote:         usdt,
		PriceScale:    2,
		QuantityScale: 3,
		PriceStep:     "0.10",
		QuantityStep:  "0.001",
		MinNotional:   "5",
	}
	tests := []struct {
		name   string
		mutate func(*InstrumentSpec)
	}{
		{name: "missing base", mutate: func(spec *InstrumentSpec) { spec.Base = Asset{} }},
		{name: "missing quote", mutate: func(spec *InstrumentSpec) { spec.Quote = Asset{} }},
		{name: "unsupported price scale", mutate: func(spec *InstrumentSpec) { spec.PriceScale = 19 }},
		{name: "zero price step", mutate: func(spec *InstrumentSpec) { spec.PriceStep = "0" }},
		{name: "negative quantity step", mutate: func(spec *InstrumentSpec) { spec.QuantityStep = "-0.001" }},
		{name: "zero minimum notional", mutate: func(spec *InstrumentSpec) { spec.MinNotional = "0" }},
		{name: "insufficient quote scale", mutate: func(spec *InstrumentSpec) {
			spec.Quote = usdtScaleFour
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := valid
			tt.mutate(&spec)
			_, err := NewInstrument(spec)
			if !errors.Is(err, ErrInvalidInstrument) {
				t.Fatalf("NewInstrument() error = %v, want ErrInvalidInstrument", err)
			}
		})
	}
}

func TestZeroValueInstrumentFailsClosed(t *testing.T) {
	t.Parallel()

	err := (Instrument{}).ValidateLimitOrder(Price{}, Quantity{})
	if !errors.Is(err, ErrInvalidInstrument) {
		t.Fatalf("ValidateLimitOrder() error = %v, want ErrInvalidInstrument", err)
	}
}

func TestInstrumentRejectsZeroValuePriceAndQuantity(t *testing.T) {
	t.Parallel()

	instrument := mustBTCUSDTInstrument(t)
	validPrice := mustPrice(t, instrument, "5000.00")
	validQuantity := mustQuantity(t, instrument, "0.001")
	tests := []struct {
		name     string
		price    Price
		quantity Quantity
	}{
		{name: "zero price", price: Price{}, quantity: validQuantity},
		{name: "zero quantity", price: validPrice, quantity: Quantity{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := instrument.ValidateLimitOrder(tt.price, tt.quantity)
			if !errors.Is(err, ErrNonPositive) {
				t.Fatalf("ValidateLimitOrder() error = %v, want ErrNonPositive", err)
			}
		})
	}
}
