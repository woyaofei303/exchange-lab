package trading

import (
	"fmt"
	"math/big"
)

// Money 表示以某种资产计价的精确有符号金额。
type Money struct {
	asset Asset
	value fixed
}

// ParseMoney 使用资产的权威账本精度解析十进制金额。
func ParseMoney(asset Asset, text string) (Money, error) {
	if asset.Code() == "" {
		return Money{}, fmt.Errorf("parse money: %w: zero-value asset", ErrInvalidAsset)
	}
	value, err := parseFixed(text, asset.Scale())
	if err != nil {
		return Money{}, fmt.Errorf("parse money: %w", err)
	}
	return Money{asset: asset, value: value}, nil
}

// Asset 返回该金额的计价资产。
func (m Money) Asset() Asset {
	return m.asset
}

// Atoms 返回按 Asset().Scale() 精度表示的权威整数金额。
func (m Money) Atoms() int64 {
	return m.value.atoms
}

// String 返回精确金额及其资产代码。
func (m Money) String() string {
	return m.value.String() + " " + m.asset.Code()
}

// Decimal 返回不带资产后缀的精确十进制金额。
func (m Money) Decimal() string {
	return m.value.String()
}

// CalculateNotional 返回以 quoteAsset 计价的价格乘数量。
// 乘法内部使用任意精度整数；当报价资产精度无法表示精确结果时返回错误，
// 而不是执行舍入。
func CalculateNotional(price Price, quantity Quantity, quoteAsset Asset) (Money, error) {
	if quoteAsset.Code() == "" {
		return Money{}, fmt.Errorf("calculate notional: %w: zero-value quote asset", ErrInvalidAsset)
	}
	if price.Atoms() <= 0 || quantity.Atoms() <= 0 {
		return Money{}, fmt.Errorf("calculate notional: %w", ErrNonPositive)
	}

	product := new(big.Int).Mul(big.NewInt(price.Atoms()), big.NewInt(quantity.Atoms()))
	productScale := int(price.Scale()) + int(quantity.Scale())
	targetScale := int(quoteAsset.Scale())

	switch {
	case productScale < targetScale:
		product.Mul(product, pow10(targetScale-productScale))
	case productScale > targetScale:
		quotient, remainder := new(big.Int), new(big.Int)
		quotient.QuoRem(product, pow10(productScale-targetScale), remainder)
		if remainder.Sign() != 0 {
			return Money{}, fmt.Errorf("calculate notional: %w: result requires scale %d but %s uses scale %d", ErrPrecisionExceeded, productScale, quoteAsset.Code(), targetScale)
		}
		product = quotient
	}

	if !product.IsInt64() {
		return Money{}, fmt.Errorf("calculate notional: %w", ErrOverflow)
	}
	return Money{
		asset: quoteAsset,
		value: fixed{atoms: product.Int64(), scale: quoteAsset.Scale()},
	}, nil
}

func pow10(exponent int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
}
