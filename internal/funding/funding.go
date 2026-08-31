// Package funding calculates capped rates and zero-sum settlement deltas.
package funding

import (
	"errors"
	"math/big"

	"github.com/woyaofei303/exchange-lab/internal/fixedmath"
)

// ErrInvalidFunding reports invalid rates, prices, scales or arithmetic overflow.
var ErrInvalidFunding = errors.New("invalid funding input")

// ComputeRate combines premium and interest, clamped symmetrically to capBPS.
func ComputeRate(premiumBPS, interestBPS, capBPS int64) (int64, error) {
	if capBPS < 0 || capBPS > 10_000 {
		return 0, ErrInvalidFunding
	}
	rate := premiumBPS + interestBPS
	if rate > capBPS {
		rate = capBPS
	}
	if rate < -capBPS {
		rate = -capBPS
	}
	return rate, nil
}

// Payment returns the signed user ledger delta: positive rate means longs pay shorts.
func Payment(positionSizeAtoms, markPriceAtoms int64, quantityScale uint8, rateBPS int64) (int64, error) {
	if markPriceAtoms <= 0 || rateBPS < -10_000 || rateBPS > 10_000 {
		return 0, ErrInvalidFunding
	}
	factor, err := fixedmath.Pow10(quantityScale)
	if err != nil {
		return 0, ErrInvalidFunding
	}
	notional, err := fixedmath.MulDiv(positionSizeAtoms, markPriceAtoms, factor)
	if err != nil {
		return 0, ErrInvalidFunding
	}
	payment, err := fixedmath.MulDiv(notional, rateBPS, 10_000)
	if err != nil {
		return 0, ErrInvalidFunding
	}
	return -payment, nil
}

// SystemOffset returns the exact insurance-fund ledger delta required to keep
// all funding entries zero-sum when liquidations leave a temporary one-sided
// teaching-system exposure.
func SystemOffset(payments []int64) (int64, error) {
	total := big.NewInt(0)
	for _, payment := range payments {
		total.Add(total, big.NewInt(payment))
	}
	total.Neg(total)
	if !total.IsInt64() {
		return 0, ErrInvalidFunding
	}
	return total.Int64(), nil
}
