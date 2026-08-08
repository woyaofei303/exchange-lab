package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/woyaofei303/exchange-lab/internal/health"
)

type Readiness interface {
	Check(context.Context) health.Report
}

type response struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

func NewHandler(readiness Readiness, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", getOnly(logger, func(w http.ResponseWriter, _ *http.Request) {
		writeResponse(logger, w, http.StatusOK, response{Status: "ok"})
	}))
	mux.HandleFunc("/readyz", getOnly(logger, func(w http.ResponseWriter, r *http.Request) {
		report := readiness.Check(r.Context())
		if report.Ready {
			writeResponse(logger, w, http.StatusOK, response{Status: "ready", Checks: report.Checks})
			return
		}
		for name := range report.Errors {
			logger.Error("dependency readiness check failed", "dependency", name)
		}
		writeResponse(logger, w, http.StatusServiceUnavailable, response{Status: "not_ready", Checks: report.Checks})
	}))
	return mux
}

func getOnly(logger *slog.Logger, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeResponse(logger, w, http.StatusMethodNotAllowed, response{Status: "method_not_allowed"})
			return
		}
		next(w, r)
	}
}

func writeResponse(logger *slog.Logger, w http.ResponseWriter, status int, value response) {
	if err := writeJSON(w, status, value); err != nil {
		logger.Error("write http response failed")
	}
}

func writeJSON(w http.ResponseWriter, status int, value response) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(payload)
	return err
}
