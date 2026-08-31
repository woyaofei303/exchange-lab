// Package liquidation 评估标记价格风险及其幂等状态机。
package liquidation

import (
	"errors"
	"exchange-lab/internal/fixedmath"
	"fmt"
)

// 清算错误会区分无效风险输入和非法状态转换。
var (
	ErrInvalidSnapshot = errors.New("invalid liquidation snapshot")
	ErrTransition      = errors.New("invalid liquidation transition")
)

// Snapshot 是某一仓位版本不可变的标记价格风险输入。
type Snapshot struct {
	UserID                                                                                         int64
	Symbol                                                                                         string
	PositionSizeAtoms, EntryPriceAtoms, MarkPriceAtoms, MarginBalanceAtoms, MaintenanceMarginAtoms int64
	QuantityScale                                                                                  uint8
	RiskVersion                                                                                    int64
}

// Decision 标识是否需要清算及其幂等键。
type Decision struct {
	Required             bool
	JobKey               string
	BankruptcyPriceAtoms int64
	Reason               string
}

// Evaluate 比较保证金余额与维持保证金，并计算破产价格。
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

// State 表示持久化清算任务的生命周期。
type State string

// 支持的清算状态。
const (
	StatePending   State = "pending"
	StateExecuting State = "executing"
	StateSettling  State = "settling"
	StateCompleted State = "completed"
	StateParked    State = "parked"
)

// Event 表示清算状态机接受的输入。
type Event string

// 支持的清算状态转换事件。
const (
	EventStarted Event = "started"
	EventFilled  Event = "filled"
	EventSettled Event = "settled"
	EventFailed  Event = "failed"
)

// Transition 应用一个有效清算事件，否则返回 ErrTransition。
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
