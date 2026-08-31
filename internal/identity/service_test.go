package identity

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceRegistrationLoginRotationLogoutAndRBAC(t *testing.T) {
	store := newMemoryStore()
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	tokenEntropy := append(bytes.Repeat([]byte{11}, 32), bytes.Repeat([]byte{12}, 32)...)
	service, err := NewService(store, testHasher(), TokenIssuer{Random: bytes.NewReader(tokenEntropy)}, time.Hour, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	user, err := service.Register(context.Background(), " Trader@Example.com ", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "trader@example.com" || user.Role != RoleUser {
		t.Fatalf("Register() = %#v", user)
	}
	if _, err := service.Register(context.Background(), "trader@example.com", "correct horse battery staple"); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate Register() error = %v", err)
	}
	token, session, err := service.Login(context.Background(), "trader@example.com", "correct horse battery staple")
	if err != nil || token == "" || session.User.ID != user.ID {
		t.Fatalf("Login() = %q %#v %v", token, session, err)
	}
	if _, _, err := service.Login(context.Background(), "trader@example.com", "wrong password here"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong Login() error = %v", err)
	}
	current, err := service.Authenticate(context.Background(), token)
	if err != nil || current.User.ID != user.ID {
		t.Fatalf("Authenticate() = %#v %v", current, err)
	}
	rotated, _, err := service.Refresh(context.Background(), token)
	if err != nil || rotated == token {
		t.Fatalf("Refresh() = %q %v", rotated, err)
	}
	if _, err := service.Authenticate(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("old token Authenticate() error = %v", err)
	}
	if err := service.Logout(context.Background(), rotated); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), rotated); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("logged-out Authenticate() error = %v", err)
	}
	if _, err := service.AuditLog(context.Background(), user, 20); !errors.Is(err, ErrForbidden) {
		t.Fatalf("user AuditLog() error = %v", err)
	}
	if len(store.audits) < 5 {
		t.Fatalf("audit count = %d", len(store.audits))
	}
}

type memoryStore struct {
	nextID   int64
	users    map[string]User
	sessions map[TokenHash]Session
	audits   []AuditEvent
}

func newMemoryStore() *memoryStore {
	return &memoryStore{nextID: 1, users: map[string]User{}, sessions: map[TokenHash]Session{}}
}

func (s *memoryStore) CreateUser(_ context.Context, email, passwordHash string) (User, error) {
	if _, ok := s.users[email]; ok {
		return User{}, ErrConflict
	}
	user := User{ID: s.nextID, Email: email, PasswordHash: passwordHash, Role: RoleUser, Status: StatusActive}
	s.nextID++
	s.users[email] = user
	return user, nil
}

func (s *memoryStore) UserByEmail(_ context.Context, email string) (User, error) {
	user, ok := s.users[email]
	if !ok {
		return User{}, ErrNotFound
	}
	return user, nil
}

func (s *memoryStore) CreateSession(_ context.Context, session Session) error {
	if _, ok := s.sessions[session.TokenHash]; ok {
		return ErrConflict
	}
	s.sessions[session.TokenHash] = session
	return nil
}

func (s *memoryStore) Session(_ context.Context, hash TokenHash) (Session, error) {
	session, ok := s.sessions[hash]
	if !ok || !session.RevokedAt.IsZero() {
		return Session{}, ErrNotFound
	}
	return session, nil
}

func (s *memoryStore) RotateSession(_ context.Context, old TokenHash, replacement Session) error {
	current, ok := s.sessions[old]
	if !ok || !current.RevokedAt.IsZero() {
		return ErrNotFound
	}
	current.RevokedAt = replacement.CreatedAt
	s.sessions[old] = current
	s.sessions[replacement.TokenHash] = replacement
	return nil
}

func (s *memoryStore) RevokeSession(_ context.Context, hash TokenHash, revokedAt time.Time) error {
	session, ok := s.sessions[hash]
	if !ok {
		return nil
	}
	session.RevokedAt = revokedAt
	s.sessions[hash] = session
	return nil
}

func (s *memoryStore) AppendAudit(_ context.Context, event AuditEvent) error {
	s.audits = append(s.audits, event)
	return nil
}

func (s *memoryStore) ListAudit(_ context.Context, limit int) ([]AuditEvent, error) {
	if limit > len(s.audits) {
		limit = len(s.audits)
	}
	return append([]AuditEvent(nil), s.audits[len(s.audits)-limit:]...), nil
}
