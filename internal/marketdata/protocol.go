// Package marketdata 校验规范化行情源协议并维护其顺序。
package marketdata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"exchange-lab/internal/trading"
)

// ProtocolVersion 标识唯一接受的行情信封版本。
const (
	ProtocolVersion = 1
	maxPayloadBytes = 1 << 20
	maxBookLevels   = 5_000
)

// EventType 标识 Event 携带的唯一载荷类型。
type EventType string

// 支持的规范化行情载荷类型。
const (
	EventTicker       EventType = "ticker"
	EventTrade        EventType = "trade"
	EventBookSnapshot EventType = "orderbook_snapshot"
	EventBookDelta    EventType = "orderbook_delta"
)

// Event 是严格版本化且有序的规范化行情源消息。
type Event struct {
	Version   int        `json:"version"`
	StreamID  string     `json:"stream_id"`
	Type      EventType  `json:"type"`
	Symbol    string     `json:"symbol"`
	Sequence  uint64     `json:"sequence"`
	EmittedAt time.Time  `json:"emitted_at"`
	Ticker    *Ticker    `json:"ticker,omitempty"`
	Trade     *Trade     `json:"trade,omitempty"`
	OrderBook *OrderBook `json:"orderbook,omitempty"`
}

// Ticker 包含以精确十进制字符串表示的行情摘要值。
type Ticker struct {
	LastPrice   string `json:"last_price"`
	MarkPrice   string `json:"mark_price"`
	IndexPrice  string `json:"index_price"`
	PriceChange string `json:"price_change"`
	Volume      string `json:"volume"`
}

// Trade 包含一笔规范化的公开市场成交。
type Trade struct {
	TradeID  string `json:"trade_id"`
	Side     string `json:"side"`
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
}

// Level 表示一组精确十进制字符串价格和数量。
type Level struct {
	Price    string `json:"price"`
	Quantity string `json:"quantity"`
}

// OrderBook 携带买卖双方有界的快照或增量数据。
type OrderBook struct {
	Bids []Level `json:"bids"`
	Asks []Level `json:"asks"`
}

var (
	protocolSymbolPattern = regexp.MustCompile(`^[A-Z0-9]{4,24}-PERP$`)
	streamIDPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	tradeIDPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

// Encode 校验并序列化一个规范化行情事件。
func Encode(event Event) ([]byte, error) {
	if err := event.validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("encode market event: %w", err)
	}
	return payload, nil
}

