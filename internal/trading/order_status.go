package trading

import (
	"fmt"
	"strings"
)

// OrderStatus is the persisted lifecycle state of an order.
type OrderStatus string

// Supported persisted order lifecycle states.
const (
	OrderStatusPending         OrderStatus = "pending"
	OrderStatusOpen            OrderStatus = "open"
	OrderStatusPartiallyFilled OrderStatus = "partially_filled"
	OrderStatusFilled          OrderStatus = "filled"
	OrderStatusCanceled        OrderStatus = "canceled"
	OrderStatusRejected        OrderStatus = "rejected"
)

// ParseOrderStatus parses a supported status case-insensitively.
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

// String returns the canonical lowercase status.
func (s OrderStatus) String() string {
	return string(s)
}
