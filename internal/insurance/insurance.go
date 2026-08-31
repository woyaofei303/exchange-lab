// Package insurance covers bankruptcy loss and ranks auditable ADL candidates.
package insurance

import (
	"errors"
	"github.com/woyaofei303/exchange-lab/internal/fixedmath"
	"math/big"
	"sort"
)

// ErrInvalidInsurance reports malformed fund, ADL or conditional-order input.
var ErrInvalidInsurance = errors.New("invalid insurance input")

// CoverResult splits a bankruptcy loss between insurance and residual ADL evidence.
type CoverResult struct{ InsuranceDebitAtoms, ADLShortfallAtoms, RemainingFundAtoms int64 }

// CoverLoss debits no more than the available insurance balance.
func CoverLoss(fund, loss int64) (CoverResult, error) {
	if fund < 0 || loss < 0 {
		return CoverResult{}, ErrInvalidInsurance
	}
	debit := loss
	if debit > fund {
		debit = fund
	}
	return CoverResult{InsuranceDebitAtoms: debit, ADLShortfallAtoms: loss - debit, RemainingFundAtoms: fund - debit}, nil
}

// Candidate is ranked by profit times leverage for simplified ADL evidence.
type Candidate struct{ UserID, ProfitAtoms, Leverage int64 }

// RankADL returns a detached deterministic ranking with user ID as the tie-breaker.
func RankADL(items []Candidate) ([]Candidate, error) {
	result := append([]Candidate(nil), items...)
	for _, v := range result {
		if v.UserID <= 0 || v.ProfitAtoms < 0 || v.Leverage <= 0 {
			return nil, ErrInvalidInsurance
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		a := new(big.Int).Mul(big.NewInt(result[i].ProfitAtoms), big.NewInt(result[i].Leverage))
		b := new(big.Int).Mul(big.NewInt(result[j].ProfitAtoms), big.NewInt(result[j].Leverage))
		cmp := a.Cmp(b)
		if cmp == 0 {
			return result[i].UserID < result[j].UserID
		}
		return cmp > 0
	})
	return result, nil
}

// Conditional describes a reduce-only price trigger for an existing position.
type Conditional struct {
	ID                                   string
	PositionSizeAtoms, TriggerPriceAtoms int64
	TriggerDirection, TriggerSource      string
}

// TriggeredOrder is the safe reduce-only action derived from a condition.
type TriggeredOrder struct {
	Triggered     bool
	Side          string
	QuantityAtoms int64
	ReduceOnly    bool
}

// TriggerConditional evaluates one price and never produces a position-expanding order.
func TriggerConditional(condition Conditional, currentPriceAtoms int64) (TriggeredOrder, error) {
	if condition.ID == "" || condition.PositionSizeAtoms == 0 || condition.TriggerPriceAtoms <= 0 || currentPriceAtoms <= 0 || (condition.TriggerSource != "mark" && condition.TriggerSource != "last") || (condition.TriggerDirection != "lte" && condition.TriggerDirection != "gte") {
		return TriggeredOrder{}, ErrInvalidInsurance
	}
	triggered := (condition.TriggerDirection == "lte" && currentPriceAtoms <= condition.TriggerPriceAtoms) || (condition.TriggerDirection == "gte" && currentPriceAtoms >= condition.TriggerPriceAtoms)
	if !triggered {
		return TriggeredOrder{}, nil
	}
	side := "sell"
	if condition.PositionSizeAtoms < 0 {
		side = "buy"
	}
	quantity, err := fixedmath.Abs(condition.PositionSizeAtoms)
	if err != nil {
		return TriggeredOrder{}, ErrInvalidInsurance
	}
	return TriggeredOrder{Triggered: true, Side: side, QuantityAtoms: quantity, ReduceOnly: true}, nil
}
