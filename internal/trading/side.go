package trading

import (
	"fmt"
	"strings"
)

// Side identifies whether an order buys or sells the base asset.
type Side string

// Supported canonical order sides.
const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

// ParseSide parses buy or sell case-insensitively.
func ParseSide(text string) (Side, error) {
	side := Side(strings.ToLower(text))
	switch side {
	case SideBuy, SideSell:
		return side, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidSide, text)
	}
}

// String returns the canonical lowercase side.
func (s Side) String() string {
	return string(s)
}
