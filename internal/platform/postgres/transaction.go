package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Transactor 管理事务截止时间和提交/回滚行为。
type Transactor struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewTransactor 创建带固定截止时间的事务边界。
func NewTransactor(pool *pgxpool.Pool, timeout time.Duration) *Transactor {
	return &Transactor{pool: pool, timeout: timeout}
}

// WithinTx 在一个事务中运行 fn。回调返回错误时向上返回并回滚事务；
// 回调返回 nil 时提交事务。
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

// retrySerialization 在 PostgreSQL 以 SQLSTATE 40001 中止 Serializable 事务后，
// 重试幂等操作。调用方必须限制重试次数，并确保每次尝试创建新事务。
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
