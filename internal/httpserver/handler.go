// Package httpserver exposes strict HTTP contracts over exchange services.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/candle"
	"github.com/woyaofei303/exchange-lab/internal/health"
	"github.com/woyaofei303/exchange-lab/internal/identity"
	"github.com/woyaofei303/exchange-lab/internal/ledger"
	"github.com/woyaofei303/exchange-lab/internal/market"
	"github.com/woyaofei303/exchange-lab/internal/observability"
	domainorder "github.com/woyaofei303/exchange-lab/internal/order"
	"github.com/woyaofei303/exchange-lab/internal/platform/rediscache"
	"github.com/woyaofei303/exchange-lab/internal/realtime"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

var (
	errInvalidQuery = errors.New("invalid query")
	symbolPattern   = regexp.MustCompile(`^[A-Z0-9]{4,24}-PERP$`)
)

// Readiness supplies dependency status for the public readiness route.
type Readiness interface {
	Check(context.Context) health.Report
}

type response struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

type instrumentResponse struct {
	Symbol        string `json:"symbol"`
	BaseAsset     string `json:"base_asset"`
	QuoteAsset    string `json:"quote_asset"`
	PriceScale    uint8  `json:"price_scale"`
	QuantityScale uint8  `json:"quantity_scale"`
	PriceStep     string `json:"price_step"`
	QuantityStep  string `json:"quantity_step"`
	MinNotional   string `json:"min_notional"`
}

type instrumentsResponse struct {
	Data []instrumentResponse `json:"data"`
}

type tickerResponse struct {
	Symbol      string `json:"symbol"`
	LastPrice   string `json:"last_price"`
	MarkPrice   string `json:"mark_price"`
	IndexPrice  string `json:"index_price"`
	PriceChange string `json:"price_change_24h"`
	Volume      string `json:"volume_24h"`
	UpdatedAt   string `json:"updated_at"`
}

type tickersResponse struct {
	Data []tickerResponse `json:"data"`
}

type apiError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type errorResponse struct {
	Error apiError `json:"error"`
}

// NewHandler builds the health and read-only market API surface.
func NewHandler(readiness Readiness, markets market.Reader, logger *slog.Logger, requestTimeout time.Duration) http.Handler {
	return newHandler(readiness, markets, Features{}, logger, requestTimeout)
}

// CandleReader returns bounded historical candle ranges.
type CandleReader interface {
	List(context.Context, candle.Query) ([]candle.Candle, error)
}

// NewHandlerWithCandles adds historical and realtime candle routes.
func NewHandlerWithCandles(readiness Readiness, markets market.Reader, candles CandleReader, hub *realtime.Hub, logger *slog.Logger, requestTimeout time.Duration) http.Handler {
	return newHandler(readiness, markets, Features{Candles: candles, CandleHub: hub}, logger, requestTimeout)
}

// IdentityService is the authentication and authorization surface used by handlers.
type IdentityService interface {
	Register(context.Context, string, string) (identity.User, error)
	Login(context.Context, string, string) (string, identity.Session, error)
	Authenticate(context.Context, string) (identity.Session, error)
	Refresh(context.Context, string) (string, identity.Session, error)
	Logout(context.Context, string) error
	AuditLog(context.Context, identity.User, int) ([]identity.AuditEvent, error)
}

// LoginLimiter enforces fail-closed credential attempt limits.
type LoginLimiter interface {
	Allow(context.Context, string) (rediscache.RateDecision, error)
}

// LedgerService exposes simulated funding and balance reads.
type LedgerService interface {
	Faucet(context.Context, int64, int64, string) (ledger.FaucetResult, error)
	Balance(context.Context, int64) (ledger.Balance, error)
}

// OrderService exposes user-scoped order commands and queries.
type OrderService interface {
	Place(context.Context, int64, domainorder.PlaceCommand) (domainorder.Result, error)
	Cancel(context.Context, int64, string, string) (domainorder.Result, error)
	List(context.Context, int64, int) ([]domainorder.Order, error)
}

// Features enables optional route groups only when their dependencies are present.
type Features struct {
	Candles          CandleReader
	CandleHub        *realtime.Hub
	Identity         IdentityService
	LoginLimiter     LoginLimiter
	AuthCookieSecure bool
	AuthSessionTTL   time.Duration
	Ledger           LedgerService
	Orders           OrderService
	Prices           PriceReader
	Positions        PositionReader
	Derivatives      DerivativesService
	AI               AISummarizer
	Metrics          *observability.Metrics
}

// NewHandlerWithFeatures builds the complete API from explicitly enabled services.
func NewHandlerWithFeatures(readiness Readiness, markets market.Reader, features Features, logger *slog.Logger, requestTimeout time.Duration) http.Handler {
	return newHandler(readiness, markets, features, logger, requestTimeout)
}

