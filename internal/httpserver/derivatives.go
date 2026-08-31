package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"exchange-lab/internal/position"
	"exchange-lab/internal/risk"
	"exchange-lab/internal/trading"
)

// PositionReader 返回限定到用户范围的持久化仓位投影。
type PositionReader interface {
	List(context.Context, int64) ([]position.View, error)
}

// DerivativesService 提供风险、保证金、资金费和条件单操作。
type DerivativesService interface {
	Snapshot(context.Context, int64, string) (risk.Snapshot, error)
	SetLeverage(context.Context, int64, string, int64) error
	SetMode(context.Context, int64, string) error
	AdjustMargin(context.Context, int64, string, int64, string) (int64, bool, error)
	Liquidations(context.Context, int64) ([]risk.LiquidationRecord, error)
	FundingHistory(context.Context, string) ([]risk.FundingRecord, error)
	FundingPayments(context.Context, int64) ([]risk.FundingPayment, error)
	CreateConditional(context.Context, int64, risk.ConditionalOrder, string) (risk.ConditionalOrder, bool, error)
	Conditionals(context.Context, int64) ([]risk.ConditionalOrder, error)
	Insurance(context.Context) (risk.InsuranceStatus, error)
}

type positionResponse struct {
	Symbol        string `json:"symbol"`
	Size          string `json:"size"`
	EntryPrice    string `json:"entry_price"`
	MarkPrice     string `json:"mark_price"`
	RealizedPnL   string `json:"realized_pnl"`
	UnrealizedPnL string `json:"unrealized_pnl"`
	Fees          string `json:"fees"`
	Version       int64  `json:"version"`
	UpdatedAt     string `json:"updated_at"`
}
type dataResponse[T any] struct {
	Data T `json:"data"`
}

