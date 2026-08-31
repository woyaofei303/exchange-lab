package trading

import (
	"errors"
	"testing"
)

func TestParseMoneyUsesAssetLedgerScale(t *testing.T) {
	t.Parallel()

	usdt, err := NewAsset("USDT", 8)
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	money, err := ParseMoney(usdt, "12.34")
	if err != nil {
		t.Fatalf("ParseMoney() error = %v", err)
	}
	if got, want := money.String(), "12.34000000 USDT"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got, want := money.Atoms(), int64(1234000000); got != want {
		t.Fatalf("Atoms() = %d, want %d", got, want)
	}
	if got, want := money.Asset(), usdt; got != want {
		t.Fatalf("Asset() = %#v, want %#v", got, want)
	}
}

func TestCalculateNotionalMultipliesPriceAndQuantityExactly(t *testing.T) {
	t.Parallel()

	usdt, err := NewAsset("USDT", 8)
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	price, err := ParsePrice("100.50", 2)
	if err != nil {
		t.Fatalf("ParsePrice() error = %v", err)
	}
	quantity, err := ParseQuantity("0.010", 3)
	if err != nil {
		t.Fatalf("ParseQuantity() error = %v", err)
	}

	notional, err := CalculateNotional(price, quantity, usdt)
	if err != nil {
		t.Fatalf("CalculateNotional() error = %v", err)
	}
	if got, want := notional.String(), "1.00500000 USDT"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestParseMoneySupportsSignedLedgerAmounts(t *testing.T) {
	t.Parallel()

	usdt := mustAsset(t, "USDT", 2)
	money, err := ParseMoney(usdt, "-12.34")
	if err != nil {
		t.Fatalf("ParseMoney() error = %v", err)
	}
	if got, want := money.String(), "-12.34 USDT"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestParseMoneyRejectsZeroValueAsset(t *testing.T) {
	t.Parallel()

	_, err := ParseMoney(Asset{}, "1")
	if !errors.Is(err, ErrInvalidAsset) {
		t.Fatalf("ParseMoney() error = %v, want ErrInvalidAsset", err)
	}
}

func TestCalculateNotionalRejectsPrecisionLoss(t *testing.T) {
	t.Parallel()

	price, err := ParsePrice("1.23", 2)
	if err != nil {
		t.Fatalf("ParsePrice() error = %v", err)
	}
	quantity, err := ParseQuantity("0.1", 1)
	if err != nil {
		t.Fatalf("ParseQuantity() error = %v", err)
	}
	_, err = CalculateNotional(price, quantity, mustAsset(t, "USDT", 2))
	if !errors.Is(err, ErrPrecisionExceeded) {
		t.Fatalf("CalculateNotional() error = %v, want ErrPrecisionExceeded", err)
	}
}

func TestCalculateNotionalRejectsOverflow(t *testing.T) {
	t.Parallel()

	price, err := ParsePrice("9223372036854775807", 0)
	if err != nil {
		t.Fatalf("ParsePrice() error = %v", err)
	}
	quantity, err := ParseQuantity("2", 0)
	if err != nil {
		t.Fatalf("ParseQuantity() error = %v", err)
	}
	_, err = CalculateNotional(price, quantity, mustAsset(t, "USDT", 0))
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("CalculateNotional() error = %v, want ErrOverflow", err)
	}
}

func TestCalculateNotionalRejectsZeroValueOperands(t *testing.T) {
	t.Parallel()

	_, err := CalculateNotional(Price{}, Quantity{}, mustAsset(t, "USDT", 8))
	if !errors.Is(err, ErrNonPositive) {
		t.Fatalf("CalculateNotional() error = %v, want ErrNonPositive", err)
	}
}
