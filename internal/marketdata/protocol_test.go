package marketdata_test

import (
	"strings"
	"testing"
	"time"

	"exchange-lab/internal/marketdata"
)

func TestEncodeDecodeTickerRoundTrip(t *testing.T) {
	t.Parallel()

	want := marketdata.Event{
		Version: 1, StreamID: "stream-a", Type: marketdata.EventTicker,
		Symbol: "BTCUSDT-PERP", Sequence: 2, EmittedAt: time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
		Ticker: &marketdata.Ticker{
			LastPrice: "50000.10", MarkPrice: "50000.20", IndexPrice: "49999.90",
			PriceChange: "0.10", Volume: "12.345",
		},
	}

	payload, err := marketdata.Encode(want)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	got, err := marketdata.Decode(payload)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got.Type != want.Type || got.Symbol != want.Symbol || got.Sequence != want.Sequence || got.Ticker == nil || got.Ticker.LastPrice != want.Ticker.LastPrice {
		t.Fatalf("Decode() = %#v, want key fields from %#v", got, want)
	}
	if subject, err := marketdata.Subject(got); err != nil || subject != "market.BTCUSDT-PERP.ticker" {
		t.Fatalf("Subject() = %q, %v", subject, err)
	}
}

func TestDecodeRejectsMalformedAndAmbiguousEvents(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"invalid JSON":       `{`,
		"unknown field":      validTickerJSON(`,"secret":"value"`),
		"numeric price":      strings.Replace(validTickerJSON(""), `"50000.10"`, `50000.10`, 1),
		"wrong version":      strings.Replace(validTickerJSON(""), `"version":1`, `"version":2`, 1),
		"invalid symbol":     strings.Replace(validTickerJSON(""), `BTCUSDT-PERP`, `btc_usdt`, 1),
		"zero sequence":      strings.Replace(validTickerJSON(""), `"sequence":2`, `"sequence":0`, 1),
		"missing payload":    strings.Replace(validTickerJSON(""), `,"ticker":{"last_price":"50000.10","mark_price":"50000.20","index_price":"49999.90","price_change":"0.10","volume":"12.345"}`, ``, 1),
		"ambiguous payloads": validTickerJSON(`,"trade":{"trade_id":"t-1","side":"buy","price":"50000.10","quantity":"0.001"}`),
		"invalid decimal":    strings.Replace(validTickerJSON(""), `"12.345"`, `"NaN"`, 1),
	}
	for name, payload := range tests {
		payload := payload
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := marketdata.Decode([]byte(payload)); err == nil {
				t.Fatal("Decode() error = nil")
			}
		})
	}
}

func FuzzDecodeNeverPanics(f *testing.F) {
	f.Add([]byte(validTickerJSON("")))
	f.Add([]byte(`{"type":"ticker"}`))
	f.Add([]byte{0xff, 0x00})
	f.Fuzz(func(t *testing.T, payload []byte) {
		_, _ = marketdata.Decode(payload)
	})
}

func validTickerJSON(extra string) string {
	return `{"version":1,"stream_id":"stream-a","type":"ticker","symbol":"BTCUSDT-PERP","sequence":2,"emitted_at":"2026-09-14T00:00:00Z","ticker":{"last_price":"50000.10","mark_price":"50000.20","index_price":"49999.90","price_change":"0.10","volume":"12.345"}` + extra + `}`
}
