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

	"github.com/woyaofei303/exchange-lab/internal/health"
	"github.com/woyaofei303/exchange-lab/internal/httpserver"
)

type readiness struct{ report health.Report }

func (r readiness) Check(context.Context) health.Report { return r.report }

func request(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

func TestHealthzReturnsProcessHealth(t *testing.T) {
	handler := httpserver.NewHandler(readiness{}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	response := request(t, handler, http.MethodGet, "/healthz")
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("health response = %d %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body["status"] != "ok" {
		t.Fatalf("health body = %#v, err = %v", body, err)
	}
}

func TestReadyzReturnsServiceUnavailableWithoutLeakingError(t *testing.T) {
	secret := "postgres://exchange:super-secret@postgres:5432/exchange"
	report := health.Report{
		Ready:  false,
		Checks: map[string]string{"postgres": health.StatusError, "redis": health.StatusOK, "nats": health.StatusOK},
		Errors: map[string]error{"postgres": errors.New(secret)},
	}
	handler := httpserver.NewHandler(readiness{report: report}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	response := request(t, handler, http.MethodGet, "/readyz")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready response = %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "super-secret") || strings.Contains(response.Body.String(), "postgres://") {
		t.Fatalf("ready response leaked secret: %s", response.Body.String())
	}
}

func TestHealthEndpointsRejectNonGETMethods(t *testing.T) {
	handler := httpserver.NewHandler(readiness{}, slog.Default())
	for _, path := range []string{"/healthz", "/readyz"} {
		response := request(t, handler, http.MethodPost, path)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s status = %d", path, response.Code)
		}
	}
}

type failingWriter struct{ header http.Header }

func (w *failingWriter) Header() http.Header     { return w.header }
func (*failingWriter) WriteHeader(int)           {}
func (*failingWriter) Write([]byte) (int, error) { return 0, errors.New("writer-secret") }

func TestHandlerLogsSanitizedResponseWriteFailure(t *testing.T) {
	var logs bytes.Buffer
	handler := httpserver.NewHandler(readiness{}, slog.New(slog.NewTextHandler(&logs, nil)))
	handler.ServeHTTP(&failingWriter{header: make(http.Header)}, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if !strings.Contains(logs.String(), "write http response failed") {
		t.Fatalf("log did not record response failure: %s", logs.String())
	}
	if strings.Contains(logs.String(), "writer-secret") {
		t.Fatalf("log leaked writer detail: %s", logs.String())
	}
}
