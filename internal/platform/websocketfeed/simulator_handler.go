package websocketfeed

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"exchange-lab/internal/marketdata"
	"github.com/coder/websocket"
)

// SimulatorFactory 为每个连接创建隔离的确定性状态。
type SimulatorFactory func() (*marketdata.Simulator, error)

// SimulatorHandler 按固定间隔流式发送规范化事件。
type SimulatorHandler struct {
	interval time.Duration
	factory  SimulatorFactory
}

// NewSimulatorHandler 校验事件发送节奏和逐连接工厂。
func NewSimulatorHandler(interval time.Duration, factory SimulatorFactory) (*SimulatorHandler, error) {
	if interval <= 0 || factory == nil {
		return nil, fmt.Errorf("create simulator handler: invalid configuration")
	}
	return &SimulatorHandler{interval: interval, factory: factory}, nil
}

// ServeHTTP 升级请求，并按配置间隔写入一个事件。
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
