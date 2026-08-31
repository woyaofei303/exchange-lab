package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/woyaofei303/exchange-lab/internal/ledger"
	"github.com/woyaofei303/exchange-lab/internal/order"
	"github.com/woyaofei303/exchange-lab/internal/trading"
)

// OrderRepository owns atomic order, margin reserve, ledger and Outbox writes.
type OrderRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
	now     func() time.Time
}

// NewOrderRepository constructs a context-bounded order repository.
func NewOrderRepository(pool *pgxpool.Pool, timeout time.Duration) *OrderRepository {
	return &OrderRepository{pool: pool, timeout: timeout, now: time.Now}
}

// Place retries serialization failures around one idempotent order transaction.
func (r *OrderRepository) Place(ctx context.Context, placed order.Order, key string, requestHash [32]byte) (order.Result, error) {
	return retrySerialization(ctx, 3, func() (order.Result, error) {
		return r.placeOnce(ctx, placed, key, requestHash)
	})
}

func (r *OrderRepository) placeOnce(ctx context.Context, placed order.Order, key string, requestHash [32]byte) (order.Result, error) {
	if r == nil || r.pool == nil || r.timeout <= 0 || placed.Status != trading.OrderStatusOpen || placed.ReservedMarginAtoms <= 0 {
		return order.Result{}, order.ErrInvalid
	}
	operation, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	tx, err := r.pool.BeginTx(operation, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return order.Result{}, fmt.Errorf("place order: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := lockIdempotency(operation, tx, placed.UserID, "order.place", key); err != nil {
		return order.Result{}, err
	}
	if result, found, err := replayOrder(operation, tx, placed.UserID, "order.place", key, requestHash); err != nil {
		return order.Result{}, err
	} else if found {
		if err := tx.Commit(operation); err != nil {
			return order.Result{}, fmt.Errorf("place order replay: commit: %w", err)
		}
		return result, nil
	}

	availableAccount, lockedAccount, current, err := lockOrderBalance(operation, tx, placed.UserID)
	if err != nil {
		return order.Result{}, err
	}
	updated, err := current.Lock(placed.ReservedMarginAtoms)
	if err != nil {
		return order.Result{}, ledger.ErrInsufficient
	}
	ledgerID, err := randomLedgerID()
	if err != nil {
		return order.Result{}, err
	}
	transaction, err := ledger.NewTransaction(ledgerID, "order_margin_reserve", "reserve:"+placed.ID, []ledger.Entry{
		{AccountID: lockedAccount, Asset: "USDT", AmountAtoms: placed.ReservedMarginAtoms},
		{AccountID: availableAccount, Asset: "USDT", AmountAtoms: -placed.ReservedMarginAtoms},
	})
	if err != nil {
		return order.Result{}, err
	}
	if err := insertLedgerTransaction(operation, tx, placed.UserID, transaction, placed.CreatedAt); err != nil {
		return order.Result{}, err
	}
	if _, err := tx.Exec(operation, `UPDATE balances SET available_atoms=$2, locked_atoms=$3, version=version+1, updated_at=$4 WHERE user_id=$1 AND asset='USDT'`, placed.UserID, updated.AvailableAtoms, updated.LockedAtoms, placed.UpdatedAt); err != nil {
		return order.Result{}, fmt.Errorf("place order: update balance: %w", err)
	}
	if err := insertOrder(operation, tx, placed); err != nil {
		return order.Result{}, err
	}
	if err := appendOrderFacts(operation, tx, placed, "accepted", "orders.accepted", key, placed.CreatedAt); err != nil {
		return order.Result{}, err
	}
	if _, err := tx.Exec(operation, `INSERT INTO idempotency_keys (user_id, operation, idempotency_key, request_hash, resource_id, created_at) VALUES ($1,'order.place',$2,$3,$4,$5)`, placed.UserID, key, requestHash[:], placed.ID, placed.CreatedAt); err != nil {
		return order.Result{}, fmt.Errorf("place order: store idempotency: %w", err)
	}
	metadata := fmt.Sprintf(`{"symbol":%q,"side":%q}`, placed.Symbol, placed.Side)
	if _, err := tx.Exec(operation, `INSERT INTO audit_logs (actor_user_id,action,resource_type,resource_id,metadata,occurred_at) VALUES ($1,'order.accepted','order',$2,$3,$4)`, placed.UserID, placed.ID, metadata, placed.CreatedAt); err != nil {
		return order.Result{}, fmt.Errorf("place order: audit: %w", err)
	}
	if err := tx.Commit(operation); err != nil {
		return order.Result{}, fmt.Errorf("place order: commit: %w", err)
	}
	return order.Result{Order: placed}, nil
}

// Cancel releases remaining reserve and emits cancellation facts atomically.
func (r *OrderRepository) Cancel(ctx context.Context, userID int64, orderID, key string, requestHash [32]byte) (order.Result, error) {
	return retrySerialization(ctx, 3, func() (order.Result, error) {
		return r.cancelOnce(ctx, userID, orderID, key, requestHash)
	})
}

func (r *OrderRepository) cancelOnce(ctx context.Context, userID int64, orderID, key string, requestHash [32]byte) (order.Result, error) {
	operation, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	tx, err := r.pool.BeginTx(operation, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return order.Result{}, fmt.Errorf("cancel order: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := lockIdempotency(operation, tx, userID, "order.cancel", key); err != nil {
		return order.Result{}, err
	}
	if result, found, err := replayOrder(operation, tx, userID, "order.cancel", key, requestHash); err != nil {
		return order.Result{}, err
	} else if found {
		if err := tx.Commit(operation); err != nil {
			return order.Result{}, fmt.Errorf("cancel order replay: commit: %w", err)
		}
		return result, nil
	}
	stored, err := scanOrder(tx.QueryRow(operation, `SELECT id,user_id,client_order_id,symbol,side,order_type,status,price_atoms,price_scale,quantity_atoms,remaining_atoms,quantity_scale,quote_scale,leverage,reserved_margin_atoms,created_at,updated_at FROM orders WHERE id=$1 FOR UPDATE`, orderID))
	if errors.Is(err, pgx.ErrNoRows) {
		return order.Result{}, order.ErrNotFound
	}
	if err != nil {
		return order.Result{}, err
	}
	if stored.UserID != userID {
		return order.Result{}, order.ErrForbidden
	}
	canceled, released, err := stored.Cancel()
	if err != nil {
		return order.Result{}, err
	}
	canceled.UpdatedAt = r.now().UTC()
	availableAccount, lockedAccount, current, err := lockOrderBalance(operation, tx, userID)
	if err != nil {
		return order.Result{}, err
	}
	updated, err := current.Release(released)
	if err != nil {
		return order.Result{}, err
	}
	ledgerID, err := randomLedgerID()
	if err != nil {
		return order.Result{}, err
	}
	transaction, err := ledger.NewTransaction(ledgerID, "order_margin_release", "cancel:"+canceled.ID, []ledger.Entry{
		{AccountID: availableAccount, Asset: "USDT", AmountAtoms: released},
		{AccountID: lockedAccount, Asset: "USDT", AmountAtoms: -released},
	})
	if err != nil {
		return order.Result{}, err
	}
	if err := insertLedgerTransaction(operation, tx, userID, transaction, canceled.UpdatedAt); err != nil {
		return order.Result{}, err
	}
	if _, err := tx.Exec(operation, `UPDATE balances SET available_atoms=$2,locked_atoms=$3,version=version+1,updated_at=$4 WHERE user_id=$1 AND asset='USDT'`, userID, updated.AvailableAtoms, updated.LockedAtoms, canceled.UpdatedAt); err != nil {
		return order.Result{}, fmt.Errorf("cancel order: update balance: %w", err)
	}
	if _, err := tx.Exec(operation, `UPDATE orders SET status='canceled',reserved_margin_atoms=0,updated_at=$2 WHERE id=$1`, canceled.ID, canceled.UpdatedAt); err != nil {
		return order.Result{}, fmt.Errorf("cancel order: update order: %w", err)
	}
	if err := appendOrderFacts(operation, tx, canceled, "canceled", "orders.cancel_requested", key, canceled.UpdatedAt); err != nil {
		return order.Result{}, err
	}
	if _, err := tx.Exec(operation, `INSERT INTO idempotency_keys (user_id,operation,idempotency_key,request_hash,resource_id,created_at) VALUES ($1,'order.cancel',$2,$3,$4,$5)`, userID, key, requestHash[:], canceled.ID, canceled.UpdatedAt); err != nil {
		return order.Result{}, fmt.Errorf("cancel order: store idempotency: %w", err)
	}
	if _, err := tx.Exec(operation, `INSERT INTO audit_logs (actor_user_id,action,resource_type,resource_id,metadata,occurred_at) VALUES ($1,'order.canceled','order',$2,'{}',$3)`, userID, canceled.ID, canceled.UpdatedAt); err != nil {
		return order.Result{}, fmt.Errorf("cancel order: audit: %w", err)
	}
	if err := tx.Commit(operation); err != nil {
		return order.Result{}, fmt.Errorf("cancel order: commit: %w", err)
	}
	return order.Result{Order: canceled}, nil
}

// List returns a bounded newest-first order history for one user.
func (r *OrderRepository) List(ctx context.Context, userID int64, limit int) ([]order.Order, error) {
	operation, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	rows, err := r.pool.Query(operation, `SELECT id,user_id,client_order_id,symbol,side,order_type,status,price_atoms,price_scale,quantity_atoms,remaining_atoms,quantity_scale,quote_scale,leverage,reserved_margin_atoms,created_at,updated_at FROM orders WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()
	result := make([]order.Order, 0, limit)
	for rows.Next() {
		item, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func lockIdempotency(ctx context.Context, tx pgx.Tx, userID int64, operation, key string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("%d:%s:%s", userID, operation, key))
	if err != nil {
		return fmt.Errorf("order idempotency lock: %w", err)
	}
	return nil
}

func replayOrder(ctx context.Context, tx pgx.Tx, userID int64, operation, key string, hash [32]byte) (order.Result, bool, error) {
	var storedHash []byte
	var resourceID string
	err := tx.QueryRow(ctx, `SELECT request_hash,resource_id FROM idempotency_keys WHERE user_id=$1 AND operation=$2 AND idempotency_key=$3`, userID, operation, key).Scan(&storedHash, &resourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return order.Result{}, false, nil
	}
	if err != nil {
		return order.Result{}, false, fmt.Errorf("read order idempotency: %w", err)
	}
	if !bytes.Equal(storedHash, hash[:]) {
		return order.Result{}, false, order.ErrConflict
	}
	stored, err := scanOrder(tx.QueryRow(ctx, `SELECT id,user_id,client_order_id,symbol,side,order_type,status,price_atoms,price_scale,quantity_atoms,remaining_atoms,quantity_scale,quote_scale,leverage,reserved_margin_atoms,created_at,updated_at FROM orders WHERE id=$1`, resourceID))
	if err != nil {
		return order.Result{}, false, err
	}
	return order.Result{Order: stored, Replayed: true}, true, nil
}

func lockOrderBalance(ctx context.Context, tx pgx.Tx, userID int64) (int64, int64, ledger.Balance, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO ledger_accounts(owner_user_id,asset,account_type) VALUES ($1,'USDT','available'),($1,'USDT','locked') ON CONFLICT DO NOTHING`, userID); err != nil {
		return 0, 0, ledger.Balance{}, fmt.Errorf("order balance: create accounts: %w", err)
	}
	var available, locked int64
	if err := tx.QueryRow(ctx, `SELECT max(id) FILTER (WHERE account_type='available'),max(id) FILTER (WHERE account_type='locked') FROM ledger_accounts WHERE owner_user_id=$1 AND asset='USDT'`, userID).Scan(&available, &locked); err != nil {
		return 0, 0, ledger.Balance{}, fmt.Errorf("order balance: find accounts: %w", err)
	}
	var balance ledger.Balance
	if err := tx.QueryRow(ctx, `SELECT available_atoms,locked_atoms FROM balances WHERE user_id=$1 AND asset='USDT' FOR UPDATE`, userID).Scan(&balance.AvailableAtoms, &balance.LockedAtoms); errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, ledger.Balance{}, ledger.ErrInsufficient
	} else if err != nil {
		return 0, 0, ledger.Balance{}, fmt.Errorf("order balance: lock: %w", err)
	}
	return available, locked, balance, nil
}

func insertLedgerTransaction(ctx context.Context, tx pgx.Tx, userID int64, transaction ledger.Transaction, at time.Time) error {
	if _, err := tx.Exec(ctx, `INSERT INTO ledger_transactions(id,user_id,kind,idempotency_key,created_at) VALUES ($1,$2,$3,$4,$5)`, transaction.ID, userID, transaction.Kind, transaction.IdempotencyKey, at); err != nil {
		return fmt.Errorf("order ledger: insert transaction: %w", err)
	}
	for _, entry := range transaction.Entries {
		if _, err := tx.Exec(ctx, `INSERT INTO ledger_entries(transaction_id,account_id,asset,amount_atoms,created_at) VALUES ($1,$2,$3,$4,$5)`, transaction.ID, entry.AccountID, entry.Asset, entry.AmountAtoms, at); err != nil {
			return fmt.Errorf("order ledger: insert entry: %w", err)
		}
	}
	return nil
}

func insertOrder(ctx context.Context, tx pgx.Tx, placed order.Order) error {
	_, err := tx.Exec(ctx, `INSERT INTO orders(id,user_id,client_order_id,symbol,side,order_type,status,price_atoms,price_scale,quantity_atoms,remaining_atoms,quantity_scale,quote_scale,leverage,reserved_margin_atoms,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, placed.ID, placed.UserID, placed.ClientOrderID, placed.Symbol, placed.Side, placed.Type, placed.Status, placed.PriceAtoms, placed.PriceScale, placed.QuantityAtoms, placed.RemainingAtoms, placed.QuantityScale, placed.QuoteScale, placed.Leverage, placed.ReservedMarginAtoms, placed.CreatedAt, placed.UpdatedAt)
	if err != nil {
		return fmt.Errorf("place order: insert: %w", err)
	}
	return nil
}

func appendOrderFacts(ctx context.Context, tx pgx.Tx, placed order.Order, eventType, subject, correlation string, at time.Time) error {
	payload, err := json.Marshal(orderPayload(placed))
	if err != nil {
		return order.ErrInvalid
	}
	eventID, err := randomBusinessID("oev_")
	if err != nil {
		return err
	}
	outboxID, err := randomBusinessID("out_")
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO order_events(id,order_id,event_type,payload,occurred_at) VALUES ($1,$2,$3,$4,$5)`, eventID, placed.ID, eventType, payload, at); err != nil {
		return fmt.Errorf("order facts: event: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,subject,correlation_id,payload,available_at,created_at) VALUES ($1,'order',$2,$3,$4,$5,$6,$6)`, outboxID, placed.ID, subject, correlation, payload, at); err != nil {
		return fmt.Errorf("order facts: outbox: %w", err)
	}
	return nil
}

func orderPayload(placed order.Order) map[string]any {
	price, _ := trading.FormatDecimal(placed.PriceAtoms, placed.PriceScale)
	quantity, _ := trading.FormatDecimal(placed.QuantityAtoms, placed.QuantityScale)
	remaining, _ := trading.FormatDecimal(placed.RemainingAtoms, placed.QuantityScale)
	reserved, _ := trading.FormatDecimal(placed.ReservedMarginAtoms, placed.QuoteScale)
	return map[string]any{"version": 1, "order_id": placed.ID, "user_id": placed.UserID, "symbol": placed.Symbol, "side": placed.Side, "type": placed.Type, "status": placed.Status, "price": price, "quantity": quantity, "remaining": remaining, "leverage": placed.Leverage, "reserved_margin": reserved, "occurred_at": placed.UpdatedAt.UTC().Format(time.RFC3339Nano)}
}

func scanOrder(row rowScanner) (order.Order, error) {
	var item order.Order
	err := row.Scan(&item.ID, &item.UserID, &item.ClientOrderID, &item.Symbol, &item.Side, &item.Type, &item.Status, &item.PriceAtoms, &item.PriceScale, &item.QuantityAtoms, &item.RemainingAtoms, &item.QuantityScale, &item.QuoteScale, &item.Leverage, &item.ReservedMarginAtoms, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return order.Order{}, pgx.ErrNoRows
		}
		return order.Order{}, fmt.Errorf("scan order: %w", err)
	}
	return item, nil
}

func randomBusinessID(prefix string) (string, error) {
	id, err := randomLedgerID()
	if err != nil {
		return "", err
	}
	return prefix + id[4:], nil
}
