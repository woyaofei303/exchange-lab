// Package liquidation evaluates mark-price risk and its idempotent state machine.
package liquidation

import (
	"errors"
	"fmt"
	"github.com/woyaofei303/exchange-lab/internal/fixedmath"
)

// Liquidation errors distinguish invalid risk input from illegal state transitions.
var (
	ErrInvalidSnapshot = errors.New("invalid liquidation snapshot")
	ErrTransition      = errors.New("invalid liquidation transition")
)

// Snapshot is the immutable mark-price risk input for one position version.
type Snapshot struct {
	UserID                                                                                         int64
	Symbol                                                                                         string
	PositionSizeAtoms, EntryPriceAtoms, MarkPriceAtoms, MarginBalanceAtoms, MaintenanceMarginAtoms int64
	QuantityScale                                                                                  uint8
	RiskVersion                                                                                    int64
}

// Decision identifies whether liquidation is required and its idempotency key.
type Decision struct {
	Required             bool
	JobKey               string
	BankruptcyPriceAtoms int64
	Reason               string
}

// Evaluate compares margin balance with maintenance and derives bankruptcy price.
func Evaluate(s Snapshot) (Decision, error) {
	if s.UserID <= 0 || s.Symbol == "" || s.PositionSizeAtoms == 0 || s.EntryPriceAtoms <= 0 || s.MarkPriceAtoms <= 0 || s.MarginBalanceAtoms < 0 || s.MaintenanceMarginAtoms < 0 || s.RiskVersion <= 0 {
		return Decision{}, ErrInvalidSnapshot
	}
	abs, err := fixedmath.Abs(s.PositionSizeAtoms)
	if err != nil {
		return Decision{}, ErrInvalidSnapshot
	}
	factor, err := fixedmath.Pow10(s.QuantityScale)
	if err != nil {
		return Decision{}, ErrInvalidSnapshot
	}
	distance, err := fixedmath.MulDiv(s.MarginBalanceAtoms, factor, abs)
	if err != nil {
		return Decision{}, ErrInvalidSnapshot
	}
	bankruptcy := s.EntryPriceAtoms - distance
	if s.PositionSizeAtoms < 0 {
		bankruptcy = s.EntryPriceAtoms + distance
	}
	if bankruptcy <= 0 {
		bankruptcy = 1
	}
	required := s.MarginBalanceAtoms <= s.MaintenanceMarginAtoms
	return Decision{Required: required, JobKey: fmt.Sprintf("liq:%d:%s:%d", s.UserID, s.Symbol, s.RiskVersion), BankruptcyPriceAtoms: bankruptcy, Reason: "mark_price_margin_ratio"}, nil
}

// State is the durable liquidation job lifecycle.
type State string

// Supported liquidation states.
const (
	StatePending   State = "pending"
	StateExecuting State = "executing"
	StateSettling  State = "settling"
	StateCompleted State = "completed"
	StateParked    State = "parked"
)

// Event is an accepted liquidation state-machine input.
type Event string

// Supported liquidation transition events.
const (
	EventStarted Event = "started"
	EventFilled  Event = "filled"
	EventSettled Event = "settled"
	EventFailed  Event = "failed"
)

// Transition applies one valid liquidation event or returns ErrTransition.
func Transition(state State, event Event) (State, error) {
	switch {
	case state == StatePending && event == EventStarted:
		return StateExecuting, nil
	case state == StateExecuting && event == EventFilled:
		return StateSettling, nil
	case state == StateSettling && event == EventSettled:
		return StateCompleted, nil
	case event == EventFailed && (state == StatePending || state == StateExecuting || state == StateSettling):
		return StateParked, nil
	default:
		return "", ErrTransition
	}
}
