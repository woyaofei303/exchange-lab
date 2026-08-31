// Package identity owns users, opaque sessions, RBAC and security audit facts.
package identity

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Identity errors are deliberately coarse so transport layers do not leak credentials.
var (
	ErrConflict           = errors.New("identity conflict")
	ErrNotFound           = errors.New("identity not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthenticated    = errors.New("unauthenticated")
	ErrForbidden          = errors.New("forbidden")
	ErrInvalidInput       = errors.New("invalid identity input")
)

var emailPattern = regexp.MustCompile(`^[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

// Status is the persisted user-account lifecycle state.
type Status string

// StatusActive is the only state allowed to create or use sessions.
const StatusActive Status = "active"

// User is the persisted identity record; PasswordHash must be cleared at public boundaries.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Role         Role
	Status       Status
}

// Session is an opaque-token session; only TokenHash is persisted.
type Session struct {
	TokenHash TokenHash
	User      User
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt time.Time
}

// AuditEvent is an append-only identity or authorization fact.
type AuditEvent struct {
	ID           int64          `json:"id"`
	ActorUserID  *int64         `json:"actor_user_id,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Metadata     map[string]any `json:"metadata"`
	OccurredAt   time.Time      `json:"occurred_at"`
}

// Store owns atomic user, session and audit persistence operations.
type Store interface {
	CreateUser(context.Context, string, string) (User, error)
	UserByEmail(context.Context, string) (User, error)
	CreateSession(context.Context, Session) error
	Session(context.Context, TokenHash) (Session, error)
	RotateSession(context.Context, TokenHash, Session) error
	RevokeSession(context.Context, TokenHash, time.Time) error
	AppendAudit(context.Context, AuditEvent) error
	ListAudit(context.Context, int) ([]AuditEvent, error)
}

// Service enforces password, session expiry and RBAC rules over Store.
type Service struct {
	store  Store
	hasher PasswordHasher
	issuer TokenIssuer
	ttl    time.Duration
	now    func() time.Time
}

// NewService validates security dependencies and bounds session lifetime.
func NewService(store Store, hasher PasswordHasher, issuer TokenIssuer, ttl time.Duration, now func() time.Time) (*Service, error) {
	if store == nil || !hasher.valid() || ttl < time.Minute || ttl > 30*24*time.Hour || now == nil {
		return nil, fmt.Errorf("create identity service: invalid configuration")
	}
	return &Service{store: store, hasher: hasher, issuer: issuer, ttl: ttl, now: now}, nil
}

// Register normalizes an email, hashes the password and returns a sanitized user.
func (s *Service) Register(ctx context.Context, email, password string) (User, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return User{}, err
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return User{}, ErrInvalidInput
	}
	user, err := s.store.CreateUser(ctx, email, hash)
	if err != nil {
		return User{}, err
	}
	_ = s.audit(ctx, &user.ID, "identity.registered", "user", fmt.Sprint(user.ID), nil)
	user.PasswordHash = ""
	return user, nil
}

// Login verifies credentials and returns the only copy of a new opaque token.
func (s *Service) Login(ctx context.Context, email, password string) (string, Session, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return "", Session{}, ErrInvalidCredentials
	}
	user, err := s.store.UserByEmail(ctx, email)
	if err != nil || user.Status != StatusActive || !s.hasher.Verify(user.PasswordHash, password) {
		_ = s.audit(ctx, nil, "identity.login_denied", "user", "unknown", map[string]any{"reason": "invalid_credentials"})
		return "", Session{}, ErrInvalidCredentials
	}
	token, hash, err := s.issuer.Issue()
	if err != nil {
		return "", Session{}, err
	}
	now := s.now().UTC()
	session := Session{TokenHash: hash, User: publicUser(user), CreatedAt: now, ExpiresAt: now.Add(s.ttl)}
	if err := s.store.CreateSession(ctx, session); err != nil {
		return "", Session{}, err
	}
	_ = s.audit(ctx, &user.ID, "identity.login_succeeded", "session", "self", nil)
	return token, session, nil
}

// Authenticate resolves an active unexpired token without exposing its stored hash.
func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	if len(token) < 40 || len(token) > 128 {
		return Session{}, ErrUnauthenticated
	}
	session, err := s.store.Session(ctx, HashToken(token))
	if err != nil || !session.RevokedAt.IsZero() || !s.now().UTC().Before(session.ExpiresAt) || session.User.Status != StatusActive {
		return Session{}, ErrUnauthenticated
	}
	session.TokenHash = TokenHash{}
	session.User.PasswordHash = ""
	return session, nil
}

// Refresh atomically revokes a session and replaces it with fresh opaque credentials.
func (s *Service) Refresh(ctx context.Context, token string) (string, Session, error) {
	current, err := s.Authenticate(ctx, token)
	if err != nil {
		return "", Session{}, err
	}
	replacementToken, replacementHash, err := s.issuer.Issue()
	if err != nil {
		return "", Session{}, err
	}
	now := s.now().UTC()
	replacement := Session{TokenHash: replacementHash, User: current.User, CreatedAt: now, ExpiresAt: now.Add(s.ttl)}
	if err := s.store.RotateSession(ctx, HashToken(token), replacement); err != nil {
		return "", Session{}, ErrUnauthenticated
	}
	_ = s.audit(ctx, &current.User.ID, "identity.session_refreshed", "session", "self", nil)
	return replacementToken, replacement, nil
}

// Logout is idempotent and revokes an active token when present.
func (s *Service) Logout(ctx context.Context, token string) error {
	session, err := s.Authenticate(ctx, token)
	if err != nil {
		return nil
	}
	if err := s.store.RevokeSession(ctx, HashToken(token), s.now().UTC()); err != nil {
		return err
	}
	return s.audit(ctx, &session.User.ID, "identity.logout", "session", "self", nil)
}

// AuditLog returns a bounded audit history to administrators only.
func (s *Service) AuditLog(ctx context.Context, actor User, limit int) ([]AuditEvent, error) {
	if !actor.Role.Allows(RoleAdmin) {
		_ = s.audit(ctx, &actor.ID, "authorization.denied", "audit_log", "list", map[string]any{"required_role": string(RoleAdmin)})
		return nil, ErrForbidden
	}
	if limit < 1 || limit > 200 {
		return nil, ErrInvalidInput
	}
	return s.store.ListAudit(ctx, limit)
}

func (s *Service) audit(ctx context.Context, actorID *int64, action, resourceType, resourceID string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return s.store.AppendAudit(ctx, AuditEvent{ActorUserID: actorID, Action: action, ResourceType: resourceType, ResourceID: resourceID, Metadata: metadata, OccurredAt: s.now().UTC()})
}

func normalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) < 3 || len(value) > 320 || !emailPattern.MatchString(value) {
		return "", ErrInvalidInput
	}
	return value, nil
}

func publicUser(user User) User {
	user.PasswordHash = ""
	return user
}
