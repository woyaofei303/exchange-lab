package readtools

import (
	"context"
	"errors"
	"exchange-lab/internal/contractprice"
	"exchange-lab/internal/position"
	"exchange-lab/internal/risk"
	"testing"
	"time"
)

type sourceFake struct{ seenUser int64 }

func (s *sourceFake) Positions(context.Context, int64) ([]position.View, error) {
	return []position.View{{UserID: 7, Symbol: "BTCUSDT-PERP"}}, nil
}
func (s *sourceFake) Risk(_ context.Context, user int64, _ string) (risk.Snapshot, error) {
	s.seenUser = user
	return risk.Snapshot{UserID: user, Symbol: "BTCUSDT-PERP", RiskVersion: 1}, nil
}
func (s *sourceFake) Price(context.Context, string) (contractprice.Snapshot, error) {
	return contractprice.Snapshot{Symbol: "BTCUSDT-PERP", Fresh: true, AsOf: time.Now()}, nil
}
func (s *sourceFake) Funding(context.Context, string) ([]risk.FundingRecord, error) {
	return make([]risk.FundingRecord, 60), nil
}
func (s *sourceFake) Liquidations(context.Context, int64) ([]risk.LiquidationRecord, error) {
	return nil, nil
}

type auditFake struct {
	calls int
	err   error
}

func (a *auditFake) RecordTool(context.Context, AuditRecord) error { a.calls++; return a.err }
func TestServicePinsUserAndBoundsHistory(t *testing.T) {
	source, audit := &sourceFake{}, &auditFake{}
	service, err := New(7, source, audit, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetMarginStatus(context.Background(), "BTCUSDT-PERP"); err != nil {
		t.Fatal(err)
	}
	if source.seenUser != 7 || audit.calls != 1 {
		t.Fatalf("user=%d audit=%d", source.seenUser, audit.calls)
	}
	items, err := service.GetFunding(context.Background(), "BTCUSDT-PERP", 20)
	if err != nil || len(items) != 20 {
		t.Fatalf("funding=%d,%v", len(items), err)
	}
	if _, err := service.GetFunding(context.Background(), "BTCUSDT-PERP", 1000); err == nil {
		t.Fatal("expected range error")
	}
}

func TestServiceReturnsAuditFailure(t *testing.T) {
	auditErr := errors.New("audit unavailable")
	service, err := New(7, &sourceFake{}, &auditFake{err: auditErr}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetMarginStatus(context.Background(), "BTCUSDT-PERP"); !errors.Is(err, auditErr) {
		t.Fatalf("expected audit failure, got %v", err)
	}
}
