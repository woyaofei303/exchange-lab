package position

import "testing"

func TestApplyFillCoversIncreaseReduceCloseAndReverse(t *testing.T) {
	p := State{Symbol: "BTCUSDT-PERP", QuantityScale: 3, PriceScale: 2}
	var realized int64
	p, realized, _ = ApplyFill(p, Fill{Side: Buy, PriceAtoms: 5_000_000, QuantityAtoms: 1_000})
	if p.SizeAtoms != 1_000 || p.EntryPriceAtoms != 5_000_000 || realized != 0 {
		t.Fatalf("open = %#v pnl=%d", p, realized)
	}
	p, realized, _ = ApplyFill(p, Fill{Side: Buy, PriceAtoms: 5_100_000, QuantityAtoms: 1_000})
	if p.SizeAtoms != 2_000 || p.EntryPriceAtoms != 5_050_000 || realized != 0 {
		t.Fatalf("increase = %#v pnl=%d", p, realized)
	}
	p, realized, _ = ApplyFill(p, Fill{Side: Sell, PriceAtoms: 5_200_000, QuantityAtoms: 500, ReduceOnly: true})
	if p.SizeAtoms != 1_500 || realized != 75_000 {
		t.Fatalf("reduce = %#v pnl=%d", p, realized)
	}
	p, realized, _ = ApplyFill(p, Fill{Side: Sell, PriceAtoms: 4_900_000, QuantityAtoms: 2_000})
	if p.SizeAtoms != -500 || p.EntryPriceAtoms != 4_900_000 || realized != -225_000 {
		t.Fatalf("reverse = %#v pnl=%d", p, realized)
	}
}

func TestReduceOnlyCannotExpandOrReverse(t *testing.T) {
	_, _, err := ApplyFill(State{SizeAtoms: 100, EntryPriceAtoms: 100, QuantityScale: 0, PriceScale: 0}, Fill{Side: Sell, PriceAtoms: 100, QuantityAtoms: 101, ReduceOnly: true})
	if err != ErrReduceOnly {
		t.Fatalf("error=%v", err)
	}
}

func TestUnrealizedPnLUsesSignedPosition(t *testing.T) {
	long := State{SizeAtoms: 2_000, EntryPriceAtoms: 5_000_000, QuantityScale: 3, PriceScale: 2}
	if got, err := UnrealizedPnL(long, 5_100_000); err != nil || got != 200_000 {
		t.Fatalf("long=%d,%v", got, err)
	}
	short := State{SizeAtoms: -2_000, EntryPriceAtoms: 5_000_000, QuantityScale: 3, PriceScale: 2}
	if got, err := UnrealizedPnL(short, 4_900_000); err != nil || got != 200_000 {
		t.Fatalf("short=%d,%v", got, err)
	}
}

func FuzzApplyFillReplay(f *testing.F) {
	f.Add(int64(5_000_000), int64(1_000), int64(5_100_000), int64(500))
	f.Fuzz(func(t *testing.T, p1, q1, p2, q2 int64) {
		if p1 <= 0 || p2 <= 0 || q1 <= 0 || q2 <= 0 || p1 > 100_000_000 || p2 > 100_000_000 || q1 > 1_000_000 || q2 > 1_000_000 {
			return
		}
		start := State{QuantityScale: 3, PriceScale: 2}
		a, _, e1 := ApplyFill(start, Fill{Side: Buy, PriceAtoms: p1, QuantityAtoms: q1})
		if e1 != nil {
			return
		}
		a, ra, e2 := ApplyFill(a, Fill{Side: Sell, PriceAtoms: p2, QuantityAtoms: q2})
		if e2 != nil {
			return
		}
		b, _, _ := ApplyFill(start, Fill{Side: Buy, PriceAtoms: p1, QuantityAtoms: q1})
		b, rb, _ := ApplyFill(b, Fill{Side: Sell, PriceAtoms: p2, QuantityAtoms: q2})
		if a != b || ra != rb {
			t.Fatal("non deterministic replay")
		}
	})
}

func BenchmarkApplyFill(b *testing.B) {
	state := State{SizeAtoms: 1_000_000, EntryPriceAtoms: 5_000_000, QuantityScale: 3, PriceScale: 2}
	fill := Fill{Side: Sell, PriceAtoms: 5_100_000, QuantityAtoms: 1}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		_, _, _ = ApplyFill(state, fill)
	}
}
