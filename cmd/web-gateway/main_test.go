package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandlerServesHealthAssetsAndSPAFallback(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<title>terminal</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "asset.js"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler, err := newHandler(staticDir, "http://api:8080")
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		path, body string
	}{
		{"/healthz", "{\"status\":\"ok\"}\n"},
		{"/asset.js", "ok"},
		{"/positions/BTCUSDT-PERP", "<title>terminal</title>"},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || response.Body.String() != test.body {
			t.Fatalf("GET %s = %d %q", test.path, response.Code, response.Body.String())
		}
		if response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("GET %s missing security headers", test.path)
		}
	}
}

func TestHandlerRejectsInvalidConfiguration(t *testing.T) {
	if _, err := newHandler("", "http://api:8080"); err == nil {
		t.Fatal("newHandler() error = nil")
	}
	if _, err := newHandler(t.TempDir(), "file:///tmp/api"); err == nil {
		t.Fatal("newHandler() error = nil")
	}
}
