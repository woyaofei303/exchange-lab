package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sync/atomic"
	"time"
)

const requestIDHeader = "X-Request-ID"

var (
	requestIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	requestIDFallback atomic.Uint64
)

type requestMetadataContextKey struct{}

type requestMetadata struct {
	logger    *slog.Logger
	requestID string
}

func requestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(requestIDHeader)
		if !requestIDPattern.MatchString(requestID) {
			requestID = newRequestID()
		}
		w.Header().Set(requestIDHeader, requestID)
		requestLogger := logger.With("request_id", requestID)
		r = r.WithContext(context.WithValue(r.Context(), requestMetadataContextKey{}, requestMetadata{
			logger:    requestLogger,
			requestID: requestID,
		}))
		startedAt := time.Now()
		tracked := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(tracked, r)
		requestLogger.Info("http request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", tracked.statusCode(),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func withRequestTimeout(timeout time.Duration, next http.Handler) http.Handler {
	if timeout <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ws/v1/candles" {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func loggerForRequest(r *http.Request, fallback *slog.Logger) *slog.Logger {
	if metadata, ok := r.Context().Value(requestMetadataContextKey{}).(requestMetadata); ok {
		return metadata.logger
	}
	if fallback != nil {
		return fallback
	}
	return slog.Default()
}

func requestIDForRequest(r *http.Request) string {
	if metadata, ok := r.Context().Value(requestMetadataContextKey{}).(requestMetadata); ok {
		return metadata.requestID
	}
	return ""
}

func newRequestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	}
	return fmt.Sprintf("fallback-%016x", requestIDFallback.Add(1))
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

// Unwrap exposes the underlying writer to optional HTTP interfaces.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// WriteHeader records and forwards only the first response status.
func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Write records an implicit 200 response before forwarding bytes.
func (w *statusWriter) Write(payload []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(payload)
}

func (w *statusWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}
