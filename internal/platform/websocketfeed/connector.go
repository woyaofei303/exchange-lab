// Package websocketfeed adapts WebSocket transport to the market-data protocol.
package websocketfeed

import (
	"context"
	"fmt"
	"net/url"

	"github.com/coder/websocket"
	"github.com/woyaofei303/exchange-lab/internal/marketdata"
)

// Connector opens WebSocket feed sessions with a bounded frame size.
type Connector struct {
	url            string
	readLimitBytes int64
}

// NewConnector validates the feed URL and maximum message size.
func NewConnector(rawURL string, readLimitBytes int64) (*Connector, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") || parsed.Hostname() == "" || parsed.User != nil || readLimitBytes <= 0 {
		return nil, fmt.Errorf("create WebSocket connector: invalid configuration")
	}
	return &Connector{url: rawURL, readLimitBytes: readLimitBytes}, nil
}

// Connect opens one text-message feed session.
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

// Connection adapts coder/websocket to the marketdata single-reader contract.
type Connection struct {
	connection *websocket.Conn
}

// Read returns one bounded text frame and rejects binary messages.
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

// Ping checks peer responsiveness without starting a second read loop.
func (c *Connection) Ping(ctx context.Context) error {
	if err := c.connection.Ping(ctx); err != nil {
		return fmt.Errorf("ping market WebSocket")
	}
	return nil
}

// Close immediately ends the feed session.
func (c *Connection) Close() error {
	if err := c.connection.CloseNow(); err != nil {
		return fmt.Errorf("close market WebSocket")
	}
	return nil
}
