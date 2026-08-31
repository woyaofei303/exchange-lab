package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Transactor owns transaction deadlines and commit/rollback behavior.
type Transactor struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewTransactor constructs a transaction boundary with a fixed deadline.
func NewTransactor(pool *pgxpool.Pool, timeout time.Duration) *Transactor {
	return &Transactor{pool: pool, timeout: timeout}
}

// WithinTx runs fn inside one transaction. A callback error is returned and
// the transaction is rolled back; a nil callback result is committed.
func (t *Transactor) WithinTx(ctx context.Context, options pgx.TxOptions, fn func(context.Context, pgx.Tx) error) error {
	if t == nil || t.pool == nil || t.timeout <= 0 || fn == nil {
		return fmt.Errorf("run postgres transaction: invalid configuration")
	}
	txContext, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	tx, err := t.pool.BeginTx(txContext, options)
	if err != nil {
		return fmt.Errorf("begin postgres transaction: %w", err)
	}
	if err := fn(txContext, tx); err != nil {
		if rollbackErr := tx.Rollback(txContext); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return errors.Join(err, fmt.Errorf("rollback postgres transaction: %w", rollbackErr))
		}
		return err
	}
	if err := tx.Commit(txContext); err != nil {
		return fmt.Errorf("commit postgres transaction: %w", err)
	}
	return nil
}

// retrySerialization retries an idempotent operation after PostgreSQL aborts a
// Serializable transaction with SQLSTATE 40001. Callers must keep the retry
// count bounded and ensure that each attempt creates a fresh transaction.
func retrySerialization[T any](ctx context.Context, attempts int, operation func() (T, error)) (T, error) {
	var zero T
	if attempts < 1 || operation == nil {
		return zero, fmt.Errorf("retry postgres serialization: invalid configuration")
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		result, err := operation()
		if err == nil || !IsSerializationFailure(err) {
			return result, err
		}
		lastErr = err
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		if attempt+1 == attempts {
			break
		}
		delay := time.Duration(1<<attempt) * 5 * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return zero, ctx.Err()
		case <-timer.C:
		}
	}
	return zero, lastErr
}
