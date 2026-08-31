package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"exchange-lab/internal/risk"
)

type evalSample struct {
	Name                   string `json:"name"`
	PositionSizeAtoms      int64  `json:"position_size_atoms"`
	Fresh                  bool   `json:"fresh"`
	MarginRatioBPS         int64  `json:"margin_ratio_bps"`
	LiquidationDistanceBPS int64  `json:"liquidation_distance_bps"`
	FundingRateBPS         int64  `json:"funding_rate_bps"`
	ExpectedPosition       string `json:"expected_position"`
	ExpectedRisk           string `json:"expected_risk"`
}

func TestContractRiskEvaluationSet(t *testing.T) {
	file, err := os.Open("testdata/contract-risk-evals.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		count++
		var sample evalSample
		if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil {
			t.Fatalf("sample %d: %v", count, err)
		}
		input := modelInput{
			Snapshot: risk.Snapshot{
				PositionSizeAtoms: sample.PositionSizeAtoms, Fresh: sample.Fresh,
				MarginRatioBPS: sample.MarginRatioBPS, LiquidationDistanceBPS: sample.LiquidationDistanceBPS,
			},
			Funding:            []risk.FundingRecord{{FinalRateBPS: sample.FundingRateBPS}},
			AllowedEvidenceIDs: []string{"risk:evaluation"},
		}
		prompt, _ := json.Marshal(input)
		payload, _, err := (DeterministicModel{}).Generate(context.Background(), string(prompt), SummarySchema())
		if err != nil {
			t.Fatalf("%s: Generate() error = %v", sample.Name, err)
		}
		summary, err := decodeSummary(payload)
		if err != nil {
			t.Fatalf("%s: decodeSummary() error = %v", sample.Name, err)
		}
		if summary.Position != sample.ExpectedPosition || summary.MarginRatioBPS != sample.MarginRatioBPS || summary.LiquidationDistanceBPS != sample.LiquidationDistanceBPS || summary.FundingRateBPS != sample.FundingRateBPS || !slices.Contains(summary.Risks, sample.ExpectedRisk) || summary.Disclaimer == "" {
			t.Fatalf("%s: summary = %#v", sample.Name, summary)
		}
		if len(summary.EvidenceIDs) != 1 || summary.EvidenceIDs[0] != "risk:evaluation" {
			t.Fatalf("%s: evidence = %#v", sample.Name, summary.EvidenceIDs)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 30 {
		t.Fatalf("evaluation sample count = %d, want 30", count)
	}
}
