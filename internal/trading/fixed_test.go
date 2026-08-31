package trading_test

import (
	"errors"
	"testing"

	"exchange-lab/internal/trading"
)

func TestFormatDecimalPreservesAtomsAndScale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		atoms int64
		scale uint8
		want  string
	}{
		{name: "integer", atoms: 5, scale: 0, want: "5"},
		{name: "leading zero", atoms: 10, scale: 2, want: "0.10"},
		{name: "ledger scale", atoms: 500000000, scale: 8, want: "5.00000000"},
		{name: "negative", atoms: -125, scale: 2, want: "-1.25"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := trading.FormatDecimal(tt.atoms, tt.scale)
			if err != nil {
				t.Fatalf("FormatDecimal() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("FormatDecimal(%d, %d) = %q, want %q", tt.atoms, tt.scale, got, tt.want)
			}
		})
	}
}

func TestFormatDecimalRejectsUnsupportedScale(t *testing.T) {
	t.Parallel()

	if _, err := trading.FormatDecimal(1, 19); !errors.Is(err, trading.ErrInvalidScale) {
		t.Fatalf("FormatDecimal() error = %v, want ErrInvalidScale", err)
	}
}
