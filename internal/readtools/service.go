// Package readtools 是唯一向 AI/MCP 暴露的业务接口。
// 它刻意保持只读，并将每个用户范围查询绑定到一个主体。
package readtools

import (
	"context"
	"errors"
	"exchange-lab/internal/contractprice"
	"exchange-lab/internal/position"
	"exchange-lab/internal/risk"
	"time"
)

// ErrInvalidScope 表示主体、交易对、限制或依赖项无效。
var ErrInvalidScope = errors.New("invalid read tool scope")

// Source 是 AI 和 MCP 工具可用的只读仓储接口。
type Source interface {
	Positions(context.Context, int64) ([]position.View, error)
	Risk(context.Context, int64, string) (risk.Snapshot, error)
	Price(context.Context, string) (contractprice.Snapshot, error)
	Funding(context.Context, string) ([]risk.FundingRecord, error)
	Liquidations(context.Context, int64) ([]risk.LiquidationRecord, error)
}

// AuditRecord 记录一次工具请求、其证据和成功结果。
type AuditRecord struct {
	UserID      int64
	Tool        string
	Scope       map[string]any
	EvidenceIDs []string
	Success     bool
	OccurredAt  time.Time
}

// Audit 独立于响应生成过程持久化工具调用证据。
type Audit interface {
	RecordTool(context.Context, AuditRecord) error
}

// Service 将所有用户范围工具调用绑定到一个已认证主体。
type Service struct {
	userID int64
	source Source
	audit  Audit
	now    func() time.Time
}

// New 校验主体并创建其只读工具服务。
func New(userID int64, source Source, audit Audit, now func() time.Time) (*Service, error) {
	if userID <= 0 || source == nil || audit == nil || now == nil {
		return nil, ErrInvalidScope
	}
	return &Service{userID: userID, source: source, audit: audit, now: now}, nil
}

// GetPosition 返回主体在唯一受支持交易对上的仓位。
func (s *Service) GetPosition(ctx context.Context, symbol string) (position.View, error) {
	if !validSymbol(symbol) {
		return position.View{}, ErrInvalidScope
	}
	items, err := s.source.Positions(ctx, s.userID)
	for _, item := range items {
		if item.Symbol == symbol {
			return item, errors.Join(err, s.record(ctx, "get_position", symbol, []string{"position:" + symbol}, err == nil))
		}
	}
	recordErr := s.record(ctx, "get_position", symbol, nil, false)
	if err != nil {
		return position.View{}, errors.Join(err, recordErr)
	}
	return position.View{}, errors.Join(ErrInvalidScope, recordErr)
}

// GetMarginStatus 返回服务端计算的风险快照并记录证据。
func (s *Service) GetMarginStatus(ctx context.Context, symbol string) (risk.Snapshot, error) {
	if !validSymbol(symbol) {
		return risk.Snapshot{}, ErrInvalidScope
	}
	value, err := s.source.Risk(ctx, s.userID, symbol)
	ids := []string{}
	if err == nil {
		ids = []string{"risk:" + symbol}
	}
	return value, errors.Join(err, s.record(ctx, "get_margin_status", symbol, ids, err == nil))
}

// GetMarkPrice 返回经过来源审计的标记价格快照。
func (s *Service) GetMarkPrice(ctx context.Context, symbol string) (contractprice.Snapshot, error) {
	if !validSymbol(symbol) {
		return contractprice.Snapshot{}, ErrInvalidScope
	}
	value, err := s.source.Price(ctx, symbol)
	ids := []string{}
	if err == nil {
		ids = []string{"mark:" + symbol}
	}
	return value, errors.Join(err, s.record(ctx, "get_mark_price", symbol, ids, err == nil))
}

// GetFunding 返回有界的近期资金费事实切片。
func (s *Service) GetFunding(ctx context.Context, symbol string, limit int) ([]risk.FundingRecord, error) {
	if !validSymbol(symbol) || limit < 1 || limit > 50 {
		return nil, ErrInvalidScope
	}
	items, err := s.source.Funding(ctx, symbol)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, errors.Join(err, s.record(ctx, "get_funding", symbol, []string{"funding:" + symbol}, err == nil))
}

// GetLiquidationHistory 仅返回该主体的有界清算历史。
func (s *Service) GetLiquidationHistory(ctx context.Context, limit int) ([]risk.LiquidationRecord, error) {
	if limit < 1 || limit > 50 {
		return nil, ErrInvalidScope
	}
	items, err := s.source.Liquidations(ctx, s.userID)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, errors.Join(err, s.record(ctx, "get_liquidation_history", "user", []string{"liquidations:user"}, err == nil))
}
func (s *Service) record(ctx context.Context, tool, scope string, evidence []string, success bool) error {
	return s.audit.RecordTool(ctx, AuditRecord{UserID: s.userID, Tool: tool, Scope: map[string]any{"scope": scope}, EvidenceIDs: evidence, Success: success, OccurredAt: s.now().UTC()})
}
func validSymbol(symbol string) bool { return symbol == "BTCUSDT-PERP" }
