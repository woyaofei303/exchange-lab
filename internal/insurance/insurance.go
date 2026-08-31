// Package insurance 弥补破产损失，并对可审计的 ADL 候选者排序。
package insurance

import (
	"errors"
	"exchange-lab/internal/fixedmath"
	"math/big"
	"sort"
)

// ErrInvalidInsurance 表示基金、ADL 或条件单输入格式错误。
var ErrInvalidInsurance = errors.New("invalid insurance input")

// CoverResult 将破产损失拆分为保险基金承担部分和剩余 ADL 证据。
type CoverResult struct{ InsuranceDebitAtoms, ADLShortfallAtoms, RemainingFundAtoms int64 }

// CoverLoss 扣减的金额不会超过可用保险基金余额。
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

// Candidate 按利润乘杠杆排序，作为简化的 ADL 证据。
type Candidate struct{ UserID, ProfitAtoms, Leverage int64 }

// RankADL 返回独立的确定性排序，并使用用户 ID 打破平局。
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

// Conditional 描述现有仓位的只减仓价格触发条件。
type Conditional struct {
	ID                                   string
	PositionSizeAtoms, TriggerPriceAtoms int64
	TriggerDirection, TriggerSource      string
}

// TriggeredOrder 是根据条件生成的安全只减仓操作。
type TriggeredOrder struct {
	Triggered     bool
	Side          string
	QuantityAtoms int64
	ReduceOnly    bool
}

// TriggerConditional 评估一个价格，绝不会生成扩大仓位的订单。
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
