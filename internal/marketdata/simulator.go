package marketdata

import (
	"fmt"
	"math"
	"time"

	"exchange-lab/internal/trading"
)

// SimulatorConfig 固定确定性数据流的标识、精度和时钟。
type SimulatorConfig struct {
	StreamID          string
	Symbol            string
	PriceScale        uint8
	QuantityScale     uint8
	InitialPriceAtoms int64
	TickAtoms         int64
	QuantityAtoms     int64
	Now               func() time.Time
}

// Simulator 按可重复的快照、行情、成交、增量循环生成事件。
type Simulator struct {
	config       SimulatorConfig
	sequence     uint64
	currentPrice int64
}

// NewSimulator 校验数值，确保后续生成过程不会溢出。
func NewSimulator(config SimulatorConfig) (*Simulator, error) {
	if !streamIDPattern.MatchString(config.StreamID) || !protocolSymbolPattern.MatchString(config.Symbol) ||
		config.PriceScale > 18 || config.QuantityScale > 18 || config.InitialPriceAtoms <= 0 ||
		config.TickAtoms <= 0 || config.QuantityAtoms <= 0 || config.QuantityAtoms > math.MaxInt64/100_000 || config.Now == nil ||
		config.InitialPriceAtoms > math.MaxInt64-config.TickAtoms || config.InitialPriceAtoms <= config.TickAtoms {
		return nil, fmt.Errorf("create market simulator: invalid configuration")
	}
	return &Simulator{config: config, currentPrice: config.InitialPriceAtoms}, nil
}

// Next 返回下一条带单调递增序号的确定性事件。
func (s *Simulator) Next() Event {
	s.sequence++
	event := Event{
		Version: ProtocolVersion, StreamID: s.config.StreamID, Symbol: s.config.Symbol,
		Sequence: s.sequence, EmittedAt: s.config.Now().UTC(),
	}
	switch (s.sequence - 1) % 4 {
	case 0:
		event.Type = EventBookSnapshot
		event.OrderBook = s.book()
	case 1:
		s.currentPrice += s.config.TickAtoms
		event.Type = EventTicker
		event.Ticker = &Ticker{
			LastPrice: s.price(s.currentPrice), MarkPrice: s.price(s.currentPrice),
			IndexPrice:  s.price(s.config.InitialPriceAtoms),
			PriceChange: s.price(s.currentPrice - s.config.InitialPriceAtoms), Volume: s.quantity(s.config.QuantityAtoms * int64((s.sequence%100_000)+1)),
		}
	case 2:
		event.Type = EventTrade
		event.Trade = &Trade{
			TradeID: fmt.Sprintf("sim-%d", s.sequence), Side: "buy",
			Price: s.price(s.currentPrice), Quantity: s.quantity(s.config.QuantityAtoms),
		}
	case 3:
		event.Type = EventBookDelta
		event.OrderBook = &OrderBook{Bids: []Level{{Price: s.price(s.currentPrice), Quantity: s.quantity(s.config.QuantityAtoms)}}}
	}
	return event
}

func (s *Simulator) book() *OrderBook {
	return &OrderBook{
		Bids: []Level{{Price: s.price(s.currentPrice - s.config.TickAtoms), Quantity: s.quantity(s.config.QuantityAtoms)}},
		Asks: []Level{{Price: s.price(s.currentPrice + s.config.TickAtoms), Quantity: s.quantity(s.config.QuantityAtoms)}},
	}
}

func (s *Simulator) price(atoms int64) string {
	value, err := trading.FormatDecimal(atoms, s.config.PriceScale)
	if err != nil {
		panic("validated simulator price became invalid")
	}
	return value
}

func (s *Simulator) quantity(atoms int64) string {
	value, err := trading.FormatDecimal(atoms, s.config.QuantityScale)
	if err != nil {
		panic("validated simulator quantity became invalid")
	}
	return value
}