func registerDerivativeRoutes(mux *http.ServeMux, identities IdentityService, positions PositionReader, service DerivativesService, logger *slog.Logger) {
	mux.HandleFunc("/v1/positions", getOnly(logger, func(w http.ResponseWriter, r *http.Request) {
		session, ok := authenticatedSession(w, r, identities, logger)
		if !ok {
			return
		}
		items, err := positions.List(r.Context(), session.User.ID)
		if err != nil {
			writeDerivativeError(w, r, logger)
			return
		}
		result := make([]positionResponse, 0, len(items))
		for _, v := range items {
			size, _ := trading.FormatDecimal(v.SizeAtoms, v.QuantityScale)
			entry, _ := trading.FormatDecimal(v.EntryPriceAtoms, v.PriceScale)
			mark, _ := trading.FormatDecimal(v.MarkPriceAtoms, v.PriceScale)
			rpnl, _ := trading.FormatDecimal(v.RealizedPnLAtoms, v.QuoteScale)
			upnl, _ := trading.FormatDecimal(v.UnrealizedPnLAtoms, v.QuoteScale)
			fees, _ := trading.FormatDecimal(v.FeeAtoms, v.QuoteScale)
			result = append(result, positionResponse{Symbol: v.Symbol, Size: size, EntryPrice: entry, MarkPrice: mark, RealizedPnL: rpnl, UnrealizedPnL: upnl, Fees: fees, Version: v.Version, UpdatedAt: v.UpdatedAt.UTC().Format(timeFormat)})
		}
		writeResponse(loggerForRequest(r, logger), w, 200, dataResponse[[]positionResponse]{Data: result})
	}))
	mux.HandleFunc("/v1/risk", getOnly(logger, func(w http.ResponseWriter, r *http.Request) {
		session, ok := authenticatedSession(w, r, identities, logger)
		if !ok {
			return
		}
		symbol, err := requiredSymbolFilter(r)
		if err != nil {
			writeAPIError(loggerForRequest(r, logger), w, r, 400, "invalid_request", "symbol is required")
			return
		}
		snapshot, err := service.Snapshot(r.Context(), session.User.ID, symbol)
		if err != nil {
			writeDerivativeError(w, r, logger)
			return
		}
		writeResponse(loggerForRequest(r, logger), w, 200, dataResponse[map[string]any]{Data: publicRisk(snapshot)})
	}))
	mux.HandleFunc("/v1/leverage", postOnly(logger, sameOrigin(logger, func(w http.ResponseWriter, r *http.Request) {
		session, ok := authenticatedSession(w, r, identities, logger)
		if !ok {
			return
		}
		var input struct {
			Symbol   string `json:"symbol"`
			Leverage int64  `json:"leverage"`
		}
		if !decodeStrict(w, r, &input) || service.SetLeverage(r.Context(), session.User.ID, input.Symbol, input.Leverage) != nil {
			writeDerivativeError(w, r, logger)
			return
		}
		writeResponse(loggerForRequest(r, logger), w, 200, dataResponse[map[string]any]{Data: map[string]any{"symbol": input.Symbol, "leverage": input.Leverage}})
	})))
	mux.HandleFunc("/v1/margin-mode", postOnly(logger, sameOrigin(logger, func(w http.ResponseWriter, r *http.Request) {
		session, ok := authenticatedSession(w, r, identities, logger)
		if !ok {
			return
		}
		var input struct {
			Mode string `json:"mode"`
		}
		if !decodeStrict(w, r, &input) || service.SetMode(r.Context(), session.User.ID, input.Mode) != nil {
			writeDerivativeError(w, r, logger)
			return
		}
		writeResponse(loggerForRequest(r, logger), w, 200, dataResponse[map[string]string]{Data: map[string]string{"mode": input.Mode}})
	})))
	mux.HandleFunc("/v1/margin/adjust", postOnly(logger, sameOrigin(logger, func(w http.ResponseWriter, r *http.Request) {
		session, ok := authenticatedSession(w, r, identities, logger)
		if !ok {
			return
		}
		var input struct {
			Symbol string `json:"symbol"`
			Amount string `json:"amount"`
		}
		if !decodeStrict(w, r, &input) {
			writeDerivativeError(w, r, logger)
			return
		}
		asset, _ := trading.NewAsset("USDT", 8)
		amountAtoms, err := parseSignedMoney(asset, input.Amount)
		if err != nil || amountAtoms == 0 {
			writeDerivativeError(w, r, logger)
			return
		}
		after, replayed, err := service.AdjustMargin(r.Context(), session.User.ID, input.Symbol, amountAtoms, r.Header.Get("Idempotency-Key"))
		if err != nil {
			writeDerivativeError(w, r, logger)
			return
		}
		formatted, _ := trading.FormatDecimal(after, 8)
		writeResponse(loggerForRequest(r, logger), w, 200, dataResponse[map[string]any]{Data: map[string]any{"margin": formatted, "replayed": replayed}})
	})))
	mux.HandleFunc("/v1/liquidations", getOnly(logger, func(w http.ResponseWriter, r *http.Request) {
		session, ok := authenticatedSession(w, r, identities, logger)
		if !ok {
			return
		}
		items, err := service.Liquidations(r.Context(), session.User.ID)
		if err != nil {
			writeDerivativeError(w, r, logger)
			return
		}
		writeResponse(loggerForRequest(r, logger), w, 200, dataResponse[[]risk.LiquidationRecord]{Data: items})
	}))
	mux.HandleFunc("/v1/funding-rates", getOnly(logger, func(w http.ResponseWriter, r *http.Request) {
		symbol, err := requiredSymbolFilter(r)
		if err != nil {
			writeDerivativeError(w, r, logger)
			return
		}
		items, err := service.FundingHistory(r.Context(), symbol)
		if err != nil {
			writeDerivativeError(w, r, logger)
			return
		}
		writeResponse(loggerForRequest(r, logger), w, 200, dataResponse[[]risk.FundingRecord]{Data: items})
	}))
	mux.HandleFunc("/v1/funding-payments", getOnly(logger, func(w http.ResponseWriter, r *http.Request) {
		session, ok := authenticatedSession(w, r, identities, logger)
		if !ok {
			return
		}
		items, err := service.FundingPayments(r.Context(), session.User.ID)
		if err != nil {
			writeDerivativeError(w, r, logger)
			return
		}
		writeResponse(loggerForRequest(r, logger), w, 200, dataResponse[[]risk.FundingPayment]{Data: items})
	}))
	mux.HandleFunc("/v1/insurance-fund", getOnly(logger, func(w http.ResponseWriter, r *http.Request) {
		status, err := service.Insurance(r.Context())
		if err != nil {
			writeDerivativeError(w, r, logger)
			return
		}
		balance, _ := trading.FormatDecimal(status.BalanceAtoms, 8)
		writeResponse(loggerForRequest(r, logger), w, 200, dataResponse[map[string]any]{Data: map[string]any{"balance": balance, "adl_pending": status.ADLPending, "updated_at": status.UpdatedAt.UTC().Format(timeFormat)}})
	}))
	mux.HandleFunc("/v1/conditional-orders", func(w http.ResponseWriter, r *http.Request) {
		session, ok := authenticatedSession(w, r, identities, logger)
		if !ok {
			return
		}
		switch r.Method {
		case http.MethodGet:
			items, err := service.Conditionals(r.Context(), session.User.ID)
			if err != nil {
				writeDerivativeError(w, r, logger)
				return
			}
			writeResponse(loggerForRequest(r, logger), w, 200, dataResponse[[]risk.ConditionalOrder]{Data: items})
		case http.MethodPost:
			sameOrigin(logger, func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Symbol           string `json:"symbol"`
					Type             string `json:"type"`
					TriggerSource    string `json:"trigger_source"`
					TriggerDirection string `json:"trigger_direction"`
					TriggerPrice     string `json:"trigger_price"`
					Quantity         string `json:"quantity"`
				}
				if !decodeStrict(w, r, &input) {
					writeDerivativeError(w, r, logger)
					return
				}
				price, pe := trading.ParsePrice(input.TriggerPrice, 2)
				quantity, qe := trading.ParseQuantity(input.Quantity, 3)
				if pe != nil || qe != nil {
					writeDerivativeError(w, r, logger)
					return
				}
				item, replayed, err := service.CreateConditional(r.Context(), session.User.ID, risk.ConditionalOrder{Symbol: input.Symbol, Type: input.Type, TriggerSource: input.TriggerSource, TriggerDirection: input.TriggerDirection, TriggerPriceAtoms: price.Atoms(), QuantityAtoms: quantity.Atoms()}, r.Header.Get("Idempotency-Key"))
				if err != nil {
					writeDerivativeError(w, r, logger)
					return
				}
				writeResponse(loggerForRequest(r, logger), w, 200, dataResponse[map[string]any]{Data: map[string]any{"order": item, "replayed": replayed}})
			})(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			writeAPIError(loggerForRequest(r, logger), w, r, 405, "method_not_allowed", "method not allowed")
		}
	})
}

