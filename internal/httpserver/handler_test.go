package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/health"
	"github.com/woyaofei303/exchange-lab/internal/httpserver"
	"github.com/woyaofei303/exchange-lab/internal/market"
	"github.com/woyaofei303/exchange-lab/internal/trading"
)

type readiness struct {
	report health.Report
	calls  *int
}

type marketReader struct {
	instruments func(context.Context, string) ([]trading.Instrument, error)
	tickers     func(context.Context, string) ([]market.Ticker, error)
}

func (r marketReader) Instruments(ctx context.Context, symbol string) ([]trading.Instrument, error) {
	return r.instruments(ctx, symbol)
}

func (r marketReader) Tickers(ctx context.Context, symbol string) ([]market.Ticker, error) {
	return r.tickers(ctx, symbol)
}

func (r readiness) Check(context.Context) health.Report {
	if r.calls != nil {
		*r.calls = *r.calls + 1
	}
	return r.report
}

func request(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

func newHandler(readiness httpserver.Readiness, logger *slog.Logger) http.Handler {
	return httpserver.NewHandler(readiness, market.NewStaticReader(nil, nil), logger, time.Second)
}

func TestHealthzReturnsProcessHealth(t *testing.T) {
	handler := newHandler(readiness{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	response := request(t, handler, http.MethodGet, "/healthz")
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("health response = %d %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body["status"] != "ok" {
		t.Fatalf("health body = %#v, err = %v", body, err)
	}
}

func TestHandlerAddsProductionSecurityHeaders(t *testing.T) {
	handler := newHandler(readiness{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	response := request(t, handler, http.MethodGet, "/healthz")
	wants := map[string]string{
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'; base-uri 'none'",
		"Referrer-Policy":         "no-referrer",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Permissions-Policy":      "camera=(), microphone=(), geolocation=()",
	}
	for name, want := range wants {
		if got := response.Header().Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestHealthzNeverCallsReadiness(t *testing.T) {
	calls := 0
	handler := newHandler(readiness{calls: &calls}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	response := request(t, handler, http.MethodGet, "/healthz")
	if response.Code != http.StatusOK {
		t.Fatalf("health response = %d %s", response.Code, response.Body.String())
	}
	if calls != 0 {
		t.Fatalf("readiness Check() calls = %d, want 0", calls)
	}
}

func TestReadyzReturnsServiceUnavailableWithoutLeakingError(t *testing.T) {
	secret := "postgres://exchange:super-secret@postgres:5432/exchange"
	report := health.Report{
		Ready:  false,
		Checks: map[string]string{"postgres": health.StatusError, "redis": health.StatusOK, "nats": health.StatusOK},
		Errors: map[string]error{"postgres": errors.New(secret)},
	}
	handler := newHandler(readiness{report: report}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	response := request(t, handler, http.MethodGet, "/readyz")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready response = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode ready body: %v", err)
	}
	if body.Status != "not_ready" {
		t.Fatalf("ready status = %q, want not_ready", body.Status)
	}
	for name, want := range report.Checks {
		if got := body.Checks[name]; got != want {
			t.Fatalf("ready check %s = %q, want %q", name, got, want)
		}
	}
	if strings.Contains(response.Body.String(), "super-secret") || strings.Contains(response.Body.String(), "postgres://") {
		t.Fatalf("ready response leaked secret: %s", response.Body.String())
	}
}

func TestReadyzReturnsReadyWithAllDependencyStatuses(t *testing.T) {
	report := health.Report{
		Ready:  true,
		Checks: map[string]string{"postgres": health.StatusOK, "redis": health.StatusOK, "nats": health.StatusOK},
	}
	handler := newHandler(readiness{report: report}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	response := request(t, handler, http.MethodGet, "/readyz")
	if response.Code != http.StatusOK {
		t.Fatalf("ready response = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode ready body: %v", err)
	}
	if body.Status != "ready" {
		t.Fatalf("ready status = %q, want ready", body.Status)
	}
	for name, want := range report.Checks {
		if got := body.Checks[name]; got != want {
			t.Fatalf("ready check %s = %q, want %q", name, got, want)
		}
	}
}

func TestHealthEndpointsRejectNonGETMethods(t *testing.T) {
	handler := newHandler(readiness{}, slog.Default())
	for _, path := range []string{"/healthz", "/readyz"} {
		response := request(t, handler, http.MethodPost, path)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s status = %d", path, response.Code)
		}
		if allow := response.Header().Get("Allow"); allow != http.MethodGet {
			t.Fatalf("POST %s Allow = %q, want %q", path, allow, http.MethodGet)
		}
	}
}

type failingWriter struct{ header http.Header }

func (w *failingWriter) Header() http.Header     { return w.header }
func (*failingWriter) WriteHeader(int)           {}
func (*failingWriter) Write([]byte) (int, error) { return 0, errors.New("writer-secret") }

func TestHandlerLogsSanitizedResponseWriteFailure(t *testing.T) {
	var logs bytes.Buffer
	handler := newHandler(readiness{}, slog.New(slog.NewTextHandler(&logs, nil)))
	handler.ServeHTTP(&failingWriter{header: make(http.Header)}, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if !strings.Contains(logs.String(), "write http response failed") {
		t.Fatalf("log did not record response failure: %s", logs.String())
	}
	if strings.Contains(logs.String(), "writer-secret") {
		t.Fatalf("log leaked writer detail: %s", logs.String())
	}
}

func TestInstrumentsReturnsTradingFilters(t *testing.T) {
	base, err := trading.NewAsset("BTC", 8)
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	quote, err := trading.NewAsset("USDT", 8)
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	instrument, err := trading.NewInstrument(trading.InstrumentSpec{
		Base:          base,
		Quote:         quote,
		PriceScale:    2,
		QuantityScale: 3,
		PriceStep:     "0.10",
		QuantityStep:  "0.001",
		MinNotional:   "5",
	})
	if err != nil {
		t.Fatalf("NewInstrument() error = %v", err)
	}
	handler := httpserver.NewHandler(
		readiness{},
		market.NewStaticReader([]trading.Instrument{instrument}, nil),
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		time.Second,
	)

	response := request(t, handler, http.MethodGet, "/v1/instruments")
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Data []struct {
			Symbol        string `json:"symbol"`
			BaseAsset     string `json:"base_asset"`
			QuoteAsset    string `json:"quote_asset"`
			PriceScale    uint8  `json:"price_scale"`
			QuantityScale uint8  `json:"quantity_scale"`
			PriceStep     string `json:"price_step"`
			QuantityStep  string `json:"quantity_step"`
			MinNotional   string `json:"min_notional"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, want := len(body.Data), 1; got != want {
		t.Fatalf("len(data) = %d, want %d", got, want)
	}
	got := body.Data[0]
	if got.Symbol != "BTCUSDT-PERP" || got.BaseAsset != "BTC" || got.QuoteAsset != "USDT" {
		t.Fatalf("instrument identity = %#v", got)
	}
	if got.PriceScale != 2 || got.QuantityScale != 3 || got.PriceStep != "0.10" || got.QuantityStep != "0.001" || got.MinNotional != "5.00000000" {
		t.Fatalf("instrument filters = %#v", got)
	}
}

func TestTickersReturnsStringEncodedMarketData(t *testing.T) {
	updatedAt := time.Date(2026, 8, 24, 12, 30, 0, 123000000, time.UTC)
	markets := market.NewStaticReader(nil, []market.Ticker{{
		Symbol:      "BTCUSDT-PERP",
		LastPrice:   "50000.10",
		MarkPrice:   "50001.20",
		IndexPrice:  "49999.90",
		PriceChange: "1.25",
		Volume:      "1234.567",
		UpdatedAt:   updatedAt,
	}})
	handler := httpserver.NewHandler(
		readiness{},
		markets,
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		time.Second,
	)

	response := request(t, handler, http.MethodGet, "/v1/tickers")
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Data []struct {
			Symbol      string `json:"symbol"`
			LastPrice   string `json:"last_price"`
			MarkPrice   string `json:"mark_price"`
			IndexPrice  string `json:"index_price"`
			PriceChange string `json:"price_change_24h"`
			Volume      string `json:"volume_24h"`
			UpdatedAt   string `json:"updated_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, want := len(body.Data), 1; got != want {
		t.Fatalf("len(data) = %d, want %d", got, want)
	}
	got := body.Data[0]
	if got.Symbol != "BTCUSDT-PERP" || got.LastPrice != "50000.10" || got.MarkPrice != "50001.20" || got.IndexPrice != "49999.90" || got.PriceChange != "1.25" || got.Volume != "1234.567" || got.UpdatedAt != updatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("ticker = %#v", got)
	}
}

func TestInstrumentsFiltersByCanonicalSymbol(t *testing.T) {
	markets := market.NewStaticReader([]trading.Instrument{
		mustHTTPInstrument(t, "BTC"),
		mustHTTPInstrument(t, "ETH"),
	}, nil)
	handler := httpserver.NewHandler(
		readiness{},
		markets,
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		time.Second,
	)

	response := request(t, handler, http.MethodGet, "/v1/instruments?symbol=ETHUSDT-PERP")
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Data []struct {
			Symbol string `json:"symbol"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, want := len(body.Data), 1; got != want || body.Data[0].Symbol != "ETHUSDT-PERP" {
		t.Fatalf("data = %#v, want only ETHUSDT-PERP", body.Data)
	}
}

func TestMarketEndpointsRejectInvalidQuery(t *testing.T) {
	handler := newHandler(readiness{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	tests := []string{
		"?symbol=ethusdt-perp",
		"?symbol=BTCUSDT-PERP&symbol=ETHUSDT-PERP",
		"?symbol=",
		"?token=secret",
	}
	for _, endpoint := range []string{"/v1/instruments", "/v1/tickers"} {
		for _, query := range tests {
			response := request(t, handler, http.MethodGet, endpoint+query)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("GET %s%s status = %d, want %d", endpoint, query, response.Code, http.StatusBadRequest)
			}
			if strings.Contains(response.Body.String(), "secret") {
				t.Fatalf("GET %s%s leaked query value: %s", endpoint, query, response.Body.String())
			}
		}
	}
}

func TestHandlerAssignsRequestIDAndIncludesItInStructuredLog(t *testing.T) {
	var logs bytes.Buffer
	handler := newHandler(readiness{}, slog.New(slog.NewTextHandler(&logs, nil)))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	requestID := recorder.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Fatal("X-Request-ID response header is empty")
	}
	if !strings.Contains(logs.String(), "request_id="+requestID) {
		t.Fatalf("logs do not contain request ID %q: %s", requestID, logs.String())
	}
	if !strings.Contains(logs.String(), "method=GET") || !strings.Contains(logs.String(), "path=/healthz") {
		t.Fatalf("logs do not contain request metadata: %s", logs.String())
	}
}

func TestHandlerPropagatesValidClientRequestID(t *testing.T) {
	var logs bytes.Buffer
	handler := newHandler(readiness{}, slog.New(slog.NewTextHandler(&logs, nil)))
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-ID", "client-123")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if got, want := recorder.Header().Get("X-Request-ID"), "client-123"; got != want {
		t.Fatalf("X-Request-ID = %q, want %q", got, want)
	}
	if !strings.Contains(logs.String(), "request_id=client-123") {
		t.Fatalf("logs do not contain propagated request ID: %s", logs.String())
	}
}

func TestHandlerReplacesUnsafeClientRequestIDWithoutLoggingIt(t *testing.T) {
	const unsafeID = "secret token"
	var logs bytes.Buffer
	handler := newHandler(readiness{}, slog.New(slog.NewTextHandler(&logs, nil)))
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-ID", unsafeID)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("X-Request-ID"); got == "" || got == unsafeID {
		t.Fatalf("replacement X-Request-ID = %q", got)
	}
	if strings.Contains(logs.String(), unsafeID) {
		t.Fatalf("logs leaked unsafe request ID: %s", logs.String())
	}
}

func TestMarketErrorLogIncludesRequestIDWithoutLeakingCause(t *testing.T) {
	const secret = "market-store-secret"
	var logs bytes.Buffer
	markets := marketReader{
		instruments: func(context.Context, string) ([]trading.Instrument, error) {
			return nil, errors.New(secret)
		},
		tickers: func(context.Context, string) ([]market.Ticker, error) {
			return nil, nil
		},
	}
	handler := httpserver.NewHandler(readiness{}, markets, slog.New(slog.NewTextHandler(&logs, nil)), time.Second)
	request := httptest.NewRequest(http.MethodGet, "/v1/instruments", nil)
	request.Header.Set("X-Request-ID", "request-error-1")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "internal_error" || body.Error.RequestID != "request-error-1" {
		t.Fatalf("error response = %#v", body.Error)
	}
	var foundErrorLog bool
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "list instruments failed") {
			foundErrorLog = true
			if !strings.Contains(line, "request_id=request-error-1") {
				t.Fatalf("error log lacks request ID: %s", line)
			}
		}
	}
	if !foundErrorLog {
		t.Fatalf("missing market error log: %s", logs.String())
	}
	if strings.Contains(logs.String(), secret) || strings.Contains(recorder.Body.String(), secret) {
		t.Fatalf("market error leaked secret: logs=%s body=%s", logs.String(), recorder.Body.String())
	}
}

func TestInvalidQueryUsesUnifiedErrorEnvelope(t *testing.T) {
	handler := newHandler(readiness{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	request := httptest.NewRequest(http.MethodGet, "/v1/instruments?symbol=invalid", nil)
	request.Header.Set("X-Request-ID", "request-invalid-1")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "invalid_request" || body.Error.Message != "invalid request" || body.Error.RequestID != "request-invalid-1" {
		t.Fatalf("error response = %#v", body.Error)
	}
}

func TestUnknownPathUsesUnifiedNotFoundError(t *testing.T) {
	handler := newHandler(readiness{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	request := httptest.NewRequest(http.MethodGet, "/v1/unknown", nil)
	request.Header.Set("X-Request-ID", "request-not-found-1")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "not_found" || body.Error.RequestID != "request-not-found-1" {
		t.Fatalf("error response = %#v", body.Error)
	}
}

func TestKnownEndpointsUseUnifiedMethodNotAllowedError(t *testing.T) {
	handler := newHandler(readiness{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	for _, path := range []string{"/healthz", "/readyz", "/v1/instruments", "/v1/tickers"} {
		request := httptest.NewRequest(http.MethodPost, path, nil)
		request.Header.Set("X-Request-ID", "request-method-1")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("POST %s response = %d Allow=%q", path, recorder.Code, recorder.Header().Get("Allow"))
		}
		var body struct {
			Error struct {
				Code      string `json:"code"`
				RequestID string `json:"request_id"`
			} `json:"error"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode POST %s response: %v", path, err)
		}
		if body.Error.Code != "method_not_allowed" || body.Error.RequestID != "request-method-1" {
			t.Fatalf("POST %s error response = %#v", path, body.Error)
		}
	}
}

func TestMarketDeadlineUsesGatewayTimeoutError(t *testing.T) {
	markets := marketReader{
		instruments: func(ctx context.Context, _ string) ([]trading.Instrument, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		tickers: func(context.Context, string) ([]market.Ticker, error) { return nil, nil },
	}
	handler := httpserver.NewHandler(readiness{}, markets, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/v1/instruments", nil).WithContext(ctx)
	request.Header.Set("X-Request-ID", "request-timeout-1")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusGatewayTimeout, recorder.Body.String())
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "request_timeout" || body.Error.RequestID != "request-timeout-1" {
		t.Fatalf("error response = %#v", body.Error)
	}
}

func TestCanceledMarketRequestUsesRequestCanceledError(t *testing.T) {
	markets := marketReader{
		instruments: func(ctx context.Context, _ string) ([]trading.Instrument, error) {
			return nil, ctx.Err()
		},
		tickers: func(context.Context, string) ([]market.Ticker, error) { return nil, nil },
	}
	handler := httpserver.NewHandler(readiness{}, markets, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/v1/instruments", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestTimeout {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusRequestTimeout, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"request_canceled"`) {
		t.Fatalf("response = %s, want request_canceled", recorder.Body.String())
	}
}

func TestHandlerAppliesConfiguredRequestTimeout(t *testing.T) {
	markets := marketReader{
		instruments: func(ctx context.Context, _ string) ([]trading.Instrument, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return nil, errors.New("reader ignored expected timeout")
			}
		},
		tickers: func(context.Context, string) ([]market.Ticker, error) { return nil, nil },
	}
	handler := httpserver.NewHandler(
		readiness{},
		markets,
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		10*time.Millisecond,
	)
	startedAt := time.Now()
	response := request(t, handler, http.MethodGet, "/v1/instruments")

	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusGatewayTimeout, response.Body.String())
	}
	if elapsed := time.Since(startedAt); elapsed >= 100*time.Millisecond {
		t.Fatalf("request elapsed = %s, configured timeout was not applied", elapsed)
	}
}

func TestStructuredRequestLogOmitsQueryAndAuthorizationSecrets(t *testing.T) {
	const secret = "request-log-secret"
	var logs bytes.Buffer
	handler := newHandler(readiness{}, slog.New(slog.NewTextHandler(&logs, nil)))
	request := httptest.NewRequest(http.MethodGet, "/v1/instruments?token="+secret, nil)
	request.Header.Set("Authorization", "Bearer "+secret)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if strings.Contains(logs.String(), secret) || strings.Contains(logs.String(), "Authorization") {
		t.Fatalf("request log leaked secret: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "path=/v1/instruments") {
		t.Fatalf("request log lacks sanitized path: %s", logs.String())
	}
}

func mustHTTPInstrument(t *testing.T, baseCode string) trading.Instrument {
	t.Helper()

	base, err := trading.NewAsset(baseCode, 8)
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	quote, err := trading.NewAsset("USDT", 8)
	if err != nil {
		t.Fatalf("NewAsset() error = %v", err)
	}
	instrument, err := trading.NewInstrument(trading.InstrumentSpec{
		Base:          base,
		Quote:         quote,
		PriceScale:    2,
		QuantityScale: 3,
		PriceStep:     "0.10",
		QuantityStep:  "0.001",
		MinNotional:   "5",
	})
	if err != nil {
		t.Fatalf("NewInstrument() error = %v", err)
	}
	return instrument
}
