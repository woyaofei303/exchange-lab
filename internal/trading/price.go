package trading

import "fmt"

// Price 表示精确的定点报价，绝不使用二进制浮点数。
type Price struct {
	value fixed
}

// ParsePrice 按指定精度解析正十进制价格。
func ParsePrice(text string, scale uint8) (Price, error) {
	value, err := parseFixed(text, scale)
	if err != nil {
		return Price{}, fmt.Errorf("parse price: %w", err)
	}
	if value.atoms <= 0 {
		return Price{}, fmt.Errorf("parse price: %w", ErrNonPositive)
	}
	return Price{value: value}, nil
}

// Atoms 返回按 Scale 位小数表示的权威整数值。
func (p Price) Atoms() int64 {
	return p.value.atoms
}

// Scale 返回 Atoms 所表示的小数位数。
func (p Price) Scale() uint8 {
	return p.value.scale
}

// String 返回精确的定点十进制表示。
func (p Price) String() string {
	return p.value.String()
}