func decodeStrict(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil && decoder.Decode(&struct{}{}) == io.EOF
}

func parseSignedMoney(asset trading.Asset, value string) (int64, error) {
	negative := strings.HasPrefix(value, "-")
	unsigned := value
	if negative {
		unsigned = strings.TrimPrefix(value, "-")
	}
	money, err := trading.ParseMoney(asset, unsigned)
	if err != nil {
		return 0, err
	}
	if negative {
		return -money.Atoms(), nil
	}
	return money.Atoms(), nil
}
func publicRisk(s risk.Snapshot) map[string]any {
	format := func(v int64, scale uint8) string { x, _ := trading.FormatDecimal(v, scale); return x }
	return map[string]any{"symbol": s.Symbol, "mode": s.Mode, "leverage": s.Leverage, "position_size": format(s.PositionSizeAtoms, s.QuantityScale), "entry_price": format(s.EntryPriceAtoms, s.PriceScale), "mark_price": format(s.MarkPriceAtoms, s.PriceScale), "position_margin": format(s.PositionMarginAtoms, s.QuoteScale), "order_margin": format(s.OrderMarginAtoms, s.QuoteScale), "wallet_available": format(s.WalletAvailableAtoms, s.QuoteScale), "unrealized_pnl": format(s.UnrealizedPnLAtoms, s.QuoteScale), "margin_balance": format(s.MarginBalanceAtoms, s.QuoteScale), "maintenance_margin": format(s.MaintenanceMarginAtoms, s.QuoteScale), "margin_ratio_bps": s.MarginRatioBPS, "liquidation_distance_bps": s.LiquidationDistanceBPS, "risk_version": s.RiskVersion, "fresh": s.Fresh, "as_of": s.AsOf.UTC().Format(timeFormat)}
}
func writeDerivativeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger) {
	loggerForRequest(r, logger).Warn("derivative operation failed")
	writeAPIError(loggerForRequest(r, logger), w, r, 400, "invalid_request", "invalid request")
}