func newHandler(readiness Readiness, markets market.Reader, features Features, logger *slog.Logger, requestTimeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", getOnly(logger, func(w http.ResponseWriter, r *http.Request) {
		writeResponse(loggerForRequest(r, logger), w, http.StatusOK, response{Status: "ok"})
	}))
	mux.HandleFunc("/readyz", getOnly(logger, func(w http.ResponseWriter, r *http.Request) {
		requestLogger := loggerForRequest(r, logger)
		report := readiness.Check(r.Context())
		if report.Ready {
			writeResponse(requestLogger, w, http.StatusOK, response{Status: "ready", Checks: report.Checks})
			return
		}
		for name := range report.Errors {
			requestLogger.Error("dependency readiness check failed", "dependency", name)
		}
		writeResponse(requestLogger, w, http.StatusServiceUnavailable, response{Status: "not_ready", Checks: report.Checks})
	}))
	if features.Metrics != nil {
		mux.Handle("/metrics", features.Metrics.Handler())
	}
	mux.HandleFunc("/v1/instruments", getOnly(logger, func(w http.ResponseWriter, r *http.Request) {
		requestLogger := loggerForRequest(r, logger)
		symbol, err := symbolFilter(r)
		if err != nil {
			writeAPIError(requestLogger, w, r, http.StatusBadRequest, "invalid_request", "invalid request")
			return
		}
		instruments, err := markets.Instruments(r.Context(), symbol)
		if err != nil {
			writeMarketReadError(requestLogger, w, r, "list instruments failed", err)
			return
		}
		items := make([]instrumentResponse, 0, len(instruments))
		for _, instrument := range instruments {
			items = append(items, instrumentResponse{
				Symbol:        instrument.Symbol(),
				BaseAsset:     instrument.Base().Code(),
				QuoteAsset:    instrument.Quote().Code(),
				PriceScale:    instrument.PriceScale(),
				QuantityScale: instrument.QuantityScale(),
				PriceStep:     instrument.PriceStep().String(),
				QuantityStep:  instrument.QuantityStep().String(),
				MinNotional:   instrument.MinNotional().Decimal(),
			})
		}
		writeResponse(requestLogger, w, http.StatusOK, instrumentsResponse{Data: items})
	}))
	mux.HandleFunc("/v1/tickers", getOnly(logger, func(w http.ResponseWriter, r *http.Request) {
		requestLogger := loggerForRequest(r, logger)
		symbol, err := symbolFilter(r)
		if err != nil {
			writeAPIError(requestLogger, w, r, http.StatusBadRequest, "invalid_request", "invalid request")
			return
		}
		tickers, err := markets.Tickers(r.Context(), symbol)
		if err != nil {
			writeMarketReadError(requestLogger, w, r, "list tickers failed", err)
			return
		}
		items := make([]tickerResponse, 0, len(tickers))
		for _, ticker := range tickers {
			items = append(items, tickerResponse{
				Symbol:      ticker.Symbol,
				LastPrice:   ticker.LastPrice,
				MarkPrice:   ticker.MarkPrice,
				IndexPrice:  ticker.IndexPrice,
				PriceChange: ticker.PriceChange,
				Volume:      ticker.Volume,
				UpdatedAt:   ticker.UpdatedAt.UTC().Format(time.RFC3339Nano),
			})
		}
		writeResponse(requestLogger, w, http.StatusOK, tickersResponse{Data: items})
	}))
	if features.Candles != nil && features.CandleHub != nil {
		registerCandleRoutes(mux, features.Candles, features.CandleHub, logger)
	}
	if features.Prices != nil {
		registerPriceRoutes(mux, features.Prices, logger)
	}
	if features.Identity != nil && features.LoginLimiter != nil && features.AuthSessionTTL > 0 {
		registerIdentityRoutes(mux, features.Identity, features.LoginLimiter, features.AuthCookieSecure, features.AuthSessionTTL, logger)
		if features.Ledger != nil {
			registerLedgerRoutes(mux, features.Identity, features.Ledger, logger)
		}
		if features.Orders != nil {
			registerOrderRoutes(mux, features.Identity, features.Orders, logger)
		}
		if features.Positions != nil && features.Derivatives != nil {
			registerDerivativeRoutes(mux, features.Identity, features.Positions, features.Derivatives, logger)
		}
		if features.AI != nil {
			registerAIRoutes(mux, features.Identity, features.AI, logger)
		}
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(loggerForRequest(r, logger), w, r, http.StatusNotFound, "not_found", "resource not found")
	})
	handler := securityHeaders(requestLogging(logger, withRequestTimeout(requestTimeout, mux)))
	if features.Metrics != nil {
		return features.Metrics.Wrap("api", otelhttp.NewHandler(handler, "exchange-api"))
	}
	return handler
}

func symbolFilter(r *http.Request) (string, error) {
	query := r.URL.Query()
	if len(query) == 0 {
		return "", nil
	}
	values, ok := query["symbol"]
	if !ok || len(query) != 1 || len(values) != 1 || !symbolPattern.MatchString(values[0]) {
		return "", errInvalidQuery
	}
	return values[0], nil
}

func getOnly(logger *slog.Logger, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeAPIError(loggerForRequest(r, logger), w, r, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		next(w, r)
	}
}

func writeResponse(logger *slog.Logger, w http.ResponseWriter, status int, value any) {
	if err := writeJSON(w, status, value); err != nil {
		logger.Error("write http response failed")
	}
}

func writeAPIError(logger *slog.Logger, w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeResponse(logger, w, status, errorResponse{Error: apiError{
		Code:      code,
		Message:   message,
		RequestID: requestIDForRequest(r),
	}})
}

func writeMarketReadError(logger *slog.Logger, w http.ResponseWriter, r *http.Request, logMessage string, err error) {
	logger.Error(logMessage)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		writeAPIError(logger, w, r, http.StatusGatewayTimeout, "request_timeout", "request timed out")
	case errors.Is(err, context.Canceled):
		writeAPIError(logger, w, r, http.StatusRequestTimeout, "request_canceled", "request canceled")
	default:
		writeAPIError(logger, w, r, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) error {
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
