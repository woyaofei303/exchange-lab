// Package matching implements a deterministic price-time-priority order book.
package matching

import (
	"errors"
	"fmt"
	"sort"

	"github.com/woyaofei303/exchange-lab/internal/trading"
)

// Matching errors classify invalid input, duplicate order IDs and missing orders.
var (
	ErrInvalidOrder = errors.New("invalid matching order")
	ErrDuplicate    = errors.New("duplicate matching order")
	ErrNotFound     = errors.New("matching order not found")
)

// LimitOrder is a validated engine input ordered by its durable stream sequence.
type LimitOrder struct {
	ID            string
	UserID        int64
	Side          trading.Side
	PriceAtoms    int64
	QuantityAtoms int64
	Sequence      int64
}

// Trade is an immutable maker-price execution produced by the engine.
type Trade struct {
	ID            string
	MakerOrderID  string
	TakerOrderID  string
	MakerUserID   int64
	TakerUserID   int64
	TakerSide     trading.Side
	PriceAtoms    int64
	QuantityAtoms int64
	Sequence      int64
}

// Event records the resulting lifecycle change for one order.
type Event struct {
	Type           string
	OrderID        string
	Status         trading.OrderStatus
	RemainingAtoms int64
	TradeID        string
}

// Result contains all executions and order transitions from one incoming order.
type Result struct {
	Trades                 []Trade
	Events                 []Event
	IncomingStatus         trading.OrderStatus
	IncomingRemainingAtoms int64
}

// SnapshotOrder is the durable representation of one resting book order.
type SnapshotOrder struct {
	ID             string
	UserID         int64
	Side           trading.Side
	PriceAtoms     int64
	QuantityAtoms  int64
	RemainingAtoms int64
	Sequence       int64
	Status         trading.OrderStatus
}

// Snapshot contains price-time ordered bids and asks for deterministic recovery.
type Snapshot struct {
	Bids []SnapshotOrder
	Asks []SnapshotOrder
}

type restingOrder struct {
	LimitOrder
	remaining int64
}

// Engine is an in-memory deterministic order book with no I/O or clock dependency.
type Engine struct {
	bids          map[int64][]*restingOrder
	asks          map[int64][]*restingOrder
	bidPrices     []int64
	askPrices     []int64
	byID          map[string]*restingOrder
	lastSequence  int64
	tradeSequence int64
}

// NewEngine returns an empty order book.
func NewEngine() *Engine {
	return &Engine{bids: make(map[int64][]*restingOrder), asks: make(map[int64][]*restingOrder), byID: make(map[string]*restingOrder)}
}

// NewEngineFromSnapshot validates and restores a non-crossed durable book.
func NewEngineFromSnapshot(snapshot Snapshot, lastSequence, tradeSequence int64) (*Engine, error) {
	if lastSequence < 0 || tradeSequence < 0 {
		return nil, ErrInvalidOrder
	}
	engine := NewEngine()
	items := append(append([]SnapshotOrder(nil), snapshot.Bids...), snapshot.Asks...)
	sort.Slice(items, func(i, j int) bool { return items[i].Sequence < items[j].Sequence })
	seenSequence := int64(0)
	for _, item := range items {
		if item.Sequence <= seenSequence || item.Sequence > lastSequence || item.RemainingAtoms <= 0 || item.RemainingAtoms > item.QuantityAtoms {
			return nil, ErrInvalidOrder
		}
		order := LimitOrder{ID: item.ID, UserID: item.UserID, Side: item.Side, PriceAtoms: item.PriceAtoms, QuantityAtoms: item.QuantityAtoms, Sequence: item.Sequence}
		if !order.valid() {
			return nil, ErrInvalidOrder
		}
		engine.rest(order, item.RemainingAtoms)
		seenSequence = item.Sequence
	}
	if len(engine.bidPrices) > 0 && len(engine.askPrices) > 0 && engine.bidPrices[0] >= engine.askPrices[0] {
		return nil, ErrInvalidOrder
	}
	engine.lastSequence = lastSequence
	engine.tradeSequence = tradeSequence
	return engine, nil
}

// Add matches an order at resting-maker prices, then rests any remainder.
func (e *Engine) Add(incoming LimitOrder) (Result, error) {
	if e == nil || !incoming.valid() || incoming.Sequence <= e.lastSequence {
		return Result{}, ErrInvalidOrder
	}
	if _, exists := e.byID[incoming.ID]; exists {
		return Result{}, ErrDuplicate
	}
	e.lastSequence = incoming.Sequence
	remaining := incoming.QuantityAtoms
	result := Result{Trades: make([]Trade, 0), Events: make([]Event, 0)}
	for remaining > 0 {
		maker := e.bestOpposite(incoming.Side)
		if maker == nil || !marketable(incoming.Side, incoming.PriceAtoms, maker.PriceAtoms) {
			break
		}
		quantity := min(remaining, maker.remaining)
		e.tradeSequence++
		trade := Trade{
			ID: fmt.Sprintf("trade_%020d", e.tradeSequence), MakerOrderID: maker.ID, TakerOrderID: incoming.ID,
			MakerUserID: maker.UserID, TakerUserID: incoming.UserID, TakerSide: incoming.Side,
			PriceAtoms: maker.PriceAtoms, QuantityAtoms: quantity, Sequence: e.tradeSequence,
		}
		result.Trades = append(result.Trades, trade)
		remaining -= quantity
		maker.remaining -= quantity
		makerStatus := trading.OrderStatusPartiallyFilled
		if maker.remaining == 0 {
			makerStatus = trading.OrderStatusFilled
			e.removeHead(maker)
		}
		result.Events = append(result.Events, Event{Type: "matched", OrderID: maker.ID, Status: makerStatus, RemainingAtoms: maker.remaining, TradeID: trade.ID})
	}
	switch {
	case remaining == 0:
		result.IncomingStatus = trading.OrderStatusFilled
	case remaining < incoming.QuantityAtoms:
		result.IncomingStatus = trading.OrderStatusPartiallyFilled
		e.rest(incoming, remaining)
	default:
		result.IncomingStatus = trading.OrderStatusOpen
		e.rest(incoming, remaining)
	}
	result.IncomingRemainingAtoms = remaining
	result.Events = append(result.Events, Event{Type: string(result.IncomingStatus), OrderID: incoming.ID, Status: result.IncomingStatus, RemainingAtoms: remaining})
	return result, nil
}