// Decode 严格解析一个大小受限的事件，并拒绝未知字段。
func Decode(payload []byte) (Event, error) {
	if len(payload) == 0 || len(payload) > maxPayloadBytes {
		return Event{}, fmt.Errorf("decode market event: invalid payload size")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var event Event
	if err := decoder.Decode(&event); err != nil {
		return Event{}, fmt.Errorf("decode market event: invalid JSON: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Event{}, err
	}
	if err := event.validate(); err != nil {
		return Event{}, err
	}
	return event, nil
}

// Subject 将事件映射到稳定的 market.<symbol>.<kind> 主题。
func Subject(event Event) (string, error) {
	if !protocolSymbolPattern.MatchString(event.Symbol) {
		return "", fmt.Errorf("market subject: invalid symbol")
	}
	suffix := ""
	switch event.Type {
	case EventTicker:
		suffix = "ticker"
	case EventTrade:
		suffix = "trade"
	case EventBookSnapshot, EventBookDelta:
		suffix = "orderbook"
	default:
		return "", fmt.Errorf("market subject: invalid event type")
	}
	return "market." + event.Symbol + "." + suffix, nil
}

func (e Event) validate() error {
	if e.Version != ProtocolVersion {
		return fmt.Errorf("validate market event: unsupported version")
	}
	if !streamIDPattern.MatchString(e.StreamID) || !protocolSymbolPattern.MatchString(e.Symbol) || e.Sequence == 0 || e.EmittedAt.IsZero() {
		return fmt.Errorf("validate market event: invalid envelope")
	}
	payloadCount := 0
	if e.Ticker != nil {
		payloadCount++
	}
	if e.Trade != nil {
		payloadCount++
	}
	if e.OrderBook != nil {
		payloadCount++
	}
	if payloadCount != 1 {
		return fmt.Errorf("validate market event: exactly one payload is required")
	}
	switch e.Type {
	case EventTicker:
		if e.Ticker == nil || e.Trade != nil || e.OrderBook != nil {
			return fmt.Errorf("validate market event: ticker payload mismatch")
		}
		if err := e.Ticker.validate(); err != nil {
			return err
		}
	case EventTrade:
		if e.Trade == nil || e.Ticker != nil || e.OrderBook != nil {
			return fmt.Errorf("validate market event: trade payload mismatch")
		}
		if err := e.Trade.validate(); err != nil {
			return err
		}
	case EventBookSnapshot, EventBookDelta:
		if e.OrderBook == nil || e.Ticker != nil || e.Trade != nil {
			return fmt.Errorf("validate market event: orderbook payload mismatch")
		}
		if err := e.OrderBook.validate(e.Type == EventBookSnapshot); err != nil {
			return err
		}
	default:
		return fmt.Errorf("validate market event: unsupported event type")
	}
	return nil
}

func (t Ticker) validate() error {
	for name, value := range map[string]string{
		"last_price": t.LastPrice, "mark_price": t.MarkPrice, "index_price": t.IndexPrice,
	} {
		if err := validatePositiveDecimal(value); err != nil {
			return fmt.Errorf("validate ticker %s: %w", name, err)
		}
	}
	if err := validateSignedDecimal(t.PriceChange); err != nil {
		return fmt.Errorf("validate ticker price_change: %w", err)
	}
	if err := validateNonNegativeDecimal(t.Volume); err != nil {
		return fmt.Errorf("validate ticker volume: %w", err)
	}
	return nil
}

func (t Trade) validate() error {
	if !tradeIDPattern.MatchString(t.TradeID) || (t.Side != "buy" && t.Side != "sell") {
		return fmt.Errorf("validate trade: invalid identity or side")
	}
	if err := validatePositiveDecimal(t.Price); err != nil {
		return fmt.Errorf("validate trade price: %w", err)
	}
	if err := validatePositiveDecimal(t.Quantity); err != nil {
		return fmt.Errorf("validate trade quantity: %w", err)
	}
	return nil
}

func (b OrderBook) validate(snapshot bool) error {
	if len(b.Bids)+len(b.Asks) == 0 || len(b.Bids) > maxBookLevels || len(b.Asks) > maxBookLevels {
		return fmt.Errorf("validate orderbook: invalid level count")
	}
	if snapshot && (len(b.Bids) == 0 || len(b.Asks) == 0) {
		return fmt.Errorf("validate orderbook snapshot: both sides are required")
	}
	for _, level := range append(append([]Level(nil), b.Bids...), b.Asks...) {
		if err := validatePositiveDecimal(level.Price); err != nil {
			return fmt.Errorf("validate orderbook price: %w", err)
		}
		if err := validateNonNegativeDecimal(level.Quantity); err != nil {
			return fmt.Errorf("validate orderbook quantity: %w", err)
		}
	}
	return nil
}

func validatePositiveDecimal(value string) error {
	scale, err := decimalScale(value, false)
	if err != nil {
		return err
	}
	if _, err := trading.ParsePrice(value, scale); err != nil {
		return err
	}
	return nil
}

func validateNonNegativeDecimal(value string) error {
	scale, err := decimalScale(value, false)
	if err != nil {
		return err
	}
	asset, err := trading.NewAsset("UNIT", scale)
	if err != nil {
		return err
	}
	money, err := trading.ParseMoney(asset, value)
	if err != nil || money.Atoms() < 0 {
		return fmt.Errorf("decimal must be non-negative")
	}
	return nil
}

func validateSignedDecimal(value string) error {
	scale, err := decimalScale(value, true)
	if err != nil {
		return err
	}
	asset, err := trading.NewAsset("UNIT", scale)
	if err != nil {
		return err
	}
	if _, err := trading.ParseMoney(asset, value); err != nil {
		return err
	}
	return nil
}

func decimalScale(value string, signed bool) (uint8, error) {
	if value == "" || strings.TrimSpace(value) != value || len(value) > 40 {
		return 0, fmt.Errorf("invalid decimal")
	}
	unsigned := value
	if signed && strings.HasPrefix(unsigned, "-") {
		unsigned = strings.TrimPrefix(unsigned, "-")
	}
	parts := strings.Split(unsigned, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts) == 2 && parts[1] == "") {
		return 0, fmt.Errorf("invalid decimal")
	}
	for _, part := range parts {
		for _, character := range part {
			if character < '0' || character > '9' {
				return 0, fmt.Errorf("invalid decimal")
			}
		}
	}
	scale := 0
	if len(parts) == 2 {
		scale = len(parts[1])
	}
	if scale > 18 {
		return 0, fmt.Errorf("decimal precision exceeds 18")
	}
	return uint8(scale), nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("decode market event: trailing JSON")
	}
	return nil
}
