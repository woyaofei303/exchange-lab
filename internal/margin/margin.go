// Package margin 使用整数最小单位计算逐仓和全仓风险。
package margin

import (
	"errors"
	"exchange-lab/internal/fixedmath"
	"exchange-lab/internal/position"
)

// 保证金错误按无效快照、档位超限和抵押品不足分类。
var (
	ErrInvalidRisk        = errors.New("invalid margin risk input")
	ErrLeverage           = errors.New("leverage exceeds tier")
	ErrRiskLimit          = errors.New("risk limit exceeded")
	ErrInsufficientMargin = errors.New("insufficient margin")
)

// Tier 定义名义价值上限、杠杆上限和维持保证金要求。
type Tier struct {
	NotionalCapAtoms     int64
	MaxLeverage          int64
	MaintenanceMarginBPS int64
}

// Input 包含逐仓评估所需的精确账户和仓位值。
type Input struct {
	WalletBalanceAtoms, PositionSizeAtoms, EntryPriceAtoms, MarkPriceAtoms int64
	QuantityScale                                                          uint8
	Leverage                                                               int64
	OrderMarginAtoms                                                       int64
}

// IsolatedSnapshot 是一次逐仓保证金评估的完整结果。
type IsolatedSnapshot struct{ NotionalAtoms, UnrealizedPnLAtoms, InitialMarginAtoms, MaintenanceMarginAtoms, MarginBalanceAtoms, AvailableBalanceAtoms int64 }

// EvaluateIsolated 在不修改账户状态的情况下计算保证金要求。
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

// CheckOrder 根据可用抵押品和正向版本变更校验新的资金预留。
func CheckOrder(available, alreadyReserved, required, versionDelta int64) error {
	if available < 0 || alreadyReserved < 0 || required < 0 || versionDelta <= 0 {
		return ErrInvalidRisk
	}
	if required > available-alreadyReserved {
		return ErrInsufficientMargin
	}
	return nil
}

// PositionRisk 表示一个仓位对全仓保证金快照的贡献。
type PositionRisk struct{ UnrealizedPnLAtoms, MaintenanceMarginAtoms int64 }

// CrossSnapshot 汇总共享抵押品和所有仓位要求。
type CrossSnapshot struct {
	MarginBalanceAtoms, MaintenanceMarginAtoms, OrderMarginAtoms, AvailableBalanceAtoms int64
	Version                                                                             int64
}

// EvaluateCross 构建带版本的共享抵押品风险快照。
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

// SameVersion 表示快照对预期风险版本是否仍然有效。
func SameVersion(snapshot CrossSnapshot, expected int64) bool { return snapshot.Version == expected }
