package trading

import (
	"fmt"
	"strings"
)

// OrderType identifies how an order is priced.
type OrderType string

// Supported public order-type tokens; the v1 matching path accepts limit only.
const (
	OrderTypeLimit  OrderType = "limit"
	OrderTypeMarket OrderType = "market"
)

// ParseOrderType parses limit or market case-insensitively.
func ParseOrderType(text string) (OrderType, error) {
	orderType := OrderType(strings.ToLower(text))
	switch orderType {
	case OrderTypeLimit, OrderTypeMarket:
		return orderType, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidOrderType, text)
	}
}

// String returns the canonical lowercase order type.
func (o OrderType) String() string {
	return string(o)
}
