package trading

import "fmt"

// InstrumentSpec contains the precision and risk filters for one USDT-margined
// perpetual contract. Decimal configuration is supplied as strings so no
// binary floating-point value can enter the domain.
type InstrumentSpec struct {
	Base          Asset
	Quote         Asset
	PriceScale    uint8
	QuantityScale uint8
	PriceStep     string
	QuantityStep  string
	MinNotional   string
}

// Instrument owns decimal parsing and order-filter validation for one contract.
type Instrument struct {
	base          Asset
	quote         Asset
	priceScale    uint8
	quantityScale uint8
	priceStep     Price
	quantityStep  Quantity
	minNotional   Money
}

// NewInstrument constructs a perpetual contract and rejects inconsistent filters.
func NewInstrument(spec InstrumentSpec) (Instrument, error) {
	if spec.Base.Code() == "" || spec.Quote.Code() == "" {
		return Instrument{}, fmt.Errorf("%w: base and quote assets are required", ErrInvalidInstrument)
	}
	if spec.Base.Code() == spec.Quote.Code() {
		return Instrument{}, fmt.Errorf("%w: base and quote assets must differ", ErrInvalidInstrument)
	}
	if spec.PriceScale > maxScale || spec.QuantityScale > maxScale {
		return Instrument{}, fmt.Errorf("%w: %w", ErrInvalidInstrument, ErrInvalidScale)
	}
	if int(spec.PriceScale)+int(spec.QuantityScale) > int(spec.Quote.Scale()) {
		return Instrument{}, fmt.Errorf("%w: quote asset scale %d cannot exactly represent price scale %d × quantity scale %d", ErrInvalidInstrument, spec.Quote.Scale(), spec.PriceScale, spec.QuantityScale)
	}

	priceStep, err := ParsePrice(spec.PriceStep, spec.PriceScale)
	if err != nil {
		return Instrument{}, fmt.Errorf("%w: price step: %w", ErrInvalidInstrument, err)
	}
	quantityStep, err := ParseQuantity(spec.QuantityStep, spec.QuantityScale)
	if err != nil {
		return Instrument{}, fmt.Errorf("%w: quantity step: %w", ErrInvalidInstrument, err)
	}
	minNotional, err := ParseMoney(spec.Quote, spec.MinNotional)
	if err != nil {
		return Instrument{}, fmt.Errorf("%w: minimum notional: %w", ErrInvalidInstrument, err)
	}
	if minNotional.Atoms() <= 0 {
		return Instrument{}, fmt.Errorf("%w: minimum notional: %w", ErrInvalidInstrument, ErrNonPositive)
	}

	return Instrument{
		base:          spec.Base,
		quote:         spec.Quote,
		priceScale:    spec.PriceScale,
		quantityScale: spec.QuantityScale,
		priceStep:     priceStep,
		quantityStep:  quantityStep,
		minNotional:   minNotional,
	}, nil
}

// Symbol returns the canonical perpetual contract symbol.
func (i Instrument) Symbol() string {
	return i.base.Code() + i.quote.Code() + "-PERP"
}

// Base returns the contract's underlying asset.
func (i Instrument) Base() Asset {
	return i.base
}

// Quote returns the contract's margin and settlement asset.
func (i Instrument) Quote() Asset {
	return i.quote
}

// PriceScale returns the maximum number of price decimal places.
func (i Instrument) PriceScale() uint8 {
	return i.priceScale
}

// QuantityScale returns the maximum number of quantity decimal places.
func (i Instrument) QuantityScale() uint8 {
	return i.quantityScale
}

// PriceStep returns the configured tick size.
func (i Instrument) PriceStep() Price {
	return i.priceStep
}

// QuantityStep returns the configured order quantity increment.
func (i Instrument) QuantityStep() Quantity {
	return i.quantityStep
}

// MinNotional returns the minimum accepted order value in the quote asset.
func (i Instrument) MinNotional() Money {
	return i.minNotional
}

// ParsePrice parses a price using this instrument's configured precision.
func (i Instrument) ParsePrice(text string) (Price, error) {
	if !i.configured() {
		return Price{}, fmt.Errorf("parse instrument price: %w", ErrInvalidInstrument)
	}
	return ParsePrice(text, i.priceScale)
}

// ParseQuantity parses a quantity using this instrument's configured precision.
func (i Instrument) ParseQuantity(text string) (Quantity, error) {
	if !i.configured() {
		return Quantity{}, fmt.Errorf("parse instrument quantity: %w", ErrInvalidInstrument)
	}
	return ParseQuantity(text, i.quantityScale)
}

// ValidateLimitOrder applies this instrument's precision, step and minimum
// notional filters to an exact price and quantity.
func (i Instrument) ValidateLimitOrder(price Price, quantity Quantity) error {
	if !i.configured() {
		return fmt.Errorf("validate limit order: %w", ErrInvalidInstrument)
	}
	if price.Atoms() <= 0 {
		return fmt.Errorf("validate limit order: price: %w", ErrNonPositive)
	}
	if quantity.Atoms() <= 0 {
		return fmt.Errorf("validate limit order: quantity: %w", ErrNonPositive)
	}
	if price.Scale() != i.priceScale {
		return fmt.Errorf("validate limit order: price: %w: got %d, want %d", ErrScaleMismatch, price.Scale(), i.priceScale)
	}
	if quantity.Scale() != i.quantityScale {
		return fmt.Errorf("validate limit order: quantity: %w: got %d, want %d", ErrScaleMismatch, quantity.Scale(), i.quantityScale)
	}
	if price.Atoms()%i.priceStep.Atoms() != 0 {
		return fmt.Errorf("validate limit order: price: %w: %s is not a multiple of %s", ErrStepMismatch, price.String(), i.priceStep.String())
	}
	if quantity.Atoms()%i.quantityStep.Atoms() != 0 {
		return fmt.Errorf("validate limit order: quantity: %w: %s is not a multiple of %s", ErrStepMismatch, quantity.String(), i.quantityStep.String())
	}
	notional, err := CalculateNotional(price, quantity, i.quote)
	if err != nil {
		return fmt.Errorf("validate limit order: %w", err)
	}
	if notional.Atoms() < i.minNotional.Atoms() {
		return fmt.Errorf("validate limit order: %w: got %s, minimum is %s", ErrBelowMinimumNotional, notional.String(), i.minNotional.String())
	}
	return nil
}

func (i Instrument) configured() bool {
	return i.base.Code() != "" &&
		i.quote.Code() != "" &&
		i.priceStep.Atoms() > 0 &&
		i.quantityStep.Atoms() > 0 &&
		i.minNotional.Atoms() > 0
}
