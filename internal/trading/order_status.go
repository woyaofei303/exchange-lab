package trading

import (
	"fmt"
	"strings"
)

// OrderStatus 表示订单持久化后的生命周期状态。
type OrderStatus string

// 支持持久化的订单生命周期状态。
const (
	OrderStatusPending         OrderStatus = "pending"
	OrderStatusOpen            OrderStatus = "open"
	OrderStatusPartiallyFilled OrderStatus = "partially_filled"
	OrderStatusFilled          OrderStatus = "filled"
	OrderStatusCanceled        OrderStatus = "canceled"
	OrderStatusRejected        OrderStatus = "rejected"
)

// ParseOrderStatus 以不区分大小写的方式解析受支持状态。
func ParseOrderStatus(text string) (OrderStatus, error) {
	status := OrderStatus(strings.ToLower(text))
	switch status {
	case OrderStatusPending,
		OrderStatusOpen,
		OrderStatusPartiallyFilled,
		OrderStatusFilled,
		OrderStatusCanceled,
		OrderStatusRejected:
		return status, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidOrderStatus, text)
	}
}

// String 返回规范的小写状态。
func (s OrderStatus) String() string {
	return string(s)
}
