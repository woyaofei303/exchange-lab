package trading

import (
	"errors"
	"testing"
)

func TestParseSideNormalizesKnownValue(t *testing.T) {
	t.Parallel()

	side, err := ParseSide("BUY")
	if err != nil {
		t.Fatalf("ParseSide() error = %v", err)
	}
	if got, want := side, SideBuy; got != want {
		t.Fatalf("ParseSide() = %q, want %q", got, want)
	}
	if got, want := side.String(), "buy"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestParseSideRejectsUnknownValue(t *testing.T) {
	t.Parallel()

	_, err := ParseSide("hold")
	if !errors.Is(err, ErrInvalidSide) {
		t.Fatalf("ParseSide() error = %v, want ErrInvalidSide", err)
	}
}
