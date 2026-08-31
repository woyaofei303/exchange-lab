package postgres

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/eventing"
	"github.com/woyaofei303/exchange-lab/internal/trading"
)

func TestDecodeExecutedTradeAcceptsCanonicalSnakeCasePayload(t *testing.T) {
	occurredAt := time.Date(2026, 8, 10, 5, 45, 51, 213126853, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"version": 1, "trade_id": "trade_0002", "symbol": "BTCUSDT-PERP",
		"maker_order_id": "ord_maker", "taker_order_id": "ord_taker",
		"maker_user_id": int64(7), "taker_user_id": int64(8), "taker_side": "buy",
		"price": "60000.00", "quantity": "0.010", "occurred_at": occurredAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	trade, err := decodeExecutedTrade(eventing.Envelope{Type: "trades.executed", AggregateID: "trade_0002", Data: payload})
	if err != nil {
		t.Fatalf("decodeExecutedTrade() error = %v", err)
	}
	if trade.TradeID != "trade_0002" || trade.MakerUserID != 7 || trade.TakerUserID != 8 || trade.TakerSide != trading.SideBuy || !trade.OccurredAt.Equal(occurredAt) {
		t.Fatalf("decoded trade = %#v", trade)
	}
}
