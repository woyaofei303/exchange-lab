package trading

import "fmt"

// Quantity 表示精确的定点订单数量。
type Quantity struct {
	value fixed
}

// ParseQuantity 按指定精度解析正十进制数量。
func ParseQuantity(text string, scale uint8) (Quantity, error) {
	value, err := parseFixed(text, scale)
	if err != nil {
		return Quantity{}, fmt.Errorf("parse quantity: %w", err)
	}
	if value.atoms <= 0 {
		return Quantity{}, fmt.Errorf("parse quantity: %w", ErrNonPositive)
	}
	return Quantity{value: value}, nil
}

// Atoms 返回按 Scale 位小数表示的权威整数值。
func (q Quantity) Atoms() int64 {
	return q.value.atoms
}

// Scale 返回 Atoms 所表示的小数位数。
func (q Quantity) Scale() uint8 {
	return q.value.scale
}

// String 返回精确的定点十进制表示。
func (q Quantity) String() string {
	return q.value.String()
}
