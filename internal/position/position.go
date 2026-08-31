// Package position applies fills to one-way positions and calculates exact PnL.
package position

import (
	"errors"
	"fmt"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/fixedmath"
)

// View is the persisted position projection exposed to read boundaries.
type View struct {
	UserID                                                                                     int64
	Symbol                                                                                     string
	SizeAtoms, EntryPriceAtoms, MarkPriceAtoms, RealizedPnLAtoms, UnrealizedPnLAtoms, FeeAtoms int64
	PriceScale, QuantityScale, QuoteScale                                                      uint8
	Version                                                                                    int64
	UpdatedAt                                                                                  time.Time
}

// Position errors reject malformed fills and invalid reduce-only effects.
var (
	ErrInvalidFill = errors.New("invalid fill")
	ErrReduceOnly  = errors.New("reduce-only fill would expand or reverse position")
)

// Side is the signed direction of an applied fill.
type Side string

// Supported fill directions.
const (
	Buy  Side = "buy"
	Sell Side = "sell"
)

// State is the minimal one-way position state needed for deterministic PnL.
type State struct {
	Symbol          string
	SizeAtoms       int64
	EntryPriceAtoms int64
	QuantityScale   uint8
	PriceScale      uint8
}

// Fill describes one execution applied to a position.
type Fill struct {
	Side          Side
	PriceAtoms    int64
	QuantityAtoms int64
	ReduceOnly    bool
}

// ApplyFill updates a one-way position and returns the realized quote PnL atoms.
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

// UnrealizedPnL values a signed position against the supplied mark price.
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
