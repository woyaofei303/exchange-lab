package httpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"exchange-lab/internal/candle"
	"exchange-lab/internal/httpserver"
	"exchange-lab/internal/market"
	"exchange-lab/internal/realtime"
	"github.com/coder/websocket"
)

func TestCandlesReturnsExactHistoricalViews(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	reader := candleReader{list: func(_ context.Context, query candle.Query) ([]candle.Candle, error) {
		if query.Symbol != "BTCUSDT-PERP" || query.Interval != candle.OneMinute || !query.Start.Equal(start) || query.Limit != 100 {
			t.Fatalf("candle query = %#v", query)
		}
		return []candle.Candle{httpCandle(start)}, nil
	}}
	hub, _ := realtime.NewHub(8)
	handler := httpserver.NewHandlerWithCandles(readiness{}, market.NewStaticReader(nil, nil), reader, hub, nil, time.Second)
	path := "/v1/candles?symbol=BTCUSDT-PERP&interval=1m&start=2026-09-21T12:00:00Z&end=2026-09-21T12:01:00Z&limit=100"
	response := request(t, handler, http.MethodGet, path)
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Data []candle.View `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Data) != 1 {
		t.Fatalf("body = %#v, %v", body, err)
	}
	if body.Data[0].Open != "50000.00" || body.Data[0].Close != "50000.50" || body.Data[0].Volume != "0.010" || body.Data[0].Interval != "1m" {
		t.Fatalf("candle view = %#v", body.Data[0])
	}
}

func TestCandlesRejectsInvalidOrUnboundedQueries(t *testing.T) {
	t.Parallel()

	reader := candleReader{list: func(context.Context, candle.Query) ([]candle.Candle, error) { return nil, nil }}
	hub, _ := realtime.NewHub(8)
	handler := httpserver.NewHandlerWithCandles(readiness{}, market.NewStaticReader(nil, nil), reader, hub, nil, time.Second)
	queries := []string{
		"?symbol=btcusdt-perp&interval=1m&start=2026-09-21T12:00:00Z&end=2026-09-21T12:01:00Z",
		"?symbol=BTCUSDT-PERP&interval=15m&start=2026-09-21T12:00:00Z&end=2026-09-21T12:01:00Z",
		"?symbol=BTCUSDT-PERP&interval=1m&start=bad&end=2026-09-21T12:01:00Z",
		"?symbol=BTCUSDT-PERP&interval=1m&start=2026-09-21T12:01:00Z&end=2026-09-21T12:00:00Z",
		"?symbol=BTCUSDT-PERP&interval=1m&start=2026-09-21T12:00:00Z&end=2026-09-22T12:00:00Z&limit=1001",
		"?symbol=BTCUSDT-PERP&interval=1m&start=2026-09-21T12:00:00Z&end=2026-09-21T12:01:00Z&token=secret",
	}
	for _, query := range queries {
		response := request(t, handler, http.MethodGet, "/v1/candles"+query)
		if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "secret") {
			t.Fatalf("GET %s = %d %s", query, response.Code, response.Body.String())
		}
	}
}

func TestCandleWebSocketStreamsMatchingUpdatesBeyondRequestTimeout(t *testing.T) {
	t.Parallel()

	hub, _ := realtime.NewHub(8)
	reader := candleReader{list: func(context.Context, candle.Query) ([]candle.Candle, error) { return nil, nil }}
	handler := httpserver.NewHandlerWithCandles(readiness{}, market.NewStaticReader(nil, nil), reader, hub, nil, 10*time.Millisecond)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	url := strings.Replace(server.URL, "http://", "ws://", 1) + "/ws/v1/candles?symbol=BTCUSDT-PERP&interval=1m"
	connection, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer connection.CloseNow()
	deadline := time.Now().Add(time.Second)
	for hub.SubscriberCount() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	view, err := httpCandle(time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)).View()
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(candle.Update{Version: 1, Type: "candle", Data: view})
	hub.Publish(payload)
	messageType, got, err := connection.Read(ctx)
	if err != nil || messageType != websocket.MessageText || string(got) != string(payload) {
		t.Fatalf("Read() = %d %s, %v", messageType, got, err)
	}
}

type candleReader struct {
	list func(context.Context, candle.Query) ([]candle.Candle, error)
}

func (r candleReader) List(ctx context.Context, query candle.Query) ([]candle.Candle, error) {
	return r.list(ctx, query)
}

func httpCandle(openTime time.Time) candle.Candle {
	return candle.Candle{
		Symbol: "BTCUSDT-PERP", Interval: candle.OneMinute, OpenTime: openTime,
		PriceScale: 2, QuantityScale: 3,
		OpenAtoms: 5_000_000, HighAtoms: 5_001_000, LowAtoms: 4_999_000, CloseAtoms: 5_000_050,
		VolumeAtoms: 10, TradeCount: 2, Final: true,
		FirstTradeTime: openTime.Add(time.Second), LastTradeTime: openTime.Add(50 * time.Second),
		FirstSequence: 1, LastSequence: 2,
	}
}
