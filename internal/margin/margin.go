// Package margin calculates isolated and cross-margin risk with integer atoms.
package margin

import (
	"errors"
	"github.com/woyaofei303/exchange-lab/internal/fixedmath"
	"github.com/woyaofei303/exchange-lab/internal/position"
)

// Margin errors classify invalid snapshots, tier breaches and insufficient collateral.
var (
	ErrInvalidRisk        = errors.New("invalid margin risk input")
	ErrLeverage           = errors.New("leverage exceeds tier")
	ErrRiskLimit          = errors.New("risk limit exceeded")
	ErrInsufficientMargin = errors.New("insufficient margin")
)

// Tier defines the notional cap, leverage cap and maintenance requirement.
type Tier struct {
	NotionalCapAtoms     int64
	MaxLeverage          int64
	MaintenanceMarginBPS int64
}

// Input contains exact account and position values for isolated evaluation.
type Input struct {
	WalletBalanceAtoms, PositionSizeAtoms, EntryPriceAtoms, MarkPriceAtoms int64
	QuantityScale                                                          uint8
	Leverage                                                               int64
	OrderMarginAtoms                                                       int64
}

// IsolatedSnapshot is the complete result of one isolated-margin evaluation.
type IsolatedSnapshot struct{ NotionalAtoms, UnrealizedPnLAtoms, InitialMarginAtoms, MaintenanceMarginAtoms, MarginBalanceAtoms, AvailableBalanceAtoms int64 }

// EvaluateIsolated calculates margin requirements without mutating account state.
func EvaluateIsolated(in Input, tier Tier) (IsolatedSnapshot, error) {
	if in.WalletBalanceAtoms < 0 || in.MarkPriceAtoms <= 0 || in.Leverage <= 0 || in.OrderMarginAtoms < 0 || tier.NotionalCapAtoms <= 0 || tier.MaxLeverage <= 0 || tier.MaintenanceMarginBPS <= 0 {
		return IsolatedSnapshot{}, ErrInvalidRisk
	}
	if in.Leverage > tier.MaxLeverage {
		return IsolatedSnapshot{}, ErrLeverage
	}
	factor, err := fixedmath.Pow10(in.QuantityScale)
	if err != nil {
		return IsolatedSnapshot{}, ErrInvalidRisk
	}
	absSize, err := fixedmath.Abs(in.PositionSizeAtoms)
	if err != nil {
		return IsolatedSnapshot{}, ErrInvalidRisk
	}
	notional, err := fixedmath.MulDivCeil(absSize, in.MarkPriceAtoms, factor)
	if err != nil {
		return IsolatedSnapshot{}, ErrInvalidRisk
	}
	if notional > tier.NotionalCapAtoms {
		return IsolatedSnapshot{}, ErrRiskLimit
	}
	im, err := fixedmath.MulDivCeil(notional, 1, in.Leverage)
	if err != nil {
		return IsolatedSnapshot{}, ErrInvalidRisk
	}
	mm, err := fixedmath.MulDivCeil(notional, tier.MaintenanceMarginBPS, 10_000)
	if err != nil {
		return IsolatedSnapshot{}, ErrInvalidRisk
	}
	upnl, err := position.UnrealizedPnL(position.State{SizeAtoms: in.PositionSizeAtoms, EntryPriceAtoms: in.EntryPriceAtoms, QuantityScale: in.QuantityScale}, in.MarkPriceAtoms)
	if err != nil {
		return IsolatedSnapshot{}, ErrInvalidRisk
	}
	marginBalance := in.WalletBalanceAtoms + upnl
	available := marginBalance - in.OrderMarginAtoms
	return IsolatedSnapshot{NotionalAtoms: notional, UnrealizedPnLAtoms: upnl, InitialMarginAtoms: im, MaintenanceMarginAtoms: mm, MarginBalanceAtoms: marginBalance, AvailableBalanceAtoms: available}, nil
}

// CheckOrder verifies a new reserve against available collateral and a positive version change.
func CheckOrder(available, alreadyReserved, required, versionDelta int64) error {
	if available < 0 || alreadyReserved < 0 || required < 0 || versionDelta <= 0 {
		return ErrInvalidRisk
	}
	if required > available-alreadyReserved {
		return ErrInsufficientMargin
	}
	return nil
}

// PositionRisk is one position's contribution to a cross-margin snapshot.
type PositionRisk struct{ UnrealizedPnLAtoms, MaintenanceMarginAtoms int64 }

// CrossSnapshot aggregates shared collateral and all position requirements.
type CrossSnapshot struct {
	MarginBalanceAtoms, MaintenanceMarginAtoms, OrderMarginAtoms, AvailableBalanceAtoms int64
	Version                                                                             int64
}

// EvaluateCross builds a versioned shared-collateral risk snapshot.
func EvaluateCross(wallet, orderMargin, version int64, positions []PositionRisk) (CrossSnapshot, error) {
	if wallet < 0 || orderMargin < 0 || version <= 0 {
		return CrossSnapshot{}, ErrInvalidRisk
	}
	result := CrossSnapshot{MarginBalanceAtoms: wallet, OrderMarginAtoms: orderMargin, Version: version}
	for _, p := range positions {
		if p.MaintenanceMarginAtoms < 0 {
			return CrossSnapshot{}, ErrInvalidRisk
		}
		result.MarginBalanceAtoms += p.UnrealizedPnLAtoms
		result.MaintenanceMarginAtoms += p.MaintenanceMarginAtoms
	}
	result.AvailableBalanceAtoms = result.MarginBalanceAtoms - orderMargin
	return result, nil
}

// SameVersion reports whether a snapshot is still valid for an expected risk version.
func SameVersion(snapshot CrossSnapshot, expected int64) bool { return snapshot.Version == expected }
