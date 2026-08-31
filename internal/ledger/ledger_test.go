package ledger

import (
	"errors"
	"math"
	"testing"
)

func TestNewTransactionRequiresBalancedSameAssetEntries(t *testing.T) {
	transaction, err := NewTransaction("tx-1", "faucet", "faucet-key-1", []Entry{
		{AccountID: 10, Asset: "USDT", AmountAtoms: 100_000_000},
		{AccountID: 1, Asset: "USDT", AmountAtoms: -100_000_000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if transaction.Total() != 0 || len(transaction.Entries) != 2 {
		t.Fatalf("transaction = %#v", transaction)
	}
}

func TestNewTransactionRejectsBrokenConservationAndOverflow(t *testing.T) {
	tests := []struct {
		name    string
		entries []Entry
	}{
		{"unbalanced", []Entry{{AccountID: 1, Asset: "USDT", AmountAtoms: 10}, {AccountID: 2, Asset: "USDT", AmountAtoms: -9}}},
		{"mixed asset", []Entry{{AccountID: 1, Asset: "USDT", AmountAtoms: 10}, {AccountID: 2, Asset: "BTC", AmountAtoms: -10}}},
		{"zero entry", []Entry{{AccountID: 1, Asset: "USDT", AmountAtoms: 10}, {AccountID: 2, Asset: "USDT", AmountAtoms: 0}, {AccountID: 3, Asset: "USDT", AmountAtoms: -10}}},
		{"overflow", []Entry{{AccountID: 1, Asset: "USDT", AmountAtoms: math.MaxInt64}, {AccountID: 2, Asset: "USDT", AmountAtoms: 1}, {AccountID: 3, Asset: "USDT", AmountAtoms: math.MinInt64}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewTransaction("tx", "test", "key", test.entries); !errors.Is(err, ErrUnbalanced) {
				t.Fatalf("NewTransaction() error = %v", err)
			}
		})
	}
}

func TestBalanceLockReleaseAndSpendPreserveTotal(t *testing.T) {
	balance := Balance{AvailableAtoms: 1_000, LockedAtoms: 0}
	locked, err := balance.Lock(350)
	if err != nil || locked.AvailableAtoms != 650 || locked.LockedAtoms != 350 || locked.Total() != balance.Total() {
		t.Fatalf("Lock() = %#v, %v", locked, err)
	}
	released, err := locked.Release(100)
	if err != nil || released.AvailableAtoms != 750 || released.LockedAtoms != 250 || released.Total() != balance.Total() {
		t.Fatalf("Release() = %#v, %v", released, err)
	}
	spent, err := released.SpendLocked(200)
	if err != nil || spent.AvailableAtoms != 750 || spent.LockedAtoms != 50 || spent.Total() != 800 {
		t.Fatalf("SpendLocked() = %#v, %v", spent, err)
	}
	if _, err := balance.Lock(1_001); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("Lock(overdraft) error = %v", err)
	}
}
