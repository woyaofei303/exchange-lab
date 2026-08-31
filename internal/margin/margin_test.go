package margin

import "testing"

func TestIsolatedRiskUsesConservativeIntegerRounding(t *testing.T) {
	tier := Tier{NotionalCapAtoms: 10_000_000_000, MaxLeverage: 50, MaintenanceMarginBPS: 50}
	s, err := EvaluateIsolated(Input{WalletBalanceAtoms: 2_000_000, PositionSizeAtoms: 1_000, EntryPriceAtoms: 5_000_000, MarkPriceAtoms: 4_900_000, QuantityScale: 3, Leverage: 10, OrderMarginAtoms: 100_000}, tier)
	if err != nil {
		t.Fatal(err)
	}
	if s.NotionalAtoms != 4_900_000 || s.UnrealizedPnLAtoms != -100_000 || s.InitialMarginAtoms != 490_000 || s.MaintenanceMarginAtoms != 24_500 || s.AvailableBalanceAtoms != 1_800_000 {
		t.Fatalf("snapshot=%#v", s)
	}
}

func TestRiskRejectsLeverageAboveTierAndInsufficientBalance(t *testing.T) {
	_, err := EvaluateIsolated(Input{WalletBalanceAtoms: 1, PositionSizeAtoms: 1_000, EntryPriceAtoms: 5_000_000, MarkPriceAtoms: 5_000_000, QuantityScale: 3, Leverage: 100}, Tier{NotionalCapAtoms: 10_000_000, MaxLeverage: 50, MaintenanceMarginBPS: 50})
	if err != ErrLeverage {
		t.Fatalf("error=%v", err)
	}
	if err := CheckOrder(100, 80, 30, 1); err != ErrInsufficientMargin {
		t.Fatalf("order error=%v", err)
	}
}

func TestCrossAggregatesPositionsAndUsesSnapshotVersion(t *testing.T) {
	s, err := EvaluateCross(2_000_000, 100_000, 7, []PositionRisk{{UnrealizedPnLAtoms: 200_000, MaintenanceMarginAtoms: 50_000}, {UnrealizedPnLAtoms: -100_000, MaintenanceMarginAtoms: 75_000}})
	if err != nil {
		t.Fatal(err)
	}
	if s.MarginBalanceAtoms != 2_100_000 || s.MaintenanceMarginAtoms != 125_000 || s.AvailableBalanceAtoms != 2_000_000 || s.Version != 7 {
		t.Fatalf("cross=%#v", s)
	}
	if !SameVersion(s, 7) || SameVersion(s, 8) {
		t.Fatal("version guard failed")
	}
}

func BenchmarkEvaluateIsolated(b *testing.B) {
	input := Input{WalletBalanceAtoms: 2_000_000, PositionSizeAtoms: 1_000, EntryPriceAtoms: 5_000_000, MarkPriceAtoms: 4_900_000, QuantityScale: 3, Leverage: 10, OrderMarginAtoms: 100_000}
	tier := Tier{NotionalCapAtoms: 10_000_000_000, MaxLeverage: 50, MaintenanceMarginBPS: 50}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		_, _ = EvaluateIsolated(input, tier)
	}
}
