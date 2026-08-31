package trading

import "fmt"

// Quantity is an exact fixed-point order quantity.
type Quantity struct {
	value fixed
}

// ParseQuantity parses a positive decimal quantity at the requested scale.
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

// Atoms returns the integer source-of-truth value at Scale decimal places.
func (q Quantity) Atoms() int64 {
	return q.value.atoms
}

// Scale returns the number of decimal places represented by Atoms.
func (q Quantity) Scale() uint8 {
	return q.value.scale
}

// String returns the exact fixed-scale decimal representation.
func (q Quantity) String() string {
	return q.value.String()
}
