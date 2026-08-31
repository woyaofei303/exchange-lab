package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/httpserver"
	"github.com/woyaofei303/exchange-lab/internal/ledger"
	"github.com/woyaofei303/exchange-lab/internal/market"
)

type fakeLedger struct {
	amount int64
	key    string
}

func (f *fakeLedger) Faucet(_ context.Context, _ int64, amount int64, key string) (ledger.FaucetResult, error) {
	f.amount, f.key = amount, key
	return ledger.FaucetResult{TransactionID: "ltx_test", Balance: ledger.Balance{AvailableAtoms: amount}}, nil
}

func (f *fakeLedger) Balance(context.Context, int64) (ledger.Balance, error) {
	return ledger.Balance{AvailableAtoms: 12_345_000_000, LockedAtoms: 500_000_000}, nil
}

func ledgerHandler(book *fakeLedger) http.Handler {
	service := &fakeIdentity{}
	return httpserver.NewHandlerWithFeatures(readiness{}, market.NewStaticReader(nil, nil), httpserver.Features{
		Identity: service, LoginLimiter: allowLogin{}, AuthSessionTTL: time.Hour, Ledger: book,
	}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), time.Second)
}

func testSessionCookie() *http.Cookie {
	return &http.Cookie{Name: "exchange_session", Value: strings.Repeat("t", 43)}
}

func TestAuthenticatedFaucetUsesExactDecimalAndIdempotencyKey(t *testing.T) {
	book := &fakeLedger{}
	request := httptest.NewRequest(http.MethodPost, "/v1/faucet", strings.NewReader(`{"amount":"50.25000000"}`))
	request.Header.Set("Idempotency-Key", "faucet-http-test-0001")
	request.AddCookie(testSessionCookie())
	response := httptest.NewRecorder()
	ledgerHandler(book).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("faucet = %d %s", response.Code, response.Body.String())
	}
	if book.amount != 5_025_000_000 || book.key != "faucet-http-test-0001" {
		t.Fatalf("ledger call = amount %d, key %q", book.amount, book.key)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(response.Body.String(), "5025000000") || !strings.Contains(response.Body.String(), `"available":"50.25000000"`) {
		t.Fatalf("faucet body = %s", response.Body.String())
	}
}

func TestBalancesRequiresSessionAndReturnsDecimalStrings(t *testing.T) {
	handler := ledgerHandler(&fakeLedger{})
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/v1/balances", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated balance = %d", unauthenticated.Code)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/balances", nil)
	request.AddCookie(testSessionCookie())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"available":"123.45000000"`) || !strings.Contains(response.Body.String(), `"locked":"5.00000000"`) {
		t.Fatalf("balance = %d %s", response.Code, response.Body.String())
	}
}
