// Package ledger enforces exact, balanced money movements and balance states.
package ledger

import (
	"errors"
	"fmt"
	"math"
	"regexp"
)

// Ledger errors classify invariant, balance and idempotency failures.
var (
	ErrUnbalanced   = errors.New("ledger transaction is unbalanced")
	ErrInvalid      = errors.New("invalid ledger value")
	ErrInsufficient = errors.New("insufficient balance")
	ErrConflict     = errors.New("ledger idempotency conflict")
)

var assetPattern = regexp.MustCompile(`^[A-Z0-9]{2,12}$`)

// Entry is one signed account movement in an exact asset unit.
type Entry struct {
	AccountID   int64
	Asset       string
	AmountAtoms int64
}

// Transaction is an immutable, single-asset set of zero-sum entries.
type Transaction struct {
	ID             string
	Kind           string
	IdempotencyKey string
	Entries        []Entry
}

// FaucetResult is returned by an idempotent simulated-funds credit. Replayed
// calls return the original transaction identifier and the current balance.
type FaucetResult struct {
	TransactionID string
	Balance       Balance
	Replayed      bool
}

// NewTransaction copies and validates entries, including int64-safe zero balance.
func NewTransaction(id, kind, idempotencyKey string, entries []Entry) (Transaction, error) {
	if id == "" || len(id) > 128 || kind == "" || len(kind) > 64 || idempotencyKey == "" || len(idempotencyKey) > 128 || len(entries) < 2 || len(entries) > 100 {
		return Transaction{}, ErrInvalid
	}
	asset := entries[0].Asset
	if !assetPattern.MatchString(asset) {
		return Transaction{}, ErrInvalid
	}
	var total int64
	copyEntries := append([]Entry(nil), entries...)
	for _, entry := range copyEntries {
		if entry.AccountID <= 0 || entry.Asset != asset || entry.AmountAtoms == 0 {
			return Transaction{}, ErrUnbalanced
		}
		if (entry.AmountAtoms > 0 && total > math.MaxInt64-entry.AmountAtoms) || (entry.AmountAtoms < 0 && total < math.MinInt64-entry.AmountAtoms) {
			return Transaction{}, ErrUnbalanced
		}
		total += entry.AmountAtoms
	}
	if total != 0 {
		return Transaction{}, ErrUnbalanced
	}
	return Transaction{ID: id, Kind: kind, IdempotencyKey: idempotencyKey, Entries: copyEntries}, nil
}

// Total returns the signed entry sum, which is zero for a valid transaction.
func (t Transaction) Total() int64 {
	var total int64
	for _, entry := range t.Entries {
		total += entry.AmountAtoms
	}
	return total
}

// Balance is a non-negative materialized split between available and locked atoms.
type Balance struct {
	AvailableAtoms int64
	LockedAtoms    int64
}

// Total returns available plus locked atoms.
func (b Balance) Total() int64 {
	return b.AvailableAtoms + b.LockedAtoms
}

// Valid reports whether both components and their sum fit ledger invariants.
func (b Balance) Valid() bool {
	return b.valid()
}

// Lock moves available atoms into the locked component.
func (b Balance) Lock(amount int64) (Balance, error) {
	if !b.valid() || amount <= 0 || amount > b.AvailableAtoms || b.LockedAtoms > math.MaxInt64-amount {
		return Balance{}, ErrInsufficient
	}
	return Balance{AvailableAtoms: b.AvailableAtoms - amount, LockedAtoms: b.LockedAtoms + amount}, nil
}

// Release moves locked atoms back into the available component.
func (b Balance) Release(amount int64) (Balance, error) {
	if !b.valid() || amount <= 0 || amount > b.LockedAtoms || b.AvailableAtoms > math.MaxInt64-amount {
		return Balance{}, ErrInsufficient
	}
	return Balance{AvailableAtoms: b.AvailableAtoms + amount, LockedAtoms: b.LockedAtoms - amount}, nil
}

// SpendLocked consumes reserved atoms without crediting available balance.
func (b Balance) SpendLocked(amount int64) (Balance, error) {
	if !b.valid() || amount <= 0 || amount > b.LockedAtoms {
		return Balance{}, ErrInsufficient
	}
	return Balance{AvailableAtoms: b.AvailableAtoms, LockedAtoms: b.LockedAtoms - amount}, nil
}

// Credit increases available balance while rejecting overflow.
func (b Balance) Credit(amount int64) (Balance, error) {
	if !b.valid() || amount <= 0 || b.AvailableAtoms > math.MaxInt64-amount {
		return Balance{}, fmt.Errorf("credit balance: %w", ErrInvalid)
	}
	return Balance{AvailableAtoms: b.AvailableAtoms + amount, LockedAtoms: b.LockedAtoms}, nil
}

func (b Balance) valid() bool {
	return b.AvailableAtoms >= 0 && b.LockedAtoms >= 0 && b.AvailableAtoms <= math.MaxInt64-b.LockedAtoms
}
