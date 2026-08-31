// Package ledger 强制执行精确、平衡的资金变动和余额状态。
package ledger

import (
	"errors"
	"fmt"
	"math"
	"regexp"
)

// 账本错误按不变量、余额和幂等性失败分类。
var (
	ErrUnbalanced   = errors.New("ledger transaction is unbalanced")
	ErrInvalid      = errors.New("invalid ledger value")
	ErrInsufficient = errors.New("insufficient balance")
	ErrConflict     = errors.New("ledger idempotency conflict")
)

var assetPattern = regexp.MustCompile(`^[A-Z0-9]{2,12}$`)

// Entry 表示以精确资产单位记录的一笔有符号账户变动。
type Entry struct {
	AccountID   int64
	Asset       string
	AmountAtoms int64
}

// Transaction 是不可变的单资产零和条目集合。
type Transaction struct {
	ID             string
	Kind           string
	IdempotencyKey string
	Entries        []Entry
}

// FaucetResult 由幂等的模拟资金入账操作返回。
// 重放调用会返回原交易标识和当前余额。
type FaucetResult struct {
	TransactionID string
	Balance       Balance
	Replayed      bool
}

// NewTransaction 复制并校验条目，包括 int64 安全的零和检查。
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

// Total 返回有符号条目总和；有效交易的总和为零。
func (t Transaction) Total() int64 {
	var total int64
	for _, entry := range t.Entries {
		total += entry.AmountAtoms
	}
	return total
}

// Balance 是由可用和锁定最小单位组成的非负物化余额。
type Balance struct {
	AvailableAtoms int64
	LockedAtoms    int64
}

// Total 返回可用与锁定最小单位之和。
func (b Balance) Total() int64 {
	return b.AvailableAtoms + b.LockedAtoms
}

// Valid 表示两个组成部分及其总和是否符合账本不变量。
func (b Balance) Valid() bool {
	return b.valid()
}

// Lock 将可用最小单位移入锁定部分。
func (b Balance) Lock(amount int64) (Balance, error) {
	if !b.valid() || amount <= 0 || amount > b.AvailableAtoms || b.LockedAtoms > math.MaxInt64-amount {
		return Balance{}, ErrInsufficient
	}
	return Balance{AvailableAtoms: b.AvailableAtoms - amount, LockedAtoms: b.LockedAtoms + amount}, nil
}

// Release 将锁定最小单位移回可用部分。
func (b Balance) Release(amount int64) (Balance, error) {
	if !b.valid() || amount <= 0 || amount > b.LockedAtoms || b.AvailableAtoms > math.MaxInt64-amount {
		return Balance{}, ErrInsufficient
	}
	return Balance{AvailableAtoms: b.AvailableAtoms + amount, LockedAtoms: b.LockedAtoms - amount}, nil
}

// SpendLocked 消耗已预留最小单位，不增加可用余额。
func (b Balance) SpendLocked(amount int64) (Balance, error) {
	if !b.valid() || amount <= 0 || amount > b.LockedAtoms {
		return Balance{}, ErrInsufficient
	}
	return Balance{AvailableAtoms: b.AvailableAtoms, LockedAtoms: b.LockedAtoms - amount}, nil
}

// Credit 增加可用余额，并拒绝溢出。
func (b Balance) Credit(amount int64) (Balance, error) {
	if !b.valid() || amount <= 0 || b.AvailableAtoms > math.MaxInt64-amount {
		return Balance{}, fmt.Errorf("credit balance: %w", ErrInvalid)
	}
	return Balance{AvailableAtoms: b.AvailableAtoms + amount, LockedAtoms: b.LockedAtoms}, nil
}

func (b Balance) valid() bool {
	return b.AvailableAtoms >= 0 && b.LockedAtoms >= 0 && b.AvailableAtoms <= math.MaxInt64-b.LockedAtoms
}
