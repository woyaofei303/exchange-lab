package trading

import "fmt"

// InstrumentSpec 包含一个 USDT 保证金永续合约的精度和风险过滤规则。
// 十进制配置以字符串提供，避免二进制浮点值进入领域层。
type InstrumentSpec struct {
	Base          Asset
	Quote         Asset
	PriceScale    uint8
	QuantityScale uint8
	PriceStep     string
	QuantityStep  string
	MinNotional   string
}

// Instrument 负责单个合约的十进制解析和订单过滤校验。
type Instrument struct {
	base          Asset
	quote         Asset
	priceScale    uint8
	quantityScale uint8
	priceStep     Price
	quantityStep  Quantity
	minNotional   Money
}

// NewInstrument 创建永续合约，并拒绝不一致的过滤规则。
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

// Symbol 返回规范的永续合约交易对符号。
func (i Instrument) Symbol() string {
	return i.base.Code() + i.quote.Code() + "-PERP"
}

// Base 返回合约的标的资产。
func (i Instrument) Base() Asset {
	return i.base
}

// Quote 返回合约的保证金和结算资产。
func (i Instrument) Quote() Asset {
	return i.quote
}

// PriceScale 返回价格允许的最大小数位数。
func (i Instrument) PriceScale() uint8 {
	return i.priceScale
}

// QuantityScale 返回数量允许的最大小数位数。
func (i Instrument) QuantityScale() uint8 {
	return i.quantityScale
}

// PriceStep 返回配置的最小价格变动单位。
func (i Instrument) PriceStep() Price {
	return i.priceStep
}

// QuantityStep 返回配置的订单数量递增步长。
func (i Instrument) QuantityStep() Quantity {
	return i.quantityStep
}

// MinNotional 返回以报价资产计价的最小可接受订单价值。
func (i Instrument) MinNotional() Money {
	return i.minNotional
}

// ParsePrice 使用该交易品种配置的精度解析价格。
func (i Instrument) ParsePrice(text string) (Price, error) {
	if !i.configured() {
		return Price{}, fmt.Errorf("parse instrument price: %w", ErrInvalidInstrument)
	}
	return ParsePrice(text, i.priceScale)
}

// ParseQuantity 使用该交易品种配置的精度解析数量。
func (i Instrument) ParseQuantity(text string) (Quantity, error) {
	if !i.configured() {
		return Quantity{}, fmt.Errorf("parse instrument quantity: %w", ErrInvalidInstrument)
	}
	return ParseQuantity(text, i.quantityScale)
}

// ValidateLimitOrder 使用该交易品种的精度、步长和最小名义价值规则，
// 校验精确的价格与数量。
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
