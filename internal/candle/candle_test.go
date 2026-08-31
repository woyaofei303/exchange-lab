package candle_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/candle"
)

func TestApplyTradeUsesEventTimeForOHLCAndSupportsLateCorrection(t *testing.T) {
	t.Parallel()

	minute := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	trades := []candle.Trade{
		trade("middle", minute.Add(30*time.Second), 2, 5_001_000, 2),
		trade("last", minute.Add(50*time.Second), 3, 4_999_000, 3),
		trade("late-first", minute.Add(10*time.Second), 1, 5_000_000, 4),
	}
	var current *candle.Candle
	for _, item := range trades {
		updated, err := candle.ApplyTrade(current, item, candle.OneMinute)
		if err != nil {
			t.Fatalf("ApplyTrade() error = %v", err)
		}
		current = &updated
	}
	if current.OpenAtoms != 5_000_000 || current.HighAtoms != 5_001_000 || current.LowAtoms != 4_999_000 || current.CloseAtoms != 4_999_000 {
		t.Fatalf("OHLC = %d/%d/%d/%d", current.OpenAtoms, current.HighAtoms, current.LowAtoms, current.CloseAtoms)
	}
	if current.VolumeAtoms != 9 || current.TradeCount != 3 || !current.OpenTime.Equal(minute) {
		t.Fatalf("aggregate = %#v", current)
	}
}

func TestApplyTradeRejectsWrongBucketScaleAndOverflow(t *testing.T) {
	t.Parallel()

	minute := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	base, err := candle.ApplyTrade(nil, trade("one", minute, 1, 5_000_000, 1), candle.OneMinute)
	if err != nil {
		t.Fatal(err)
	}
	tests := []candle.Trade{
		trade("next-minute", minute.Add(time.Minute), 2, 5_000_000, 1),
		{StreamID: "stream-a", TradeID: "scale", Symbol: "BTCUSDT-PERP", Sequence: 3, EventTime: minute, PriceAtoms: 5_000_000, PriceScale: 3, QuantityAtoms: 1, QuantityScale: 3},
		trade("overflow", minute, 4, 5_000_000, int64(^uint64(0)>>1)),
	}
	base.VolumeAtoms = 1
	for _, item := range tests {
		if _, err := candle.ApplyTrade(&base, item, candle.OneMinute); err == nil {
			t.Fatalf("ApplyTrade(%s) error = nil", item.TradeID)
		}
	}
}

func TestPropertyOHLCAndVolumeInvariants(t *testing.T) {
	t.Parallel()

	random := rand.New(rand.NewSource(7))
	minute := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	var current *candle.Candle
	for index := 0; index < 2_000; index++ {
		price := int64(4_900_000 + random.Intn(200_001))
		quantity := int64(1 + random.Intn(100))
		item := trade("property-"+time.Duration(index).String(), minute.Add(time.Duration(random.Intn(60))*time.Second), uint64(index+1), price, quantity)
		updated, err := candle.ApplyTrade(current, item, candle.OneMinute)
		if err != nil {
			t.Fatalf("ApplyTrade(%d) error = %v", index, err)
		}
		current = &updated
		if current.HighAtoms < current.OpenAtoms || current.HighAtoms < current.CloseAtoms || current.LowAtoms > current.OpenAtoms || current.LowAtoms > current.CloseAtoms || current.VolumeAtoms < 0 {
			t.Fatalf("invariant broken after %d: %#v", index, current)
		}
	}
}

func TestParseIntervalAndExactStringView(t *testing.T) {
	t.Parallel()

	for text, want := range map[string]candle.Interval{"1m": candle.OneMinute, "5m": candle.FiveMinutes, "1h": candle.OneHour} {
		got, err := candle.ParseInterval(text)
		if err != nil || got != want || got.String() != text {
			t.Fatalf("ParseInterval(%q) = %v, %v", text, got, err)
		}
	}
	if _, err := candle.ParseInterval("15m"); err == nil {
		t.Fatal("ParseInterval(15m) error = nil")
	}
	item, err := candle.ApplyTrade(nil, trade("view", time.Date(2026, 9, 21, 12, 0, 1, 0, time.UTC), 1, 5_000_010, 2), candle.OneMinute)
	if err != nil {
		t.Fatal(err)
	}
	view, err := item.View()
	if err != nil || view.Open != "50000.10" || view.Volume != "0.002" || view.Interval != "1m" {
		t.Fatalf("View() = %#v, %v", view, err)
	}
}

func trade(id string, eventTime time.Time, sequence uint64, price, quantity int64) candle.Trade {
	return candle.Trade{
		StreamID: "stream-a", TradeID: id, Symbol: "BTCUSDT-PERP", Sequence: sequence, EventTime: eventTime,
		PriceAtoms: price, PriceScale: 2, QuantityAtoms: quantity, QuantityScale: 3,
	}
}
