// Package order 校验限价单状态并协调原子持久化。
package order

import (
	"errors"
	"fmt"
	"math/big"
	"time"

	"exchange-lab/internal/trading"
)

// 订单错误按校验、生命周期、所有权和重放冲突分类。
var (
	ErrInvalid           = errors.New("invalid order")
	ErrInvalidTransition = errors.New("invalid order transition")
	ErrNotFound          = errors.New("order not found")
	ErrForbidden         = errors.New("order forbidden")
	ErrConflict          = errors.New("order idempotency conflict")
)

// Order 表示一笔用户限价单的精确持久化状态。
type Order struct {
	ID                  string
	UserID              int64
	ClientOrderID       string
	Symbol              string
	Side                trading.Side
	Type                trading.OrderType
	Status              trading.OrderStatus
	PriceAtoms          int64
	PriceScale          uint8
	QuantityAtoms       int64
	RemainingAtoms      int64
	QuantityScale       uint8
	QuoteScale          uint8
	Leverage            int64
	ReservedMarginAtoms int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// NewLimit 校验交易品种过滤规则，并预留向上取整的初始保证金。
func NewLimit(id string, userID int64, clientOrderID string, instrument trading.Instrument, side trading.Side, price trading.Price, quantity trading.Quantity, leverage int64) (Order, error) {
	if len(id) < 1 || len(id) > 128 || userID <= 0 || len(clientOrderID) < 8 || len(clientOrderID) > 128 || leverage < 1 || leverage > 125 {
		return Order{}, ErrInvalid
	}
	if side != trading.SideBuy && side != trading.SideSell {
		return Order{}, ErrInvalid
	}
	if err := instrument.ValidateLimitOrder(price, quantity); err != nil {
		return Order{}, fmt.Errorf("create limit order: %w", err)
	}
	notional, err := trading.CalculateNotional(price, quantity, instrument.Quote())
	if err != nil {
		return Order{}, fmt.Errorf("create limit order: %w", err)
	}
	reserved, err := divideCeilPositive(notional.Atoms(), leverage)
	if err != nil {
		return Order{}, err
	}
	return Order{
		ID: id, UserID: userID, ClientOrderID: clientOrderID, Symbol: instrument.Symbol(),
		Side: side, Type: trading.OrderTypeLimit, Status: trading.OrderStatusPending,
		PriceAtoms: price.Atoms(), PriceScale: price.Scale(), QuantityAtoms: quantity.Atoms(),
		RemainingAtoms: quantity.Atoms(), QuantityScale: quantity.Scale(), QuoteScale: instrument.Quote().Scale(),
		Leverage: leverage, ReservedMarginAtoms: reserved,
	}, nil
}

// Accept 将已校验的待处理订单转换为开放状态。
func (o Order) Accept() (Order, error) {
	if o.Status != trading.OrderStatusPending || !o.valid() {
		return Order{}, ErrInvalidTransition
	}
	o.Status = trading.OrderStatusOpen
	return o, nil
}

// Cancel 关闭有效订单，并返回应释放的保证金最小单位数量。
func (o Order) Cancel() (Order, int64, error) {
	if (o.Status != trading.OrderStatusPending && o.Status != trading.OrderStatusOpen && o.Status != trading.OrderStatusPartiallyFilled) || !o.valid() {
		return Order{}, 0, ErrInvalidTransition
	}
	released := o.ReservedMarginAtoms
	o.ReservedMarginAtoms = 0
	o.Status = trading.OrderStatusCanceled
	return o, released, nil
}

func (o Order) valid() bool {
	return o.ID != "" && o.UserID > 0 && o.Symbol != "" && o.PriceAtoms > 0 && o.QuantityAtoms > 0 && o.RemainingAtoms >= 0 && o.RemainingAtoms <= o.QuantityAtoms && o.Leverage > 0 && o.ReservedMarginAtoms >= 0
}

func divideCeilPositive(numerator, denominator int64) (int64, error) {
	if numerator <= 0 || denominator <= 0 {
		return 0, ErrInvalid
	}
	value := new(big.Int).SetInt64(numerator)
	divisor := big.NewInt(denominator)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(value, divisor, remainder)
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0, ErrInvalid
	}
	return quotient.Int64(), nil
}
