package matching

import (
	"fmt"
	"reflect"
	"testing"

	"exchange-lab/internal/trading"
)

func TestEngineMatchesBestPriceThenFIFOAtMakerPrice(t *testing.T) {
	engine := NewEngine()
	for _, maker := range []LimitOrder{
		{ID: "ask-expensive", UserID: 1, Side: trading.SideSell, PriceAtoms: 5_010_000, QuantityAtoms: 5, Sequence: 1},
		{ID: "ask-first", UserID: 2, Side: trading.SideSell, PriceAtoms: 5_000_000, QuantityAtoms: 4, Sequence: 2},
		{ID: "ask-second", UserID: 3, Side: trading.SideSell, PriceAtoms: 5_000_000, QuantityAtoms: 4, Sequence: 3},
	} {
		if _, err := engine.Add(maker); err != nil {
			t.Fatal(err)
		}
	}
	result, err := engine.Add(LimitOrder{ID: "buy-taker", UserID: 4, Side: trading.SideBuy, PriceAtoms: 5_005_000, QuantityAtoms: 6, Sequence: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trades) != 2 {
		t.Fatalf("trades = %#v", result.Trades)
	}
	if got := result.Trades[0]; got.MakerOrderID != "ask-first" || got.PriceAtoms != 5_000_000 || got.QuantityAtoms != 4 {
		t.Fatalf("first trade = %#v", got)
	}
	if got := result.Trades[1]; got.MakerOrderID != "ask-second" || got.PriceAtoms != 5_000_000 || got.QuantityAtoms != 2 {
		t.Fatalf("second trade = %#v", got)
	}
	snapshot := engine.Snapshot()
	if len(snapshot.Asks) != 2 || snapshot.Asks[0].ID != "ask-second" || snapshot.Asks[0].RemainingAtoms != 2 || snapshot.Asks[1].ID != "ask-expensive" {
		t.Fatalf("asks = %#v", snapshot.Asks)
	}
}

func TestEnginePartialFillCancelAndConservation(t *testing.T) {
	engine := NewEngine()
	_, _ = engine.Add(LimitOrder{ID: "bid", UserID: 1, Side: trading.SideBuy, PriceAtoms: 5_000_000, QuantityAtoms: 3, Sequence: 1})
	result, err := engine.Add(LimitOrder{ID: "ask", UserID: 2, Side: trading.SideSell, PriceAtoms: 4_999_000, QuantityAtoms: 5, Sequence: 2})
	if err != nil || len(result.Trades) != 1 || result.Trades[0].QuantityAtoms != 3 {
		t.Fatalf("partial match = %#v, %v", result, err)
	}
	if result.IncomingStatus != trading.OrderStatusPartiallyFilled || result.IncomingRemainingAtoms != 2 {
		t.Fatalf("incoming state = %s/%d", result.IncomingStatus, result.IncomingRemainingAtoms)
	}
	canceled, err := engine.Cancel("ask")
	if err != nil || canceled.RemainingAtoms != 2 || canceled.Status != trading.OrderStatusCanceled {
		t.Fatalf("Cancel() = %#v, %v", canceled, err)
	}
	if _, err := engine.Cancel("ask"); err == nil {
		t.Fatal("duplicate cancel succeeded")
	}
}

func TestEngineReplayIsDeterministic(t *testing.T) {
	input := []LimitOrder{
		{ID: "a", UserID: 1, Side: trading.SideBuy, PriceAtoms: 100, QuantityAtoms: 10, Sequence: 1},
		{ID: "b", UserID: 2, Side: trading.SideSell, PriceAtoms: 90, QuantityAtoms: 4, Sequence: 2},
		{ID: "c", UserID: 3, Side: trading.SideSell, PriceAtoms: 100, QuantityAtoms: 8, Sequence: 3},
	}
	run := func() ([]Trade, Snapshot) {
		engine := NewEngine()
		var trades []Trade
		for _, item := range input {
			result, err := engine.Add(item)
			if err != nil {
				t.Fatal(err)
			}
			trades = append(trades, result.Trades...)
		}
		return trades, engine.Snapshot()
	}
	firstTrades, firstBook := run()
	secondTrades, secondBook := run()
	if !reflect.DeepEqual(firstTrades, secondTrades) || !reflect.DeepEqual(firstBook, secondBook) {
		t.Fatalf("replay mismatch:\n%#v %#v\n%#v %#v", firstTrades, firstBook, secondTrades, secondBook)
	}
}

func FuzzEngineNeverOverfills(f *testing.F) {
	f.Add(uint64(100), uint64(5), uint64(7))
	f.Fuzz(func(t *testing.T, rawPrice, rawBidQty, rawAskQty uint64) {
		price := int64(rawPrice%1_000_000 + 1)
		bidQty := int64(rawBidQty%100_000 + 1)
		askQty := int64(rawAskQty%100_000 + 1)
		engine := NewEngine()
		_, _ = engine.Add(LimitOrder{ID: "bid", UserID: 1, Side: trading.SideBuy, PriceAtoms: price, QuantityAtoms: bidQty, Sequence: 1})
		result, err := engine.Add(LimitOrder{ID: "ask", UserID: 2, Side: trading.SideSell, PriceAtoms: price, QuantityAtoms: askQty, Sequence: 2})
		if err != nil {
			t.Fatal(err)
		}
		var filled int64
		for _, trade := range result.Trades {
			if trade.QuantityAtoms <= 0 || trade.PriceAtoms != price {
				t.Fatalf("invalid trade = %#v", trade)
			}
			filled += trade.QuantityAtoms
		}
		if filled > bidQty || filled > askQty {
			t.Fatalf("overfill %d > bid %d or ask %d", filled, bidQty, askQty)
		}
	})
}

func BenchmarkEngineMatch(b *testing.B) {
	for iteration := 0; iteration < b.N; iteration++ {
		engine := NewEngine()
		for sequence := int64(1); sequence <= 100; sequence++ {
			_, _ = engine.Add(LimitOrder{ID: fmt.Sprintf("maker-%d", sequence), UserID: sequence, Side: trading.SideSell, PriceAtoms: 5_000_000 + sequence, QuantityAtoms: 10, Sequence: sequence})
		}
		_, _ = engine.Add(LimitOrder{ID: "taker", UserID: 999, Side: trading.SideBuy, PriceAtoms: 5_001_000, QuantityAtoms: 1_000, Sequence: 101})
	}
}
