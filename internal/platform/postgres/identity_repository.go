package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/woyaofei303/exchange-lab/internal/identity"
)

// IdentityRepository persists users, hashed sessions and append-only audit facts.
type IdentityRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewIdentityRepository constructs a context-bounded identity repository.
func NewIdentityRepository(pool *pgxpool.Pool, timeout time.Duration) *IdentityRepository {
	return &IdentityRepository{pool: pool, timeout: timeout}
}

// CreateUser inserts a normalized ordinary user and maps unique conflicts.
func (r *IdentityRepository) CreateUser(ctx context.Context, email, passwordHash string) (identity.User, error) {
	operation, cancel := r.operation(ctx)
	defer cancel()
	var user identity.User
	err := r.pool.QueryRow(operation, `
		INSERT INTO users (email, password_hash, role)
		VALUES ($1, $2, 'user')
		RETURNING id, email, password_hash, role, status
	`, email, passwordHash).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.Status)
	if uniqueViolation(err) {
		return identity.User{}, identity.ErrConflict
	}
	if err != nil {
		return identity.User{}, fmt.Errorf("create identity user: %w", err)
	}
	return user, nil
}

// UserByEmail resolves one identity case-insensitively.
func (r *IdentityRepository) UserByEmail(ctx context.Context, email string) (identity.User, error) {
	operation, cancel := r.operation(ctx)
	defer cancel()
	var user identity.User
	err := r.pool.QueryRow(operation, `
		SELECT id, email, password_hash, role, status
		FROM users WHERE lower(email) = lower($1)
	`, email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.User{}, identity.ErrNotFound
	}
	if err != nil {
		return identity.User{}, fmt.Errorf("find identity user: %w", err)
	}
	return user, nil
}

// CreateSession persists only the token hash and expiry metadata.
func (r *IdentityRepository) CreateSession(ctx context.Context, session identity.Session) error {
	operation, cancel := r.operation(ctx)
	defer cancel()
	_, err := r.pool.Exec(operation, `
		INSERT INTO user_sessions (token_hash, user_id, created_at, expires_at)
		VALUES ($1, $2, $3, $4)
	`, session.TokenHash[:], session.User.ID, session.CreatedAt, session.ExpiresAt)
	if uniqueViolation(err) {
		return identity.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("create identity session: %w", err)
	}
	return nil
}

// Session resolves one non-revoked token hash with its user identity.
func (r *IdentityRepository) Session(ctx context.Context, hash identity.TokenHash) (identity.Session, error) {
	operation, cancel := r.operation(ctx)
	defer cancel()
	var session identity.Session
	var storedHash []byte
	err := r.pool.QueryRow(operation, `
		SELECT s.token_hash, s.created_at, s.expires_at, COALESCE(s.revoked_at, '0001-01-01'::timestamptz),
		       u.id, u.email, u.role, u.status
		FROM user_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL
	`, hash[:]).Scan(&storedHash, &session.CreatedAt, &session.ExpiresAt, &session.RevokedAt,
		&session.User.ID, &session.User.Email, &session.User.Role, &session.User.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Session{}, identity.ErrNotFound
	}
	if err != nil {
		return identity.Session{}, fmt.Errorf("find identity session: %w", err)
	}
	if len(storedHash) != len(session.TokenHash) {
		return identity.Session{}, fmt.Errorf("find identity session: invalid token hash")
	}
	copy(session.TokenHash[:], storedHash)
	return session, nil
}

// RotateSession revokes and replaces a live session in one Serializable transaction.
func (r *IdentityRepository) RotateSession(ctx context.Context, old identity.TokenHash, replacement identity.Session) error {
	operation, cancel := r.operation(ctx)
	defer cancel()
	tx, err := r.pool.BeginTx(operation, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("rotate identity session: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	result, err := tx.Exec(operation, `
		UPDATE user_sessions SET revoked_at = $2
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > $2
	`, old[:], replacement.CreatedAt)
	if err != nil {
		return fmt.Errorf("rotate identity session: revoke: %w", err)
	}
	if result.RowsAffected() != 1 {
		return identity.ErrNotFound
	}
	_, err = tx.Exec(operation, `
		INSERT INTO user_sessions (token_hash, user_id, created_at, expires_at)
		VALUES ($1, $2, $3, $4)
	`, replacement.TokenHash[:], replacement.User.ID, replacement.CreatedAt, replacement.ExpiresAt)
	if uniqueViolation(err) {
		return identity.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("rotate identity session: insert: %w", err)
	}
	if err := tx.Commit(operation); err != nil {
		return fmt.Errorf("rotate identity session: commit: %w", err)
	}
	return nil
}

// RevokeSession idempotently timestamps an active session.
func (r *IdentityRepository) RevokeSession(ctx context.Context, hash identity.TokenHash, revokedAt time.Time) error {
	operation, cancel := r.operation(ctx)
	defer cancel()
	_, err := r.pool.Exec(operation, `
		UPDATE user_sessions SET revoked_at = $2
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, hash[:], revokedAt)
	if err != nil {
		return fmt.Errorf("revoke identity session: %w", err)
	}
	return nil
}

// AppendAudit stores one structured identity or authorization fact.
func (r *IdentityRepository) AppendAudit(ctx context.Context, event identity.AuditEvent) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("append identity audit: invalid metadata")
	}
	operation, cancel := r.operation(ctx)
	defer cancel()
	_, err = r.pool.Exec(operation, `
		INSERT INTO audit_logs (actor_user_id, action, resource_type, resource_id, metadata, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, event.ActorUserID, event.Action, event.ResourceType, event.ResourceID, metadata, event.OccurredAt)
	if err != nil {
		return fmt.Errorf("append identity audit: %w", err)
	}
	return nil
}

// ListAudit returns the most recent audit facts in deterministic order.
func (r *IdentityRepository) ListAudit(ctx context.Context, limit int) ([]identity.AuditEvent, error) {
	operation, cancel := r.operation(ctx)
	defer cancel()
	rows, err := r.pool.Query(operation, `
		SELECT id, actor_user_id, action, resource_type, resource_id, metadata, occurred_at
		FROM audit_logs ORDER BY occurred_at DESC, id DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list identity audit: %w", err)
	}
	defer rows.Close()
	result := make([]identity.AuditEvent, 0, limit)
	for rows.Next() {
		var event identity.AuditEvent
		var metadata []byte
		if err := rows.Scan(&event.ID, &event.ActorUserID, &event.Action, &event.ResourceType, &event.ResourceID, &metadata, &event.OccurredAt); err != nil {
			return nil, fmt.Errorf("list identity audit: scan: %w", err)
		}
		if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
			return nil, fmt.Errorf("list identity audit: invalid metadata")
		}
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list identity audit: rows: %w", err)
	}
	return result, nil
}

func (r *IdentityRepository) operation(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.timeout)
}

func uniqueViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
