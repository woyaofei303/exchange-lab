// Package risk 定义只读的风险、资金费和清算投影。
package risk

import "time"

// Snapshot 是服务端计算的带版本保证金和清算投影。
type Snapshot struct {
	UserID                 int64
	Symbol                 string
	Mode                   string
	Leverage               int64
	PositionSizeAtoms      int64
	EntryPriceAtoms        int64
	MarkPriceAtoms         int64
	PriceScale             uint8
	QuantityScale          uint8
	QuoteScale             uint8
	PositionMarginAtoms    int64
	OrderMarginAtoms       int64
	WalletAvailableAtoms   int64
	UnrealizedPnLAtoms     int64
	MarginBalanceAtoms     int64
	MaintenanceMarginAtoms int64
	MarginRatioBPS         int64
	LiquidationDistanceBPS int64
	RiskVersion            int64
	Fresh                  bool
	AsOf                   time.Time
}

// LiquidationRecord 是一个幂等清算任务的读取模型。
type LiquidationRecord struct {
	ID                   string    `json:"id"`
	Symbol               string    `json:"symbol"`
	Mode                 string    `json:"mode"`
	Status               string    `json:"status"`
	MarkPriceAtoms       int64     `json:"mark_price_atoms"`
	BankruptcyPriceAtoms int64     `json:"bankruptcy_price_atoms"`
	RiskVersion          int64     `json:"risk_version"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// FundingRecord 表示一条已计划或已完成的资金费率事实。
type FundingRecord struct {
	ID               int64     `json:"id"`
	Symbol           string    `json:"symbol"`
	FundingTime      time.Time `json:"funding_time"`
	PredictedRateBPS int64     `json:"predicted_rate_bps"`
	FinalRateBPS     int64     `json:"final_rate_bps"`
	MarkPriceAtoms   int64     `json:"mark_price_atoms"`
	Status           string    `json:"status"`
}

// FundingPayment 表示一个用户精确的有符号结算差额。
type FundingPayment struct {
	FundingRateID     int64     `json:"funding_rate_id"`
	Symbol            string    `json:"symbol"`
	PositionSizeAtoms int64     `json:"position_size_atoms"`
	PaymentAtoms      int64     `json:"payment_atoms"`
	SettledAt         time.Time `json:"settled_at"`
}

// InsuranceStatus 报告基金余额和未解决的 ADL 数量。
type InsuranceStatus struct {
	BalanceAtoms int64
	ADLPending   int64
	UpdatedAt    time.Time
}

// ConditionalOrder 是持久化的只减仓止盈或止损触发器。
type ConditionalOrder struct {
	ID                string    `json:"id"`
	UserID            int64     `json:"user_id"`
	Symbol            string    `json:"symbol"`
	Type              string    `json:"type"`
	TriggerSource     string    `json:"trigger_source"`
	TriggerDirection  string    `json:"trigger_direction"`
	TriggerPriceAtoms int64     `json:"trigger_price_atoms"`
	QuantityAtoms     int64     `json:"quantity_atoms"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
