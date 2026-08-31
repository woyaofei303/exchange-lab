package websocketfeed_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/marketdata"
	"github.com/woyaofei303/exchange-lab/internal/platform/websocketfeed"
)

func TestSimulatorHandlerStreamsProtocolEvents(t *testing.T) {
	t.Parallel()

	factory := func() (*marketdata.Simulator, error) {
		return marketdata.NewSimulator(marketdata.SimulatorConfig{
			StreamID: "handler-stream", Symbol: "BTCUSDT-PERP", PriceScale: 2, QuantityScale: 3,
			InitialPriceAtoms: 5_000_000, TickAtoms: 10, QuantityAtoms: 1, Now: time.Now,
		})
	}
	handler, err := websocketfeed.NewSimulatorHandler(time.Millisecond, factory)
	if err != nil {
		t.Fatalf("NewSimulatorHandler() error = %v", err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	connector, err := websocketfeed.NewConnector(strings.Replace(server.URL, "http://", "ws://", 1), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	connection, err := connector.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	for sequence := uint64(1); sequence <= 4; sequence++ {
		payload, err := connection.Read(ctx)
		if err != nil {
			t.Fatalf("Read(%d) error = %v", sequence, err)
		}
		event, err := marketdata.Decode(payload)
		if err != nil || event.Sequence != sequence {
			t.Fatalf("event %d = %#v, %v", sequence, event, err)
		}
	}
}
