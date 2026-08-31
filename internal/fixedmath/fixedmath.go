// Package fixedmath provides bounded integer arithmetic for financial domain
// packages. It never converts values through binary floating point.
package fixedmath

import (
	"errors"
	"math/big"
)

// ErrArithmetic reports invalid divisors, unsupported signs or int64 overflow.
var ErrArithmetic = errors.New("fixed arithmetic error")

// MulDiv returns a*b/divisor using an unbounded intermediate and truncation toward zero.
func MulDiv(a, b, divisor int64) (int64, error) {
	if divisor <= 0 {
		return 0, ErrArithmetic
	}
	n := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	q := new(big.Int).Quo(n, big.NewInt(divisor))
	if !q.IsInt64() {
		return 0, ErrArithmetic
	}
	return q.Int64(), nil
}

// MulDivCeil returns ceil(a*b/divisor) for non-negative financial values.
func MulDivCeil(a, b, divisor int64) (int64, error) {
	if a < 0 || b < 0 || divisor <= 0 {
		return 0, ErrArithmetic
	}
	n := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	d := big.NewInt(divisor)
	q, r := new(big.Int), new(big.Int)
	q.QuoRem(n, d, r)
	if r.Sign() != 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() {
		return 0, ErrArithmetic
	}
	return q.Int64(), nil
}

// Pow10 returns 10^scale when it fits in int64.
func Pow10(scale uint8) (int64, error) {
	if scale > 18 {
		return 0, ErrArithmetic
	}
	value := int64(1)
	for i := uint8(0); i < scale; i++ {
		if value > 922337203685477580 {
			return 0, ErrArithmetic
		}
		value *= 10
	}
	return value, nil
}

// Abs returns the absolute value and rejects the unrepresentable MinInt64 case.
func Abs(value int64) (int64, error) {
	if value == -1<<63 {
		return 0, ErrArithmetic
	}
	if value < 0 {
		return -value, nil
	}
	return value, nil
}
