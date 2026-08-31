// Package position 将成交应用到单向持仓并计算精确盈亏。
package position

import (
	"errors"
	"fmt"
	"time"

	"exchange-lab/internal/fixedmath"
)

// View 是向读取边界公开的持久化仓位投影。
type View struct {
	UserID                                                                                     int64
	Symbol                                                                                     string
	SizeAtoms, EntryPriceAtoms, MarkPriceAtoms, RealizedPnLAtoms, UnrealizedPnLAtoms, FeeAtoms int64
	PriceScale, QuantityScale, QuoteScale                                                      uint8
	Version                                                                                    int64
	UpdatedAt                                                                                  time.Time
}

// 仓位错误用于拒绝格式错误的成交和无效的只减仓结果。
var (
	ErrInvalidFill = errors.New("invalid fill")
	ErrReduceOnly  = errors.New("reduce-only fill would expand or reverse position")
)

// Side 表示已应用成交的有符号方向。
type Side string

// 支持的成交方向。
const (
	Buy  Side = "buy"
	Sell Side = "sell"
)

// State 是确定性计算盈亏所需的最小单向持仓状态。
type State struct {
	Symbol          string
	SizeAtoms       int64
	EntryPriceAtoms int64
	QuantityScale   uint8
	PriceScale      uint8
}

// Fill 描述应用到仓位的一笔成交。
type Fill struct {
	Side          Side
	PriceAtoms    int64
	QuantityAtoms int64
	ReduceOnly    bool
}

// ApplyFill 更新单向持仓并返回已实现的报价资产盈亏最小单位。
func ApplyFill(current State, fill Fill) (State, int64, error) {
	if (fill.Side != Buy && fill.Side != Sell) || fill.PriceAtoms <= 0 || fill.QuantityAtoms <= 0 || current.QuantityScale > 18 || current.PriceScale > 18 {
		return State{}, 0, ErrInvalidFill
	}
	delta := fill.QuantityAtoms
	if fill.Side == Sell {
		delta = -delta
	}
	if fill.ReduceOnly && (current.SizeAtoms == 0 || sameSign(current.SizeAtoms, delta) || fill.QuantityAtoms > mustAbs(current.SizeAtoms)) {
		return State{}, 0, ErrReduceOnly
	}
	if current.SizeAtoms == 0 {
		return State{Symbol: current.Symbol, SizeAtoms: delta, EntryPriceAtoms: fill.PriceAtoms, QuantityScale: current.QuantityScale, PriceScale: current.PriceScale}, 0, nil
	}
	if sameSign(current.SizeAtoms, delta) {
		oldAbs, newAbs := mustAbs(current.SizeAtoms), mustAbs(delta)
		weighted, err := fixedmath.MulDiv(current.EntryPriceAtoms, oldAbs, oldAbs+newAbs)
		if err != nil {
			return State{}, 0, ErrInvalidFill
		}
		addition, err := fixedmath.MulDiv(fill.PriceAtoms, newAbs, oldAbs+newAbs)
		if err != nil {
			return State{}, 0, ErrInvalidFill
		}
		current.SizeAtoms += delta
		current.EntryPriceAtoms = weighted + addition
		return current, 0, nil
	}
	oldAbs, deltaAbs := mustAbs(current.SizeAtoms), mustAbs(delta)
	closeQty := deltaAbs
	if oldAbs < closeQty {
		closeQty = oldAbs
	}
	factor, err := fixedmath.Pow10(current.QuantityScale)
	if err != nil {
		return State{}, 0, ErrInvalidFill
	}
	difference := fill.PriceAtoms - current.EntryPriceAtoms
	if current.SizeAtoms < 0 {
		difference = -difference
	}
	realized, err := fixedmath.MulDiv(difference, closeQty, factor)
	if err != nil {
		return State{}, 0, ErrInvalidFill
	}
	newSize := current.SizeAtoms + delta
	current.SizeAtoms = newSize
	switch {
	case newSize == 0:
		current.EntryPriceAtoms = 0
	case sameSign(newSize, delta):
		current.EntryPriceAtoms = fill.PriceAtoms
	}
	return current, realized, nil
}

// UnrealizedPnL 根据给定标记价格计算有符号仓位的未实现盈亏。
func UnrealizedPnL(state State, markPriceAtoms int64) (int64, error) {
	if state.SizeAtoms == 0 {
		return 0, nil
	}
	if state.EntryPriceAtoms <= 0 || markPriceAtoms <= 0 {
		return 0, ErrInvalidFill
	}
	factor, err := fixedmath.Pow10(state.QuantityScale)
	if err != nil {
		return 0, err
	}
	diff := markPriceAtoms - state.EntryPriceAtoms
	return fixedmath.MulDiv(diff, state.SizeAtoms, factor)
}

func sameSign(a, b int64) bool { return (a > 0 && b > 0) || (a < 0 && b < 0) }
func mustAbs(v int64) int64 {
	a, err := fixedmath.Abs(v)
	if err != nil {
		panic(fmt.Sprintf("position invariant: %v", err))
	}
	return a
}
