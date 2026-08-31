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

	"github.com/woyaofei303/exchange-lab/internal/trading"
)

// InstrumentReader resolves the authoritative filters for a contract.
type InstrumentReader interface {
	Instruments(context.Context, string) ([]trading.Instrument, error)
}

// Store commits orders, reserves, ledger entries and Outbox facts atomically.
type Store interface {
	Place(context.Context, Order, string, [32]byte) (Result, error)
	Cancel(context.Context, int64, string, string, [32]byte) (Result, error)
	List(context.Context, int64, int) ([]Order, error)
}

// Result identifies whether an idempotent command replayed a prior result.
type Result struct {
	Order    Order
	Replayed bool
}

// PlaceCommand is the untrusted transport input for a limit order.
type PlaceCommand struct {
	Symbol         string
	Side           string
	Price          string
	Quantity       string
	Leverage       int64
	ClientOrderID  string
	IdempotencyKey string
}

// Service validates commands before crossing the atomic Store boundary.
type Service struct {
	instruments InstrumentReader
	store       Store
	now         func() time.Time
}

// NewService validates order service dependencies.
func NewService(instruments InstrumentReader, store Store, now func() time.Time) (*Service, error) {
	if instruments == nil || store == nil || now == nil {
		return nil, ErrInvalid
	}
	return &Service{instruments: instruments, store: store, now: now}, nil
}

// Place canonicalizes exact values and submits one idempotent atomic command.
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

// Cancel scopes cancellation to a user and stable idempotency hash.
func (s *Service) Cancel(ctx context.Context, userID int64, orderID, idempotencyKey string) (Result, error) {
	if userID <= 0 || orderID == "" || !validKey(idempotencyKey) {
		return Result{}, ErrInvalid
	}
	hash := sha256.Sum256([]byte("cancel\x00" + strconv.FormatInt(userID, 10) + "\x00" + orderID))
	return s.store.Cancel(ctx, userID, orderID, idempotencyKey, hash)
}

// List returns a bounded user-scoped order history.
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
