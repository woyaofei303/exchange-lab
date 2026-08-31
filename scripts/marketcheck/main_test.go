package main

import (
	"strings"
	"testing"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/marketdata"
)

func TestValidateMessageRequiresMatchingCanonicalSubject(t *testing.T) {
	t.Parallel()

	event := marketdata.Event{
		Version: 1, StreamID: "check-stream", Type: marketdata.EventTicker,
		Symbol: "BTCUSDT-PERP", Sequence: 2, EmittedAt: time.Now().UTC(),
		Ticker: &marketdata.Ticker{LastPrice: "50000.10", MarkPrice: "50000.10", IndexPrice: "50000.00", PriceChange: "0.10", Volume: "1.000"},
	}
	payload, err := marketdata.Encode(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateMessage("market.BTCUSDT-PERP.ticker", payload); err != nil {
		t.Fatalf("validateMessage() error = %v", err)
	}
	if err := validateMessage("market.BTCUSDT-PERP.trade", payload); err == nil {
		t.Fatal("validateMessage(mismatched subject) error = nil")
	}
	if err := validateMessage("market.BTCUSDT-PERP.ticker", []byte(`not-json`)); err == nil || strings.Contains(err.Error(), "not-json") {
		t.Fatalf("validateMessage(invalid JSON) error = %v", err)
	}
}
