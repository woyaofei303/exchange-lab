package trading

import (
	"errors"
	"testing"
)

func TestParseOrderTypeNormalizesKnownValue(t *testing.T) {
	t.Parallel()

	orderType, err := ParseOrderType("LIMIT")
	if err != nil {
		t.Fatalf("ParseOrderType() error = %v", err)
	}
	if got, want := orderType, OrderTypeLimit; got != want {
		t.Fatalf("ParseOrderType() = %q, want %q", got, want)
	}
	if got, want := orderType.String(), "limit"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestParseOrderTypeRejectsUnknownValue(t *testing.T) {
	t.Parallel()

	_, err := ParseOrderType("stop")
	if !errors.Is(err, ErrInvalidOrderType) {
		t.Fatalf("ParseOrderType() error = %v, want ErrInvalidOrderType", err)
	}
}
