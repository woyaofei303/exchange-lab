package trading

import "fmt"

// Price is an exact fixed-point quote price. It never uses binary floating point.
type Price struct {
	value fixed
}

// ParsePrice parses a positive decimal price at the requested scale.
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

// Atoms returns the integer source-of-truth value at Scale decimal places.
func (p Price) Atoms() int64 {
	return p.value.atoms
}

// Scale returns the number of decimal places represented by Atoms.
func (p Price) Scale() uint8 {
	return p.value.scale
}

// String returns the exact fixed-scale decimal representation.
func (p Price) String() string {
	return p.value.String()
}
