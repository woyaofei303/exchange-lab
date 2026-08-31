package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/woyaofei303/exchange-lab/internal/ledger"
)

const maximumFaucetAtoms int64 = 10_000 * 100_000_000

// LedgerRepository commits exact balances and double-entry facts atomically.
type LedgerRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
	now     func() time.Time
}

// NewLedgerRepository constructs a context-bounded ledger repository.
func NewLedgerRepository(pool *pgxpool.Pool, timeout time.Duration) *LedgerRepository {
	return &LedgerRepository{pool: pool, timeout: timeout, now: time.Now}
}

// Faucet credits simulated funds once per idempotency key in a Serializable transaction.
func (r *LedgerRepository) Faucet(ctx context.Context, userID, amountAtoms int64, idempotencyKey string) (ledger.FaucetResult, error) {
	if r == nil || r.pool == nil || r.timeout <= 0 || userID <= 0 || amountAtoms <= 0 || amountAtoms > maximumFaucetAtoms || len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		return ledger.FaucetResult{}, ledger.ErrInvalid
	}
	operation, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	tx, err := r.pool.BeginTx(operation, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(operation, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, idempotencyKey); err != nil {
		return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: idempotency lock: %w", err)
	}
	if result, found, err := existingFaucet(operation, tx, userID, amountAtoms, idempotencyKey); err != nil {
		return ledger.FaucetResult{}, err
	} else if found {
		if err := tx.Commit(operation); err != nil {
			return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: replay commit: %w", err)
		}
		return result, nil
	}

	var userAccount, systemAccount int64
	if _, err := tx.Exec(operation, `
		INSERT INTO ledger_accounts (owner_user_id, asset, account_type)
		VALUES ($1, 'USDT', 'available') ON CONFLICT DO NOTHING
	`, userID); err != nil {
		return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: create account: %w", err)
	}
	if err := tx.QueryRow(operation, `SELECT id FROM ledger_accounts WHERE owner_user_id = $1 AND asset = 'USDT' AND account_type = 'available'`, userID).Scan(&userAccount); err != nil {
		return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: find user account: %w", err)
	}
	if err := tx.QueryRow(operation, `SELECT id FROM ledger_accounts WHERE owner_user_id IS NULL AND asset = 'USDT' AND account_type = 'system_faucet'`).Scan(&systemAccount); err != nil {
		return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: find system account: %w", err)
	}
	if _, err := tx.Exec(operation, `INSERT INTO balances (user_id, asset) VALUES ($1, 'USDT') ON CONFLICT DO NOTHING`, userID); err != nil {
		return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: create balance: %w", err)
	}
	var current ledger.Balance
	if err := tx.QueryRow(operation, `SELECT available_atoms, locked_atoms FROM balances WHERE user_id = $1 AND asset = 'USDT' FOR UPDATE`, userID).Scan(&current.AvailableAtoms, &current.LockedAtoms); err != nil {
		return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: lock balance: %w", err)
	}
	updated, err := current.Credit(amountAtoms)
	if err != nil || updated.AvailableAtoms > math.MaxInt64-updated.LockedAtoms {
		return ledger.FaucetResult{}, ledger.ErrInvalid
	}
	transactionID, err := randomLedgerID()
	if err != nil {
		return ledger.FaucetResult{}, err
	}
	domainTransaction, err := ledger.NewTransaction(transactionID, "faucet", idempotencyKey, []ledger.Entry{
		{AccountID: userAccount, Asset: "USDT", AmountAtoms: amountAtoms},
		{AccountID: systemAccount, Asset: "USDT", AmountAtoms: -amountAtoms},
	})
	if err != nil {
		return ledger.FaucetResult{}, err
	}
	now := r.now().UTC()
	if _, err := tx.Exec(operation, `INSERT INTO ledger_transactions (id, user_id, kind, idempotency_key, created_at) VALUES ($1, $2, $3, $4, $5)`,
		domainTransaction.ID, userID, domainTransaction.Kind, domainTransaction.IdempotencyKey, now); err != nil {
		return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: insert transaction: %w", err)
	}
	for _, entry := range domainTransaction.Entries {
		if _, err := tx.Exec(operation, `INSERT INTO ledger_entries (transaction_id, account_id, asset, amount_atoms, created_at) VALUES ($1, $2, $3, $4, $5)`,
			domainTransaction.ID, entry.AccountID, entry.Asset, entry.AmountAtoms, now); err != nil {
			return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: insert entry: %w", err)
		}
	}
	var entryCount int
	var entryTotal int64
	if err := tx.QueryRow(operation, `SELECT count(*), COALESCE(sum(amount_atoms), 0) FROM ledger_entries WHERE transaction_id = $1`, domainTransaction.ID).Scan(&entryCount, &entryTotal); err != nil || entryCount < 2 || entryTotal != 0 {
		return ledger.FaucetResult{}, ledger.ErrUnbalanced
	}
	if _, err := tx.Exec(operation, `UPDATE balances SET available_atoms = $2, version = version + 1, updated_at = $3 WHERE user_id = $1 AND asset = 'USDT'`, userID, updated.AvailableAtoms, now); err != nil {
		return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: update balance: %w", err)
	}
	if _, err := tx.Exec(operation, `INSERT INTO faucet_requests (idempotency_key, user_id, asset, amount_atoms, transaction_id, created_at) VALUES ($1, $2, 'USDT', $3, $4, $5)`, idempotencyKey, userID, amountAtoms, domainTransaction.ID, now); err != nil {
		return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: insert request: %w", err)
	}
	metadata := fmt.Sprintf(`{"amount":"%s","asset":"USDT"}`, formatUSDT(amountAtoms))
	if _, err := tx.Exec(operation, `INSERT INTO audit_logs (actor_user_id, action, resource_type, resource_id, metadata, occurred_at) VALUES ($1, 'ledger.faucet_credited', 'ledger_transaction', $2, $3, $4)`, userID, domainTransaction.ID, metadata, now); err != nil {
		return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: audit: %w", err)
	}
	if err := tx.Commit(operation); err != nil {
		return ledger.FaucetResult{}, fmt.Errorf("faucet ledger: commit: %w", err)
	}
	return ledger.FaucetResult{TransactionID: domainTransaction.ID, Balance: updated}, nil
}

