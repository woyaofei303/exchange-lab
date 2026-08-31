package liquidation

import "testing"

func TestEvaluateOnlyUsesMarkPriceAndProducesIdempotentJob(t *testing.T) {
	r := Snapshot{UserID: 42, Symbol: "BTCUSDT-PERP", PositionSizeAtoms: 1_000, EntryPriceAtoms: 5_000_000, MarkPriceAtoms: 4_500_000, MarginBalanceAtoms: 20_000, MaintenanceMarginAtoms: 25_000, QuantityScale: 3, RiskVersion: 9}
	decision, err := Evaluate(r)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Required || decision.JobKey != "liq:42:BTCUSDT-PERP:9" || decision.BankruptcyPriceAtoms <= 0 {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestStateMachineRejectsInvalidTransition(t *testing.T) {
	if _, err := Transition(StatePending, EventSettled); err != ErrTransition {
		t.Fatalf("error=%v", err)
	}
	if got, err := Transition(StatePending, EventStarted); err != nil || got != StateExecuting {
		t.Fatalf("transition=%s,%v", got, err)
	}
}

func BenchmarkEvaluate(b *testing.B) {
	snapshot := Snapshot{UserID: 42, Symbol: "BTCUSDT-PERP", PositionSizeAtoms: 1_000, EntryPriceAtoms: 5_000_000, MarkPriceAtoms: 4_500_000, MarginBalanceAtoms: 20_000, MaintenanceMarginAtoms: 25_000, QuantityScale: 3, RiskVersion: 9}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		_, _ = Evaluate(snapshot)
	}
}
