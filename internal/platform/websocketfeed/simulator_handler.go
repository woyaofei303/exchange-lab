package websocketfeed

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/woyaofei303/exchange-lab/internal/marketdata"
)

// SimulatorFactory creates isolated deterministic state for each connection.
type SimulatorFactory func() (*marketdata.Simulator, error)

// SimulatorHandler streams normalized events at a fixed interval.
type SimulatorHandler struct {
	interval time.Duration
	factory  SimulatorFactory
}

// NewSimulatorHandler validates the event cadence and per-connection factory.
func NewSimulatorHandler(interval time.Duration, factory SimulatorFactory) (*SimulatorHandler, error) {
	if interval <= 0 || factory == nil {
		return nil, fmt.Errorf("create simulator handler: invalid configuration")
	}
	return &SimulatorHandler{interval: interval, factory: factory}, nil
}

// ServeHTTP upgrades a request and writes one event per configured interval.
func (h *SimulatorHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	simulator, err := h.factory()
	if err != nil {
		http.Error(writer, "simulator unavailable", http.StatusServiceUnavailable)
		return
	}
	connection, err := websocket.Accept(writer, request, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer connection.CloseNow()
	connection.SetReadLimit(1024)
	lifecycle := connection.CloseRead(request.Context())

	if !h.write(lifecycle, connection, simulator.Next()) {
		return
	}
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-lifecycle.Done():
			return
		case <-ticker.C:
			if !h.write(lifecycle, connection, simulator.Next()) {
				return
			}
		}
	}
}

func (h *SimulatorHandler) write(ctx context.Context, connection *websocket.Conn, event marketdata.Event) bool {
	payload, err := marketdata.Encode(event)
	if err != nil {
		_ = connection.Close(websocket.StatusInternalError, "simulator event invalid")
		return false
	}
	writeContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := connection.Write(writeContext, websocket.MessageText, payload); err != nil {
		return false
	}
	return true
}
