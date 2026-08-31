package trading

import (
	"fmt"
	"math/big"
)

// Money is an exact signed amount denominated in one Asset.
type Money struct {
	asset Asset
	value fixed
}

// ParseMoney parses a decimal amount using the asset's authoritative ledger scale.
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

// Asset returns the denomination of this amount.
func (m Money) Asset() Asset {
	return m.asset
}

// Atoms returns the integer source-of-truth amount at Asset().Scale().
func (m Money) Atoms() int64 {
	return m.value.atoms
}

// String returns the exact amount followed by its asset code.
func (m Money) String() string {
	return m.value.String() + " " + m.asset.Code()
}

// Decimal returns the exact decimal amount without its asset suffix.
func (m Money) Decimal() string {
	return m.value.String()
}

// CalculateNotional returns price × quantity denominated in quoteAsset. The
// multiplication uses arbitrary-precision integers internally and fails rather
// than rounding when the quote asset scale cannot represent the exact result.
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
