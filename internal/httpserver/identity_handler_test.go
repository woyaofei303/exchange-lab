package httpserver_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"exchange-lab/internal/httpserver"
	"exchange-lab/internal/identity"
	"exchange-lab/internal/market"
	"exchange-lab/internal/platform/rediscache"
)

type fakeIdentity struct {
	loginCalls int
}

func (*fakeIdentity) Register(_ context.Context, email, _ string) (identity.User, error) {
	return identity.User{ID: 7, Email: email, Role: identity.RoleUser}, nil
}
func (f *fakeIdentity) Login(_ context.Context, email, _ string) (string, identity.Session, error) {
	f.loginCalls++
	return strings.Repeat("t", 43), identity.Session{User: identity.User{ID: 7, Email: email, Role: identity.RoleUser}, ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (*fakeIdentity) Authenticate(_ context.Context, token string) (identity.Session, error) {
	if len(token) < 40 {
		return identity.Session{}, identity.ErrUnauthenticated
	}
	return identity.Session{User: identity.User{ID: 7, Email: "trader@example.test", Role: identity.RoleUser}}, nil
}
func (*fakeIdentity) Refresh(context.Context, string) (string, identity.Session, error) {
	return "", identity.Session{}, identity.ErrUnauthenticated
}
func (*fakeIdentity) Logout(context.Context, string) error { return nil }
func (*fakeIdentity) AuditLog(context.Context, identity.User, int) ([]identity.AuditEvent, error) {
	return nil, identity.ErrForbidden
}

type allowLogin struct{}

func (allowLogin) Allow(context.Context, string) (rediscache.RateDecision, error) {
	return rediscache.RateDecision{Allowed: true}, nil
}

func identityHandler(service *fakeIdentity, secure bool) http.Handler {
	return httpserver.NewHandlerWithFeatures(readiness{}, market.NewStaticReader(nil, nil), httpserver.Features{
		Identity: service, LoginLimiter: allowLogin{}, AuthCookieSecure: secure, AuthSessionTTL: time.Hour,
	}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), time.Second)
}

func TestLoginSetsOpaqueHardenedSessionCookie(t *testing.T) {
	handler := identityHandler(&fakeIdentity{}, true)
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"email":"trader@example.test","password":"correct horse battery staple"}`))
	request.Host = "exchange.test"
	request.Header.Set("Origin", "https://exchange.test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("login = %d %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "exchange_session" || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode || len(cookies[0].Value) < 40 {
		t.Fatalf("session cookies = %#v", cookies)
	}
	if strings.Contains(response.Body.String(), cookies[0].Value) {
		t.Fatal("response body leaked session token")
	}
}

func TestCrossSiteLoginIsRejectedBeforeCredentials(t *testing.T) {
	service := &fakeIdentity{}
	handler := identityHandler(service, true)
	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"email":"trader@example.test","password":"correct horse battery staple"}`))
	request.Host = "exchange.test"
	request.Header.Set("Origin", "https://attacker.test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || service.loginCalls != 0 {
		t.Fatalf("cross-site login = %d, calls=%d", response.Code, service.loginCalls)
	}
}

func TestUserCannotReadAdminAuditLog(t *testing.T) {
	handler := identityHandler(&fakeIdentity{}, false)
	request := httptest.NewRequest(http.MethodGet, "/v1/admin/audit-logs", nil)
	request.AddCookie(&http.Cookie{Name: "exchange_session", Value: strings.Repeat("t", 43)})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), "trader@example.test") {
		t.Fatalf("audit response = %d %s", response.Code, response.Body.String())
	}
}
