// Package websocketfeed 将 WebSocket 传输适配到行情协议。
package websocketfeed

import (
	"context"
	"fmt"
	"net/url"

	"exchange-lab/internal/marketdata"
	"github.com/coder/websocket"
)

// Connector 打开帧大小受限的 WebSocket 行情会话。
type Connector struct {
	url            string
	readLimitBytes int64
}

// NewConnector 校验行情源 URL 和最大消息大小。
func NewConnector(rawURL string, readLimitBytes int64) (*Connector, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") || parsed.Hostname() == "" || parsed.User != nil || readLimitBytes <= 0 {
		return nil, fmt.Errorf("create WebSocket connector: invalid configuration")
	}
	return &Connector{url: rawURL, readLimitBytes: readLimitBytes}, nil
}

// Connect 打开一个文本消息行情会话。
func (c *Connector) Connect(ctx context.Context) (marketdata.Connection, error) {
	connection, response, err := websocket.Dial(ctx, c.url, &websocket.DialOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, fmt.Errorf("connect market WebSocket")
	}
	connection.SetReadLimit(c.readLimitBytes)
	return &Connection{connection: connection}, nil
}

// Connection 将 coder/websocket 适配到 marketdata 的单读取器契约。
type Connection struct {
	connection *websocket.Conn
}

// Read 返回一个大小受限的文本帧，并拒绝二进制消息。
func (c *Connection) Read(ctx context.Context) ([]byte, error) {
	messageType, payload, err := c.connection.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("read market WebSocket")
	}
	if messageType != websocket.MessageText {
		return nil, fmt.Errorf("read market WebSocket: binary messages are unsupported")
	}
	return payload, nil
}

// Ping 在不启动第二个读取循环的情况下检查对端响应能力。
func (c *Connection) Ping(ctx context.Context) error {
	if err := c.connection.Ping(ctx); err != nil {
		return fmt.Errorf("ping market WebSocket")
	}
	return nil
}

// Close 立即结束行情会话。
func (c *Connection) Close() error {
	if err := c.connection.CloseNow(); err != nil {
		return fmt.Errorf("close market WebSocket")
	}
	return nil
}
