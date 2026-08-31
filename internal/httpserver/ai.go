package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	contractai "github.com/woyaofei303/exchange-lab/internal/ai"
)

// AISummarizer exposes read-only, evidence-bound risk explanations.
type AISummarizer interface {
	Summarize(context.Context, int64, string) (contractai.Summary, contractai.Usage, error)
}

func registerAIRoutes(mux *http.ServeMux, identities IdentityService, service AISummarizer, logger *slog.Logger) {
	mux.HandleFunc("/v1/ai/contract-risk-summary", getOnly(logger, func(w http.ResponseWriter, r *http.Request) {
		session, ok := authenticatedSession(w, r, identities, logger)
		if !ok {
			return
		}
		symbol, err := requiredSymbolFilter(r)
		if err != nil {
			writeAPIError(loggerForRequest(r, logger), w, r, 400, "invalid_request", "symbol is required")
			return
		}
		summary, usage, err := service.Summarize(r.Context(), session.User.ID, symbol)
		if err != nil {
			loggerForRequest(r, logger).Warn("AI risk summary unavailable")
			writeAPIError(loggerForRequest(r, logger), w, r, 503, "ai_unavailable", "AI explanation unavailable")
			return
		}
		payload := dataResponse[map[string]any]{Data: map[string]any{"summary": summary, "usage": usage}}
		if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			writeResponse(loggerForRequest(r, logger), w, 200, payload)
			return
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			writeAPIError(loggerForRequest(r, logger), w, r, 500, "internal_error", "internal server error")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("event: risk-summary\ndata: "))
		_, _ = w.Write(encoded)
		_, _ = w.Write([]byte("\n\nevent: done\ndata: {}\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))
}