// Cancel removes one resting order while preserving all other FIFO positions.
func (e *Engine) Cancel(orderID string) (SnapshotOrder, error) {
	if e == nil {
		return SnapshotOrder{}, ErrNotFound
	}
	item, ok := e.byID[orderID]
	if !ok {
		return SnapshotOrder{}, ErrNotFound
	}
	levels, prices := e.levels(item.Side)
	queue := levels[item.PriceAtoms]
	for index, candidate := range queue {
		if candidate.ID == orderID {
			queue = append(queue[:index], queue[index+1:]...)
			break
		}
	}
	if len(queue) == 0 {
		delete(levels, item.PriceAtoms)
		*prices = removePrice(*prices, item.PriceAtoms)
	} else {
		levels[item.PriceAtoms] = queue
	}
	delete(e.byID, orderID)
	return snapshotOf(item, trading.OrderStatusCanceled), nil
}

// Snapshot returns a detached, price-time ordered view of the current book.
func (e *Engine) Snapshot() Snapshot {
	if e == nil {
		return Snapshot{}
	}
	return Snapshot{Bids: e.snapshotSide(trading.SideBuy), Asks: e.snapshotSide(trading.SideSell)}
}

// TradeSequence returns the last deterministic trade sequence assigned.
func (e *Engine) TradeSequence() int64 {
	if e == nil {
		return 0
	}
	return e.tradeSequence
}

func (e *Engine) rest(order LimitOrder, remaining int64) {
	item := &restingOrder{LimitOrder: order, remaining: remaining}
	levels, prices := e.levels(order.Side)
	if _, exists := levels[order.PriceAtoms]; !exists {
		*prices = append(*prices, order.PriceAtoms)
		if order.Side == trading.SideBuy {
			sort.Slice(*prices, func(i, j int) bool { return (*prices)[i] > (*prices)[j] })
		} else {
			sort.Slice(*prices, func(i, j int) bool { return (*prices)[i] < (*prices)[j] })
		}
	}
	levels[order.PriceAtoms] = append(levels[order.PriceAtoms], item)
	e.byID[order.ID] = item
}

func (e *Engine) bestOpposite(side trading.Side) *restingOrder {
	if side == trading.SideBuy {
		if len(e.askPrices) == 0 {
			return nil
		}
		return e.asks[e.askPrices[0]][0]
	}
	if len(e.bidPrices) == 0 {
		return nil
	}
	return e.bids[e.bidPrices[0]][0]
}

func (e *Engine) removeHead(item *restingOrder) {
	levels, prices := e.levels(item.Side)
	queue := levels[item.PriceAtoms]
	if len(queue) <= 1 {
		delete(levels, item.PriceAtoms)
		*prices = removePrice(*prices, item.PriceAtoms)
	} else {
		levels[item.PriceAtoms] = queue[1:]
	}
	delete(e.byID, item.ID)
}

func (e *Engine) levels(side trading.Side) (map[int64][]*restingOrder, *[]int64) {
	if side == trading.SideBuy {
		return e.bids, &e.bidPrices
	}
	return e.asks, &e.askPrices
}

func (e *Engine) snapshotSide(side trading.Side) []SnapshotOrder {
	levels, prices := e.levels(side)
	result := make([]SnapshotOrder, 0, len(e.byID))
	for _, price := range *prices {
		for _, item := range levels[price] {
			result = append(result, snapshotOf(item, trading.OrderStatusOpen))
		}
	}
	return result
}

func snapshotOf(item *restingOrder, status trading.OrderStatus) SnapshotOrder {
	return SnapshotOrder{ID: item.ID, UserID: item.UserID, Side: item.Side, PriceAtoms: item.PriceAtoms, QuantityAtoms: item.QuantityAtoms, RemainingAtoms: item.remaining, Sequence: item.Sequence, Status: status}
}

func (o LimitOrder) valid() bool {
	return o.ID != "" && len(o.ID) <= 128 && o.UserID > 0 && (o.Side == trading.SideBuy || o.Side == trading.SideSell) && o.PriceAtoms > 0 && o.QuantityAtoms > 0 && o.Sequence > 0
}

func marketable(side trading.Side, takerPrice, makerPrice int64) bool {
	if side == trading.SideBuy {
		return takerPrice >= makerPrice
	}
	return takerPrice <= makerPrice
}

func removePrice(prices []int64, price int64) []int64 {
	for index, candidate := range prices {
		if candidate == price {
			return append(prices[:index], prices[index+1:]...)
		}
	}
	return prices
}
