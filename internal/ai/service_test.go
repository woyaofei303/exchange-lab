package ai

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/woyaofei303/exchange-lab/internal/risk"
	"testing"
	"time"
)

type fakeSource struct{}

func (fakeSource) Snapshot(context.Context, int64, string) (risk.Snapshot, error) {
	return risk.Snapshot{UserID: 7, Symbol: "BTCUSDT-PERP", Mode: "isolated", Leverage: 10, MarkPriceAtoms: 5_000_000, PriceScale: 2, QuantityScale: 3, QuoteScale: 8, MarginRatioBPS: 500, LiquidationDistanceBPS: 1200, RiskVersion: 9, Fresh: true, AsOf: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, nil
}
func (fakeSource) FundingHistory(context.Context, string) ([]risk.FundingRecord, error) {
	return []risk.FundingRecord{{ID: 3, Symbol: "BTCUSDT-PERP", FinalRateBPS: 5, Status: "settled"}}, nil
}
func (fakeSource) Liquidations(context.Context, int64) ([]risk.LiquidationRecord, error) {
	return nil, nil
}

type fakeModel struct{ payload []byte }

func (f fakeModel) Generate(context.Context, string, map[string]any) ([]byte, Usage, error) {
	return f.payload, Usage{InputTokens: 10, OutputTokens: 20}, nil
}

type fakeAudit struct {
	called bool
	err    error
}

func (a *fakeAudit) Record(context.Context, RunRecord) error { a.called = true; return a.err }
func TestServiceAcceptsStructuredEvidenceBoundOutput(t *testing.T) {
	payload, _ := json.Marshal(Summary{Trend: "neutral", Position: "long", MarginRatioBPS: 500, LiquidationDistanceBPS: 1200, FundingRateBPS: 5, Risks: []string{"price volatility"}, EvidenceIDs: []string{"risk:7:BTCUSDT-PERP:9", "funding:3"}, Disclaimer: "simulation only"})
	audit := &fakeAudit{}
	service, err := NewService(fakeSource{}, fakeModel{payload}, audit, "test-model", "risk-v1", time.Now)
	if err != nil {
		t.Fatal(err)
	}
	summary, _, err := service.Summarize(context.Background(), 7, "BTCUSDT-PERP")
	if err != nil {
		t.Fatal(err)
	}
	if summary.MarginRatioBPS != 500 || !audit.called {
		t.Fatalf("summary=%#v audit=%v", summary, audit.called)
	}
}
func TestServiceRejectsInventedEvidence(t *testing.T) {
	payload, _ := json.Marshal(Summary{Trend: "neutral", Position: "flat", EvidenceIDs: []string{"invented"}, Disclaimer: "simulation only"})
	service, _ := NewService(fakeSource{}, fakeModel{payload}, &fakeAudit{}, "test", "v1", time.Now)
	if _, _, err := service.Summarize(context.Background(), 7, "BTCUSDT-PERP"); err == nil {
		t.Fatal("expected invalid evidence")
	}
}

func TestServiceReturnsAuditFailure(t *testing.T) {
	payload, _ := json.Marshal(Summary{Trend: "neutral", Position: "long", MarginRatioBPS: 500, LiquidationDistanceBPS: 1200, FundingRateBPS: 5, Risks: []string{"price volatility"}, EvidenceIDs: []string{"risk:7:BTCUSDT-PERP:9"}, Disclaimer: "simulation only"})
	auditErr := errors.New("audit unavailable")
	service, _ := NewService(fakeSource{}, fakeModel{payload}, &fakeAudit{err: auditErr}, "test", "v1", time.Now)
	if _, _, err := service.Summarize(context.Background(), 7, "BTCUSDT-PERP"); !errors.Is(err, auditErr) {
		t.Fatalf("expected audit failure, got %v", err)
	}
}
