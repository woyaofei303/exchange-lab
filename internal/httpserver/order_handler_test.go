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
	"exchange-lab/internal/market"
	domainorder "exchange-lab/internal/order"
	"exchange-lab/internal/trading"
)

type fakeOrders struct {
	command domainorder.PlaceCommand
	userID  int64
}

func (f *fakeOrders) Place(_ context.Context, userID int64, command domainorder.PlaceCommand) (domainorder.Result, error) {
	f.userID, f.command = userID, command
	return domainorder.Result{Order: sampleOrder()}, nil
}

func (*fakeOrders) Cancel(context.Context, int64, string, string) (domainorder.Result, error) {
	item := sampleOrder()
	item.Status, item.ReservedMarginAtoms = trading.OrderStatusCanceled, 0
	return domainorder.Result{Order: item}, nil
}

func (*fakeOrders) List(context.Context, int64, int) ([]domainorder.Order, error) {
	return []domainorder.Order{sampleOrder()}, nil
}

func sampleOrder() domainorder.Order {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	return domainorder.Order{ID: "ord_test", UserID: 7, ClientOrderID: "browser-order-0001", Symbol: "BTCUSDT-PERP", Side: trading.SideBuy, Type: trading.OrderTypeLimit, Status: trading.OrderStatusOpen, PriceAtoms: 5_000_000, PriceScale: 2, QuantityAtoms: 10, RemainingAtoms: 10, QuantityScale: 3, QuoteScale: 8, Leverage: 10, ReservedMarginAtoms: 5_000_000_000, CreatedAt: now, UpdatedAt: now}
}

func orderHandler(service *fakeOrders) http.Handler {
	return httpserver.NewHandlerWithFeatures(readiness{}, market.NewStaticReader(nil, nil), httpserver.Features{
		Identity: &fakeIdentity{}, LoginLimiter: allowLogin{}, AuthSessionTTL: time.Hour, Orders: service,
	}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), time.Second)
}

func TestOrderAPIAuthenticatesAndPreservesDecimalStrings(t *testing.T) {
	service := &fakeOrders{}
	request := httptest.NewRequest(http.MethodPost, "/v1/orders", strings.NewReader(`{"symbol":"BTCUSDT-PERP","side":"buy","type":"limit","price":"50000.00","quantity":"0.010","leverage":10,"client_order_id":"browser-order-0001"}`))
	request.Header.Set("Idempotency-Key", "browser-place-0001")
	request.AddCookie(testSessionCookie())
	response := httptest.NewRecorder()
	orderHandler(service).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("place order = %d %s", response.Code, response.Body.String())
	}
	if service.userID != 7 || service.command.Price != "50000.00" || service.command.IdempotencyKey != "browser-place-0001" {
		t.Fatalf("command = %#v, user=%d", service.command, service.userID)
	}
	if !strings.Contains(response.Body.String(), `"price":"50000.00"`) || !strings.Contains(response.Body.String(), `"reserved_margin":"50.00000000"`) || strings.Contains(response.Body.String(), "5000000000") {
		t.Fatalf("order body = %s", response.Body.String())
	}
}

func TestOrderListRejectsUnauthenticatedRequest(t *testing.T) {
	response := httptest.NewRecorder()
	orderHandler(&fakeOrders{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/orders", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("list orders = %d %s", response.Code, response.Body.String())
	}
}
