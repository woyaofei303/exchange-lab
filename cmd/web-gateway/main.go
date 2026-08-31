// Command web-gateway 提供 React 终端并代理 API 流量。
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	address := envOr("WEB_HTTP_ADDR", ":3000")
	staticDir := envOr("WEB_STATIC_DIR", "/srv/web")
	apiURL := envOr("WEB_API_URL", "http://api:8080")
	handler, err := newHandler(staticDir, apiURL)
	if err != nil {
		logger.Error("web gateway configuration invalid")
		os.Exit(1)
	}
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       70 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			logger.Error("web gateway shutdown failed")
		}
	}()
	logger.Info("web gateway started", "address", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("web gateway stopped unexpectedly")
		os.Exit(1)
	}
}

func newHandler(staticDir, apiAddress string) (http.Handler, error) {
	if staticDir == "" {
		return nil, fmt.Errorf("web gateway: empty static directory")
	}
	info, err := os.Stat(staticDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("web gateway: static directory unavailable")
	}
	upstream, err := url.Parse(apiAddress)
	if err != nil || (upstream.Scheme != "http" && upstream.Scheme != "https") || upstream.Host == "" || upstream.User != nil {
		return nil, fmt.Errorf("web gateway: invalid API URL")
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.ErrorHandler = func(writer http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(writer, "upstream unavailable", http.StatusBadGateway)
	}
	staticFiles := spaHandler(staticDir)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.Handle("/v1/", proxy)
	mux.Handle("/ws/", proxy)
	mux.Handle("/", staticFiles)
	return securityHeaders(mux), nil
}

func spaHandler(staticDir string) http.Handler {
	root := os.DirFS(staticDir)
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.Header().Set("Allow", "GET, HEAD")
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(request.URL.Path, "/")
		if path == "" {
			path = "."
		}
		if _, err := fs.Stat(root, path); err != nil {
			path = "."
		}
		copy := request.Clone(request.Context())
		if path == "." {
			copy.URL.Path = "/"
		} else {
			copy.URL.Path = "/" + path
		}
		files.ServeHTTP(writer, copy)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(writer, request)
	})
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
