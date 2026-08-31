package candle_test

import (
	"testing"
	"time"

	"exchange-lab/internal/candle"
)

func TestRollupUsesFirstOpenLastCloseAndSumsExactVolume(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	minutes := []candle.Candle{
		minuteCandle(start, 5_000_000, 5_000_100, 4_999_900, 5_000_050, 10, true),
		minuteCandle(start.Add(time.Minute), 5_000_050, 5_000_300, 5_000_000, 5_000_200, 20, true),
		minuteCandle(start.Add(2*time.Minute), 5_000_200, 5_000_250, 4_999_800, 4_999_900, 30, true),
		minuteCandle(start.Add(3*time.Minute), 4_999_900, 5_000_000, 4_999_700, 4_999_800, 40, true),
		minuteCandle(start.Add(4*time.Minute), 4_999_800, 5_000_400, 4_999_750, 5_000_300, 50, true),
	}
	got, err := candle.Rollup(minutes, candle.FiveMinutes)
	if err != nil {
		t.Fatalf("Rollup() error = %v", err)
	}
	if got.OpenAtoms != 5_000_000 || got.HighAtoms != 5_000_400 || got.LowAtoms != 4_999_700 || got.CloseAtoms != 5_000_300 || got.VolumeAtoms != 150 || !got.Final {
		t.Fatalf("Rollup() = %#v", got)
	}
}

func TestRepairGapsCreatesFlatZeroVolumeCandlesWithoutChangingExisting(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	existing := []candle.Candle{
		minuteCandle(start, 5_000_000, 5_000_100, 4_999_900, 5_000_050, 10, true),
		minuteCandle(start.Add(2*time.Minute), 5_000_060, 5_000_100, 5_000_000, 5_000_080, 20, false),
	}
	got, err := candle.RepairGaps(existing, start, start.Add(3*time.Minute), candle.OneMinute)
	if err != nil {
		t.Fatalf("RepairGaps() error = %v", err)
	}
	if len(got) != 3 || got[1].OpenAtoms != 5_000_050 || got[1].CloseAtoms != 5_000_050 || got[1].VolumeAtoms != 0 || got[1].TradeCount != 0 || !got[1].Final {
		t.Fatalf("RepairGaps() = %#v", got)
	}
	if got[2].CloseAtoms != existing[1].CloseAtoms {
		t.Fatal("RepairGaps() changed existing candle")
	}
}

func TestRollupRejectsCrossBucketScaleMismatchAndIncompleteFinal(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	crossBucket := []candle.Candle{
		minuteCandle(start, 1, 1, 1, 1, 1, true),
		minuteCandle(start.Add(5*time.Minute), 1, 1, 1, 1, 1, true),
	}
	if _, err := candle.Rollup(crossBucket, candle.FiveMinutes); err == nil {
		t.Fatal("Rollup(cross bucket) error = nil")
	}
	scaleMismatch := []candle.Candle{
		minuteCandle(start, 1, 1, 1, 1, 1, true),
		minuteCandle(start.Add(time.Minute), 1, 1, 1, 1, 1, true),
	}
	scaleMismatch[1].PriceScale = 3
	if _, err := candle.Rollup(scaleMismatch, candle.FiveMinutes); err == nil {
		t.Fatal("Rollup(scale mismatch) error = nil")
	}
	partial, err := candle.Rollup([]candle.Candle{minuteCandle(start, 1, 1, 1, 1, 1, true)}, candle.FiveMinutes)
	if err != nil || partial.Final {
		t.Fatalf("partial Rollup() = %#v, %v; want non-final", partial, err)
	}
}

func minuteCandle(openTime time.Time, open, high, low, close, volume int64, final bool) candle.Candle {
	return candle.Candle{
		Symbol: "BTCUSDT-PERP", Interval: candle.OneMinute, OpenTime: openTime,
		PriceScale: 2, QuantityScale: 3, OpenAtoms: open, HighAtoms: high, LowAtoms: low, CloseAtoms: close,
		VolumeAtoms: volume, TradeCount: 1, Final: final,
		FirstTradeTime: openTime.Add(time.Second), LastTradeTime: openTime.Add(59 * time.Second),
		FirstSequence: 1, LastSequence: 1,
	}
}
