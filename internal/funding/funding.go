// Package funding 计算带上限的费率和零和结算差额。
package funding

import (
	"errors"
	"math/big"

	"exchange-lab/internal/fixedmath"
)

// ErrInvalidFunding 表示费率、价格、精度无效或发生算术溢出。
var ErrInvalidFunding = errors.New("invalid funding input")

// ComputeRate 合并溢价与利率，并按 capBPS 对称限制结果。
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

// Payment 返回有符号的用户账本变动：正费率表示多方向空方支付。
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

// SystemOffset 返回维持所有资金费条目零和所需的精确保险基金账本变动，
// 用于清算导致教学系统暂时出现单边敞口的情况。
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