// Balance returns the user's materialized USDT balance or zero when absent.
func (r *LedgerRepository) Balance(ctx context.Context, userID int64) (ledger.Balance, error) {
	operation, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	var balance ledger.Balance
	err := r.pool.QueryRow(operation, `SELECT available_atoms, locked_atoms FROM balances WHERE user_id = $1 AND asset = 'USDT'`, userID).Scan(&balance.AvailableAtoms, &balance.LockedAtoms)
	if errors.Is(err, pgx.ErrNoRows) {
		return ledger.Balance{}, nil
	}
	if err != nil {
		return ledger.Balance{}, fmt.Errorf("read ledger balance: %w", err)
	}
	return balance, nil
}

// Reconcile verifies every persisted transaction has at least two zero-sum entries.
func (r *LedgerRepository) Reconcile(ctx context.Context) error {
	operation, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	var broken int
	if err := r.pool.QueryRow(operation, `
		SELECT count(*) FROM (
			SELECT transaction_id FROM ledger_entries GROUP BY transaction_id
			HAVING count(*) < 2 OR sum(amount_atoms) <> 0
		) invalid
	`).Scan(&broken); err != nil {
		return fmt.Errorf("reconcile ledger: %w", err)
	}
	if broken != 0 {
		return ledger.ErrUnbalanced
	}
	return nil
}

func existingFaucet(ctx context.Context, tx pgx.Tx, userID, amountAtoms int64, key string) (ledger.FaucetResult, bool, error) {
	var storedUser, storedAmount int64
	var transactionID string
	var balance ledger.Balance
	err := tx.QueryRow(ctx, `
		SELECT f.user_id, f.amount_atoms, f.transaction_id, b.available_atoms, b.locked_atoms
		FROM faucet_requests f JOIN balances b ON b.user_id = f.user_id AND b.asset = f.asset
		WHERE f.idempotency_key = $1
	`, key).Scan(&storedUser, &storedAmount, &transactionID, &balance.AvailableAtoms, &balance.LockedAtoms)
	if errors.Is(err, pgx.ErrNoRows) {
		return ledger.FaucetResult{}, false, nil
	}
	if err != nil {
		return ledger.FaucetResult{}, false, fmt.Errorf("faucet ledger: find request: %w", err)
	}
	if storedUser != userID || storedAmount != amountAtoms {
		return ledger.FaucetResult{}, false, ledger.ErrConflict
	}
	return ledger.FaucetResult{TransactionID: transactionID, Balance: balance, Replayed: true}, true, nil
}

func randomLedgerID() (string, error) {
	payload := make([]byte, 16)
	if _, err := rand.Read(payload); err != nil {
		return "", fmt.Errorf("create ledger transaction id: entropy unavailable")
	}
	return "ltx_" + hex.EncodeToString(payload), nil
}

func formatUSDT(atoms int64) string {
	whole := atoms / 100_000_000
	fraction := atoms % 100_000_000
	return fmt.Sprintf("%d.%08d", whole, fraction)
}
