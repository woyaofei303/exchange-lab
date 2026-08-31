package trading

import (
	"errors"
	"testing"
)

func TestParseQuantityPreservesExactDecimalValue(t *testing.T) {
	t.Parallel()

	quantity, err := ParseQuantity("0.001", 3)
	if err != nil {
		t.Fatalf("ParseQuantity() error = %v", err)
	}
	if got, want := quantity.String(), "0.001"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got, want := quantity.Atoms(), int64(1); got != want {
		t.Fatalf("Atoms() = %d, want %d", got, want)
	}
	if got, want := quantity.Scale(), uint8(3); got != want {
		t.Fatalf("Scale() = %d, want %d", got, want)
	}
}

func TestParseQuantityRejectsExcessPrecision(t *testing.T) {
	t.Parallel()

	_, err := ParseQuantity("0.0001", 3)
	if !errors.Is(err, ErrPrecisionExceeded) {
		t.Fatalf("ParseQuantity() error = %v, want ErrPrecisionExceeded", err)
	}
}

func TestParseQuantityRejectsNonPositiveValue(t *testing.T) {
	t.Parallel()

	_, err := ParseQuantity("0.000", 3)
	if !errors.Is(err, ErrNonPositive) {
		t.Fatalf("ParseQuantity() error = %v, want ErrNonPositive", err)
	}
}
