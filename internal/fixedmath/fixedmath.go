// Package fixedmath 为金融领域包提供有界整数运算，绝不通过二进制浮点数转换值。
package fixedmath

import (
	"errors"
	"math/big"
)

// ErrArithmetic 表示除数无效、符号不受支持或 int64 溢出。
var ErrArithmetic = errors.New("fixed arithmetic error")

// MulDiv 使用无限精度中间值计算 a*b/divisor，并向零截断。
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

// MulDivCeil 对非负金融数值返回 ceil(a*b/divisor)。
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

// Pow10 在结果可用 int64 表示时返回 10^scale。
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

// Abs 返回绝对值，并拒绝无法表示的 MinInt64 情况。
func Abs(value int64) (int64, error) {
	if value == -1<<63 {
		return 0, ErrArithmetic
	}
	if value < 0 {
		return -value, nil
	}
	return value, nil
}
