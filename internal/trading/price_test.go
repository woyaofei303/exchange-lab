package trading

import (
	"errors"
	"testing"
)

func TestParsePricePreservesExactDecimalValue(t *testing.T) {
	t.Parallel()

	price, err := ParsePrice("123.45", 2)
	if err != nil {
		t.Fatalf("ParsePrice() error = %v", err)
	}
	if got, want := price.String(), "123.45"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got, want := price.Atoms(), int64(12345); got != want {
		t.Fatalf("Atoms() = %d, want %d", got, want)
	}
	if got, want := price.Scale(), uint8(2); got != want {
		t.Fatalf("Scale() = %d, want %d", got, want)
	}
}

func TestParsePricePadsToConfiguredScale(t *testing.T) {
	t.Parallel()

	price, err := ParsePrice("12.3", 4)
	if err != nil {
		t.Fatalf("ParsePrice() error = %v", err)
	}
	if got, want := price.String(), "12.3000"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestParsePriceRejectsExcessPrecision(t *testing.T) {
	t.Parallel()

	_, err := ParsePrice("12.345", 2)
	if !errors.Is(err, ErrPrecisionExceeded) {
		t.Fatalf("ParsePrice() error = %v, want ErrPrecisionExceeded", err)
	}
}

func TestParsePriceRejectsNonPositiveValue(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"0", "-0.01"} {
		value := value
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			_, err := ParsePrice(value, 2)
			if !errors.Is(err, ErrNonPositive) {
				t.Fatalf("ParsePrice(%q) error = %v, want ErrNonPositive", value, err)
			}
		})
	}
}

func TestParsePriceRejectsMalformedDecimal(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", " 1.00", "+1.00", "1.", ".1", "1e2", "1.2.3"} {
		value := value
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			_, err := ParsePrice(value, 2)
			if !errors.Is(err, ErrInvalidNumber) {
				t.Fatalf("ParsePrice(%q) error = %v, want ErrInvalidNumber", value, err)
			}
		})
	}
}

func TestParsePriceRejectsAtomOverflow(t *testing.T) {
	t.Parallel()

	_, err := ParsePrice("92233720368547758.08", 2)
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("ParsePrice() error = %v, want ErrOverflow", err)
	}
}
