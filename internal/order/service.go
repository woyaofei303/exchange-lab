package order

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"exchange-lab/internal/trading"
)

// InstrumentReader 获取合约的权威过滤规则。
type InstrumentReader interface {
	Instruments(context.Context, string) ([]trading.Instrument, error)
}

// Store 原子提交订单、预留资金、账本条目和 Outbox 事实。
type Store interface {
	Place(context.Context, Order, string, [32]byte) (Result, error)
	Cancel(context.Context, int64, string, string, [32]byte) (Result, error)
	List(context.Context, int64, int) ([]Order, error)
}

// Result 标识幂等命令是否重放了先前结果。
type Result struct {
	Order    Order
	Replayed bool
}

// PlaceCommand 是来自传输层的不可信限价单输入。
type PlaceCommand struct {
	Symbol         string
	Side           string
	Price          string
	Quantity       string
	Leverage       int64
	ClientOrderID  string
	IdempotencyKey string
}

// Service 在跨越原子 Store 边界前校验命令。
type Service struct {
	instruments InstrumentReader
	store       Store
	now         func() time.Time
}

// NewService 校验订单服务依赖项。
func NewService(instruments InstrumentReader, store Store, now func() time.Time) (*Service, error) {
	if instruments == nil || store == nil || now == nil {
		return nil, ErrInvalid
	}
	return &Service{instruments: instruments, store: store, now: now}, nil
}

// Place 规范化精确数值并提交一个幂等原子命令。
func (s *Service) Place(ctx context.Context, userID int64, command PlaceCommand) (Result, error) {
	if userID <= 0 || !validKey(command.IdempotencyKey) {
		return Result{}, ErrInvalid
	}
	instruments, err := s.instruments.Instruments(ctx, command.Symbol)
	if err != nil {
		return Result{}, err
	}
	if len(instruments) != 1 || instruments[0].Symbol() != command.Symbol {
		return Result{}, ErrInvalid
	}
	side, err := trading.ParseSide(command.Side)
	if err != nil {
		return Result{}, ErrInvalid
	}
	price, err := instruments[0].ParsePrice(command.Price)
	if err != nil {
		return Result{}, ErrInvalid
	}
	quantity, err := instruments[0].ParseQuantity(command.Quantity)
	if err != nil {
		return Result{}, ErrInvalid
	}
	id, err := randomID("ord_")
	if err != nil {
		return Result{}, err
	}
	created, err := NewLimit(id, userID, command.ClientOrderID, instruments[0], side, price, quantity, command.Leverage)
	if err != nil {
		return Result{}, ErrInvalid
	}
	created, err = created.Accept()
	if err != nil {
		return Result{}, err
	}
	now := s.now().UTC()
	created.CreatedAt, created.UpdatedAt = now, now
	return s.store.Place(ctx, created, command.IdempotencyKey, hashPlace(command, side))
}

// Cancel 将撤单限定到用户和稳定的幂等哈希范围。
func (s *Service) Cancel(ctx context.Context, userID int64, orderID, idempotencyKey string) (Result, error) {
	if userID <= 0 || orderID == "" || !validKey(idempotencyKey) {
		return Result{}, ErrInvalid
	}
	hash := sha256.Sum256([]byte("cancel\x00" + strconv.FormatInt(userID, 10) + "\x00" + orderID))
	return s.store.Cancel(ctx, userID, orderID, idempotencyKey, hash)
}

// List 返回有界且限定到用户范围的订单历史。
func (s *Service) List(ctx context.Context, userID int64, limit int) ([]Order, error) {
	if userID <= 0 || limit < 1 || limit > 200 {
		return nil, ErrInvalid
	}
	return s.store.List(ctx, userID, limit)
}

func hashPlace(command PlaceCommand, side trading.Side) [32]byte {
	canonical := strings.Join([]string{"place", command.Symbol, side.String(), command.Price, command.Quantity, strconv.FormatInt(command.Leverage, 10), command.ClientOrderID}, "\x00")
	return sha256.Sum256([]byte(canonical))
}

func validKey(key string) bool {
	return len(key) >= 8 && len(key) <= 128 && strings.TrimSpace(key) == key
}

func randomID(prefix string) (string, error) {
	payload := make([]byte, 16)
	if _, err := rand.Read(payload); err != nil {
		return "", fmt.Errorf("create order identifier: entropy unavailable")
	}
	return prefix + hex.EncodeToString(payload), nil
}
