package trading

import (
	"errors"
	"testing"
)

func TestNewAssetNormalizesCode(t *testing.T) {
	t.Parallel()

	asset, err := NewAsset("usdt", 8)
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	if got, want := asset.Code(), "USDT"; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
	if got, want := asset.Scale(), uint8(8); got != want {
		t.Fatalf("Scale() = %d, want %d", got, want)
	}
}

func TestNewAssetRejectsInvalidCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
	}{
		{name: "empty", code: ""},
		{name: "one character", code: "U"},
		{name: "whitespace", code: "US DT"},
		{name: "punctuation", code: "USD-T"},
		{name: "too long", code: "ABCDEFGHIJKLM"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewAsset(tt.code, 8)
			if !errors.Is(err, ErrInvalidAsset) {
				t.Fatalf("NewAsset(%q) error = %v, want ErrInvalidAsset", tt.code, err)
			}
		})
	}
}

func TestNewAssetRejectsUnsupportedLedgerScale(t *testing.T) {
	t.Parallel()

	_, err := NewAsset("USDT", 19)
	if !errors.Is(err, ErrInvalidScale) {
		t.Fatalf("NewAsset() error = %v, want ErrInvalidScale", err)
	}
}
