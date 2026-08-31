package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/woyaofei303/exchange-lab/internal/ledger"
	domainorder "github.com/woyaofei303/exchange-lab/internal/order"
	"github.com/woyaofei303/exchange-lab/internal/trading"
)

type placeOrderRequest struct {
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	Type          string `json:"type"`
	Price         string `json:"price"`
	Quantity      string `json:"quantity"`
	Leverage      int64  `json:"leverage"`
	ClientOrderID string `json:"client_order_id"`
}

type orderResponse struct {
	ID             string              `json:"id"`
	ClientOrderID  string              `json:"client_order_id"`
	Symbol         string              `json:"symbol"`
	Side           trading.Side        `json:"side"`
	Type           trading.OrderType   `json:"type"`
	Status         trading.OrderStatus `json:"status"`
	Price          string              `json:"price"`
	Quantity       string              `json:"quantity"`
	Remaining      string              `json:"remaining"`
	Leverage       int64               `json:"leverage"`
	ReservedMargin string              `json:"reserved_margin"`
	CreatedAt      string              `json:"created_at"`
	UpdatedAt      string              `json:"updated_at"`
}

type orderResultResponse struct {
	Data     orderResponse `json:"data"`
	Replayed bool          `json:"replayed"`
}

type ordersResponse struct {
	Data []orderResponse `json:"data"`
}

func registerOrderRoutes(mux *http.ServeMux, identities IdentityService, service OrderService, logger *slog.Logger) {
	mux.HandleFunc("/v1/orders", func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			listOrders(writer, request, identities, service, logger)
		case http.MethodPost:
			sameOrigin(logger, func(writer http.ResponseWriter, request *http.Request) {
				placeOrder(writer, request, identities, service, logger)
			})(writer, request)
		default:
			writer.Header().Set("Allow", "GET, POST")
			writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		}
	})
	mux.HandleFunc("/v1/orders/", postOnly(logger, sameOrigin(logger, func(writer http.ResponseWriter, request *http.Request) {
		suffix := strings.TrimPrefix(request.URL.Path, "/v1/orders/")
		if !strings.HasSuffix(suffix, "/cancel") || strings.Count(suffix, "/") != 1 {
			writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusNotFound, "not_found", "resource not found")
			return
		}
		orderID := strings.TrimSuffix(suffix, "/cancel")
		if orderID == "" || len(orderID) > 128 {
			writeOrderError(writer, request, logger, domainorder.ErrInvalid)
			return
		}
		session, ok := authenticatedSession(writer, request, identities, logger)
		if !ok {
			return
		}
		result, err := service.Cancel(request.Context(), session.User.ID, orderID, request.Header.Get("Idempotency-Key"))
		if err != nil {
			writeOrderError(writer, request, logger, err)
			return
		}
		response, err := publicOrder(result.Order)
		if err != nil {
			writeOrderError(writer, request, logger, err)
			return
		}
		writeResponse(loggerForRequest(request, logger), writer, http.StatusOK, orderResultResponse{Data: response, Replayed: result.Replayed})
	})))
}

func placeOrder(writer http.ResponseWriter, request *http.Request, identities IdentityService, service OrderService, logger *slog.Logger) {
	session, ok := authenticatedSession(writer, request, identities, logger)
	if !ok {
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 8192)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input placeOrderRequest
	if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF || input.Type != "limit" {
		writeOrderError(writer, request, logger, domainorder.ErrInvalid)
		return
	}
	result, err := service.Place(request.Context(), session.User.ID, domainorder.PlaceCommand{
		Symbol: input.Symbol, Side: input.Side, Price: input.Price, Quantity: input.Quantity,
		Leverage: input.Leverage, ClientOrderID: input.ClientOrderID, IdempotencyKey: request.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		writeOrderError(writer, request, logger, err)
		return
	}
	response, err := publicOrder(result.Order)
	if err != nil {
		writeOrderError(writer, request, logger, err)
		return
	}
	writeResponse(loggerForRequest(request, logger), writer, http.StatusOK, orderResultResponse{Data: response, Replayed: result.Replayed})
}

func listOrders(writer http.ResponseWriter, request *http.Request, identities IdentityService, service OrderService, logger *slog.Logger) {
	session, ok := authenticatedSession(writer, request, identities, logger)
	if !ok {
		return
	}
	limit := 100
	if value := request.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeOrderError(writer, request, logger, domainorder.ErrInvalid)
			return
		}
		limit = parsed
	}
	items, err := service.List(request.Context(), session.User.ID, limit)
	if err != nil {
		writeOrderError(writer, request, logger, err)
		return
	}
	responses := make([]orderResponse, 0, len(items))
	for _, item := range items {
		response, err := publicOrder(item)
		if err != nil {
			writeOrderError(writer, request, logger, err)
			return
		}
		responses = append(responses, response)
	}
	writeResponse(loggerForRequest(request, logger), writer, http.StatusOK, ordersResponse{Data: responses})
}

func publicOrder(item domainorder.Order) (orderResponse, error) {
	price, err := trading.FormatDecimal(item.PriceAtoms, item.PriceScale)
	if err != nil {
		return orderResponse{}, err
	}
	quantity, err := trading.FormatDecimal(item.QuantityAtoms, item.QuantityScale)
	if err != nil {
		return orderResponse{}, err
	}
	remaining, err := trading.FormatDecimal(item.RemainingAtoms, item.QuantityScale)
	if err != nil {
		return orderResponse{}, err
	}
	reserved, err := trading.FormatDecimal(item.ReservedMarginAtoms, item.QuoteScale)
	if err != nil {
		return orderResponse{}, err
	}
	return orderResponse{ID: item.ID, ClientOrderID: item.ClientOrderID, Symbol: item.Symbol, Side: item.Side, Type: item.Type, Status: item.Status, Price: price, Quantity: quantity, Remaining: remaining, Leverage: item.Leverage, ReservedMargin: reserved, CreatedAt: item.CreatedAt.UTC().Format(timeFormat), UpdatedAt: item.UpdatedAt.UTC().Format(timeFormat)}, nil
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func writeOrderError(writer http.ResponseWriter, request *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, domainorder.ErrInvalid):
		writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, domainorder.ErrNotFound):
		writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, domainorder.ErrForbidden):
		writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusForbidden, "forbidden", "permission denied")
	case errors.Is(err, domainorder.ErrConflict):
		writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusConflict, "idempotency_conflict", "idempotency key conflicts with a prior request")
	case errors.Is(err, ledger.ErrInsufficient):
		writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusConflict, "insufficient_balance", "insufficient balance")
	default:
		loggerForRequest(request, logger).Error("order operation failed")
		writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
