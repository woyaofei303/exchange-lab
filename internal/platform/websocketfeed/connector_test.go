package websocketfeed_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/woyaofei303/exchange-lab/internal/marketdata"
	"github.com/woyaofei303/exchange-lab/internal/platform/websocketfeed"
)

func TestConnectorReadsTextEventAndPings(t *testing.T) {
	t.Parallel()

	event := marketdata.Event{
		Version: 1, StreamID: "server-a", Type: marketdata.EventBookSnapshot,
		Symbol: "BTCUSDT-PERP", Sequence: 1, EmittedAt: time.Now().UTC(),
		OrderBook: &marketdata.OrderBook{
			Bids: []marketdata.Level{{Price: "50000.00", Quantity: "1.000"}},
			Asks: []marketdata.Level{{Price: "50000.10", Quantity: "1.000"}},
		},
	}
	payload, err := marketdata.Encode(event)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.CloseNow()
		if err := connection.Write(request.Context(), websocket.MessageText, payload); err != nil {
			return
		}
		connection.CloseRead(request.Context())
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)

	connector, err := websocketfeed.NewConnector(strings.Replace(server.URL, "http://", "ws://", 1), 1<<20)
	if err != nil {
		t.Fatalf("NewConnector() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	connection, err := connector.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer connection.Close()
	got, err := connection.Read(ctx)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	decoded, err := marketdata.Decode(got)
	if err != nil || decoded.Sequence != 1 {
		t.Fatalf("Decode(Read()) = %#v, %v", decoded, err)
	}
	readDone := make(chan error, 1)
	go func() {
		_, err := connection.Read(ctx)
		readDone <- err
	}()
	if err := connection.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := <-readDone; err == nil {
		t.Fatal("Read() after close error = nil")
	}
}

func TestConnectorRejectsInvalidURLAndBinaryMessages(t *testing.T) {
	t.Parallel()

	if _, err := websocketfeed.NewConnector("http://example.test", 1024); err == nil {
		t.Fatal("NewConnector(http URL) error = nil")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.CloseNow()
		_ = connection.Write(request.Context(), websocket.MessageBinary, []byte("binary"))
	}))
	t.Cleanup(server.Close)
	connector, err := websocketfeed.NewConnector(strings.Replace(server.URL, "http://", "ws://", 1), 1024)
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
	if _, err := connection.Read(ctx); err == nil {
		t.Fatal("Read(binary) error = nil")
	}
}
