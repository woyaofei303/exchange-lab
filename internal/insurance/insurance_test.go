package insurance

import "testing"

func TestCoverLossUsesInsuranceThenCreatesADLShortfall(t *testing.T) {
	r, err := CoverLoss(60_000, 100_000)
	if err != nil {
		t.Fatal(err)
	}
	if r.InsuranceDebitAtoms != 60_000 || r.ADLShortfallAtoms != 40_000 || r.RemainingFundAtoms != 0 {
		t.Fatalf("result=%#v", r)
	}
}

func TestRankADLIsStableAndExplainable(t *testing.T) {
	ranked, err := RankADL([]Candidate{{UserID: 2, ProfitAtoms: 100, Leverage: 10}, {UserID: 1, ProfitAtoms: 100, Leverage: 10}, {UserID: 3, ProfitAtoms: 50, Leverage: 20}})
	if err != nil {
		t.Fatal(err)
	}
	if ranked[0].UserID != 1 || ranked[1].UserID != 2 || ranked[2].UserID != 3 {
		t.Fatalf("ranked=%#v", ranked)
	}
}

func TestConditionalOrderAlwaysReducesExistingPosition(t *testing.T) {
	o, err := TriggerConditional(Conditional{ID: "c1", PositionSizeAtoms: 1_000, TriggerPriceAtoms: 4_900_000, TriggerDirection: "lte", TriggerSource: "mark"}, 4_800_000)
	if err != nil || !o.Triggered || o.Side != "sell" || o.QuantityAtoms != 1_000 || !o.ReduceOnly {
		t.Fatalf("order=%#v err=%v", o, err)
	}
}
