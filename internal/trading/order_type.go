package trading

import (
	"fmt"
	"strings"
)

// OrderType 标识订单的定价方式。
type OrderType string

// 支持的公开订单类型；v1 撮合流程仅接受限价单。
const (
	OrderTypeLimit  OrderType = "limit"
	OrderTypeMarket OrderType = "market"
)

// ParseOrderType 以不区分大小写的方式解析限价单或市价单。
func ParseOrderType(text string) (OrderType, error) {
	orderType := OrderType(strings.ToLower(text))
	switch orderType {
	case OrderTypeLimit, OrderTypeMarket:
		return orderType, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidOrderType, text)
	}
}

// String 返回规范的小写订单类型。
func (o OrderType) String() string {
	return string(o)
}
