package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/woyaofei303/exchange-lab/internal/contractprice"
	"github.com/woyaofei303/exchange-lab/internal/position"
	"github.com/woyaofei303/exchange-lab/internal/readtools"
	"github.com/woyaofei303/exchange-lab/internal/risk"
	"time"
)

// ToolSource composes existing repositories behind the read-only tool contract.
type ToolSource struct {
	positions *PositionRepository
	risk      *RiskRepository
	prices    *PriceRepository
}

// NewToolSource constructs a shared read-only evidence source.
func NewToolSource(pool *pgxpool.Pool, timeout time.Duration) *ToolSource {
	return &ToolSource{NewPositionRepository(pool, timeout), NewRiskRepository(pool, timeout), NewPriceRepository(pool, timeout)}
}

// Positions returns persisted positions for one principal.
func (s *ToolSource) Positions(ctx context.Context, user int64) ([]position.View, error) {
	return s.positions.List(ctx, user)
}

// Risk returns a server-computed risk snapshot for one principal and symbol.
func (s *ToolSource) Risk(ctx context.Context, user int64, symbol string) (risk.Snapshot, error) {
	return s.risk.Snapshot(ctx, user, symbol)
}

// Price returns the latest audited mark-price evidence.
func (s *ToolSource) Price(ctx context.Context, symbol string) (contractprice.Snapshot, error) {
	return s.prices.Latest(ctx, symbol)
}

// Funding returns bounded funding history for a symbol.
func (s *ToolSource) Funding(ctx context.Context, symbol string) ([]risk.FundingRecord, error) {
	return s.risk.FundingHistory(ctx, symbol)
}

// Liquidations returns user-scoped liquidation history.
func (s *ToolSource) Liquidations(ctx context.Context, user int64) ([]risk.LiquidationRecord, error) {
	return s.risk.Liquidations(ctx, user)
}

// ToolAuditRepository persists the scope and evidence of every tool call.
type ToolAuditRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewToolAuditRepository constructs a context-bounded tool audit sink.
func NewToolAuditRepository(pool *pgxpool.Pool, timeout time.Duration) *ToolAuditRepository {
	return &ToolAuditRepository{pool, timeout}
}

// RecordTool appends one read-only tool audit fact.
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
