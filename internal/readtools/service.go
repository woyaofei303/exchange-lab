// Package readtools is the only business surface exposed to AI/MCP. It is
// deliberately read-only and pins every user-scoped query to one principal.
package readtools

import (
	"context"
	"errors"
	"github.com/woyaofei303/exchange-lab/internal/contractprice"
	"github.com/woyaofei303/exchange-lab/internal/position"
	"github.com/woyaofei303/exchange-lab/internal/risk"
	"time"
)

// ErrInvalidScope reports an invalid principal, symbol, limit or dependency.
var ErrInvalidScope = errors.New("invalid read tool scope")

// Source is the read-only repository surface available to AI and MCP tools.
type Source interface {
	Positions(context.Context, int64) ([]position.View, error)
	Risk(context.Context, int64, string) (risk.Snapshot, error)
	Price(context.Context, string) (contractprice.Snapshot, error)
	Funding(context.Context, string) ([]risk.FundingRecord, error)
	Liquidations(context.Context, int64) ([]risk.LiquidationRecord, error)
}

// AuditRecord captures one tool request, its evidence and success outcome.
type AuditRecord struct {
	UserID      int64
	Tool        string
	Scope       map[string]any
	EvidenceIDs []string
	Success     bool
	OccurredAt  time.Time
}

// Audit persists tool-call evidence independently from response generation.
type Audit interface {
	RecordTool(context.Context, AuditRecord) error
}

// Service pins all user-scoped tool calls to one authenticated principal.
type Service struct {
	userID int64
	source Source
	audit  Audit
	now    func() time.Time
}

// New validates a principal and constructs its read-only tool service.
func New(userID int64, source Source, audit Audit, now func() time.Time) (*Service, error) {
	if userID <= 0 || source == nil || audit == nil || now == nil {
		return nil, ErrInvalidScope
	}
	return &Service{userID: userID, source: source, audit: audit, now: now}, nil
}

// GetPosition returns the principal's position for the only supported symbol.
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

// GetMarginStatus returns the server-computed risk snapshot and records evidence.
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

// GetMarkPrice returns the source-audited mark-price snapshot.
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

// GetFunding returns a bounded slice of recent funding facts.
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

// GetLiquidationHistory returns only the principal's bounded liquidation history.
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
