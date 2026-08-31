package trading

import (
	"fmt"
	"strings"
)

// Side 标识订单买入还是卖出基础资产。
type Side string

// 支持的规范订单方向。
const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

// ParseSide 以不区分大小写的方式解析买入或卖出方向。
func ParseSide(text string) (Side, error) {
	side := Side(strings.ToLower(text))
	switch side {
	case SideBuy, SideSell:
		return side, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidSide, text)
	}
}

// String 返回规范的小写订单方向。
func (s Side) String() string {
	return string(s)
}
