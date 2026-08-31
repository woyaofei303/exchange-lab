package httpserver

import (
	"testing"
	"time"

	"exchange-lab/internal/risk"
)

func TestPublicRiskIncludesServerComputedLiquidationDistance(t *testing.T) {
	result := publicRisk(risk.Snapshot{
		Symbol:                 "BTCUSDT-PERP",
		Mode:                   "isolated",
		Leverage:               10,
		PriceScale:             2,
		QuantityScale:          3,
		QuoteScale:             8,
		LiquidationDistanceBPS: 875,
		AsOf:                   time.Unix(1, 0),
	})
	if got, ok := result["liquidation_distance_bps"].(int64); !ok || got != 875 {
		t.Fatalf("liquidation_distance_bps = %#v, want int64(875)", result["liquidation_distance_bps"])
	}
}
