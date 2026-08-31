package main

import (
	"testing"
	"time"

	"exchange-lab/internal/candle"
)

func TestValidateCandleMessageRequiresMatchingSubject(t *testing.T) {
	t.Parallel()

	view := candle.View{
		Symbol: "BTCUSDT-PERP", Interval: "1m",
		OpenTime: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC), CloseTime: time.Date(2026, 9, 21, 12, 1, 0, 0, time.UTC),
		Open: "50000.00", High: "50001.00", Low: "49999.00", Close: "50000.50", Volume: "1.000", TradeCount: 2,
	}
	payload, err := candle.EncodeUpdate(candle.Update{Version: 1, Type: "candle", Data: view})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validateCandleMessage("candles.BTCUSDT-PERP.1m", payload); err != nil {
		t.Fatalf("validateCandleMessage() error = %v", err)
	}
	if _, err := validateCandleMessage("candles.BTCUSDT-PERP.5m", payload); err == nil {
		t.Fatal("validateCandleMessage(mismatch) error = nil")
	}
}
