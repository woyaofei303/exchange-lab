package postgres

import (
	"context"
	"encoding/json"
	"exchange-lab/internal/contractprice"
	"exchange-lab/internal/position"
	"exchange-lab/internal/readtools"
	"exchange-lab/internal/risk"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// ToolSource 在只读工具契约后组合现有仓储。
type ToolSource struct {
	positions *PositionRepository
	risk      *RiskRepository
	prices    *PriceRepository
}

// NewToolSource 创建共享的只读证据来源。
func NewToolSource(pool *pgxpool.Pool, timeout time.Duration) *ToolSource {
	return &ToolSource{NewPositionRepository(pool, timeout), NewRiskRepository(pool, timeout), NewPriceRepository(pool, timeout)}
}

// Positions 返回一个主体的持久化仓位。
func (s *ToolSource) Positions(ctx context.Context, user int64) ([]position.View, error) {
	return s.positions.List(ctx, user)
}

// Risk 返回一个主体和交易对的服务端计算风险快照。
func (s *ToolSource) Risk(ctx context.Context, user int64, symbol string) (risk.Snapshot, error) {
	return s.risk.Snapshot(ctx, user, symbol)
}

// Price 返回最新的已审计标记价格证据。
func (s *ToolSource) Price(ctx context.Context, symbol string) (contractprice.Snapshot, error) {
	return s.prices.Latest(ctx, symbol)
}

// Funding 返回一个交易对的有界资金费历史。
func (s *ToolSource) Funding(ctx context.Context, symbol string) ([]risk.FundingRecord, error) {
	return s.risk.FundingHistory(ctx, symbol)
}

// Liquidations 返回限定到用户范围的清算历史。
func (s *ToolSource) Liquidations(ctx context.Context, user int64) ([]risk.LiquidationRecord, error) {
	return s.risk.Liquidations(ctx, user)
}

// ToolAuditRepository 持久化每次工具调用的范围和证据。
type ToolAuditRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewToolAuditRepository 创建受上下文限制的工具审计接收端。
func NewToolAuditRepository(pool *pgxpool.Pool, timeout time.Duration) *ToolAuditRepository {
	return &ToolAuditRepository{pool, timeout}
}

// RecordTool 追加一条只读工具审计事实。
func (r *ToolAuditRepository) RecordTool(ctx context.Context, record readtools.AuditRecord) error {
	op, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	id, err := randomBusinessID("tool_")
	if err != nil {
		return err
	}
	scope, err := json.Marshal(record.Scope)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(op, `INSERT INTO tool_audit_logs(id,user_id,tool_name,input_scope,evidence_ids,success,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, record.UserID, record.Tool, scope, record.EvidenceIDs, record.Success, record.OccurredAt.UTC())
	if err != nil {
		return fmt.Errorf("record tool audit: %w", err)
	}
	return nil
}
