package matching

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/eventing"
	"github.com/woyaofei303/exchange-lab/internal/trading"
)

// OrderEvent is the strict matching input decoded from a durable envelope.
type OrderEvent struct {
	OrderID        string
	UserID         int64
	Symbol         string
	Side           trading.Side
	Type           trading.OrderType
	Status         trading.OrderStatus
	Price          string
	Quantity       string
	Remaining      string
	Leverage       int64
	ReservedMargin string
	OccurredAt     time.Time
}

type orderEventPayload struct {
	Version        int                 `json:"version"`
	OrderID        string              `json:"order_id"`
	UserID         int64               `json:"user_id"`
	Symbol         string              `json:"symbol"`
	Side           trading.Side        `json:"side"`
	Type           trading.OrderType   `json:"type"`
	Status         trading.OrderStatus `json:"status"`
	Price          string              `json:"price"`
	Quantity       string              `json:"quantity"`
	Remaining      string              `json:"remaining"`
	Leverage       int64               `json:"leverage"`
	ReservedMargin string              `json:"reserved_margin"`
	OccurredAt     time.Time           `json:"occurred_at"`
}

// DecodeOrderEvent accepts only versioned limit-order acceptance or cancellation facts.
func DecodeOrderEvent(envelope eventing.Envelope) (OrderEvent, error) {
	if envelope.Type != "orders.accepted" && envelope.Type != "orders.cancel_requested" {
		return OrderEvent{}, ErrInvalidOrder
	}
	decoder := json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.DisallowUnknownFields()
	var payload orderEventPayload
	if err := decoder.Decode(&payload); err != nil {
		return OrderEvent{}, ErrInvalidOrder
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || payload.Version != 1 || payload.OrderID != envelope.AggregateID || payload.UserID <= 0 || payload.Symbol == "" || payload.Type != trading.OrderTypeLimit || payload.Leverage < 1 || payload.OccurredAt.IsZero() {
		return OrderEvent{}, ErrInvalidOrder
	}
	if payload.Side != trading.SideBuy && payload.Side != trading.SideSell {
		return OrderEvent{}, ErrInvalidOrder
	}
	return OrderEvent{OrderID: payload.OrderID, UserID: payload.UserID, Symbol: payload.Symbol, Side: payload.Side, Type: payload.Type, Status: payload.Status, Price: payload.Price, Quantity: payload.Quantity, Remaining: payload.Remaining, Leverage: payload.Leverage, ReservedMargin: payload.ReservedMargin, OccurredAt: payload.OccurredAt.UTC()}, nil
}
