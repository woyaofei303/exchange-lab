package httpserver

import (
	"context"
	"log/slog"
	"net/http"

	"exchange-lab/internal/contractprice"
	"exchange-lab/internal/trading"
)

// PriceReader 返回当前经过来源审计的合约价格。
type PriceReader interface {
	Latest(context.Context, string) (contractprice.Snapshot, error)
}

type priceResponse struct {
	Symbol          string   `json:"symbol"`
	IndexPrice      string   `json:"index_price"`
	MarkPrice       string   `json:"mark_price"`
	PremiumBPS      int64    `json:"premium_bps"`
	IncludedSources []string `json:"included_sources"`
	ExcludedSources []string `json:"excluded_sources"`
	Fresh           bool     `json:"fresh"`
	AsOf            string   `json:"as_of"`
}

func registerPriceRoutes(mux *http.ServeMux, prices PriceReader, logger *slog.Logger) {
	mux.HandleFunc("/v1/prices", getOnly(logger, func(w http.ResponseWriter, r *http.Request) {
		symbol, err := requiredSymbolFilter(r)
		if err != nil {
			writeAPIError(loggerForRequest(r, logger), w, r, http.StatusBadRequest, "invalid_request", "symbol is required")
			return
		}
		snapshot, err := prices.Latest(r.Context(), symbol)
		if err != nil {
			writeAPIError(loggerForRequest(r, logger), w, r, http.StatusServiceUnavailable, "price_unavailable", "risk price unavailable")
			return
		}
		indexPrice, indexErr := trading.FormatDecimal(snapshot.IndexAtoms, snapshot.PriceScale)
		markPrice, markErr := trading.FormatDecimal(snapshot.MarkAtoms, snapshot.PriceScale)
		if indexErr != nil || markErr != nil {
			writeAPIError(loggerForRequest(r, logger), w, r, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		writeResponse(loggerForRequest(r, logger), w, http.StatusOK, priceResponse{
			Symbol: snapshot.Symbol, IndexPrice: indexPrice, MarkPrice: markPrice,
			PremiumBPS: snapshot.PremiumBPS, IncludedSources: snapshot.IncludedSources,
			ExcludedSources: snapshot.ExcludedSources, Fresh: snapshot.Fresh,
			AsOf: snapshot.AsOf.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		})
	}))
}

func requiredSymbolFilter(r *http.Request) (string, error) {
	symbol, err := symbolFilter(r)
	if err != nil || symbol == "" {
		return "", errInvalidQuery
	}
	return symbol, nil
}
