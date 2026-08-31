package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"exchange-lab/internal/identity"
	"exchange-lab/internal/ledger"
	"exchange-lab/internal/trading"
)

const ledgerScale = uint8(8)

type faucetRequest struct {
	Amount string `json:"amount"`
}

type balanceResponse struct {
	Asset     string `json:"asset"`
	Available string `json:"available"`
	Locked    string `json:"locked"`
	Total     string `json:"total"`
}

type faucetResponse struct {
	TransactionID string          `json:"transaction_id"`
	Replayed      bool            `json:"replayed"`
	Balance       balanceResponse `json:"balance"`
}

type balancesResponse struct {
	Data []balanceResponse `json:"data"`
}

func registerLedgerRoutes(mux *http.ServeMux, identities IdentityService, book LedgerService, logger *slog.Logger) {
	mux.HandleFunc("/v1/faucet", postOnly(logger, sameOrigin(logger, func(writer http.ResponseWriter, request *http.Request) {
		session, ok := authenticatedSession(writer, request, identities, logger)
		if !ok {
			return
		}
		idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
		if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
			writeLedgerError(writer, request, logger, ledger.ErrInvalid)
			return
		}
		var input faucetRequest
		request.Body = http.MaxBytesReader(writer, request.Body, 4096)
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
			writeLedgerError(writer, request, logger, ledger.ErrInvalid)
			return
		}
		asset, err := trading.NewAsset("USDT", ledgerScale)
		if err != nil {
			writeLedgerError(writer, request, logger, err)
			return
		}
		amount, err := trading.ParseMoney(asset, input.Amount)
		if err != nil || amount.Atoms() <= 0 {
			writeLedgerError(writer, request, logger, ledger.ErrInvalid)
			return
		}
		result, err := book.Faucet(request.Context(), session.User.ID, amount.Atoms(), idempotencyKey)
		if err != nil {
			writeLedgerError(writer, request, logger, err)
			return
		}
		formatted, err := publicBalance(result.Balance)
		if err != nil {
			writeLedgerError(writer, request, logger, err)
			return
		}
		writeResponse(loggerForRequest(request, logger), writer, http.StatusOK, faucetResponse{TransactionID: result.TransactionID, Replayed: result.Replayed, Balance: formatted})
	})))
	mux.HandleFunc("/v1/balances", getOnly(logger, func(writer http.ResponseWriter, request *http.Request) {
		session, ok := authenticatedSession(writer, request, identities, logger)
		if !ok {
			return
		}
		balance, err := book.Balance(request.Context(), session.User.ID)
		if err != nil {
			writeLedgerError(writer, request, logger, err)
			return
		}
		formatted, err := publicBalance(balance)
		if err != nil {
			writeLedgerError(writer, request, logger, err)
			return
		}
		writeResponse(loggerForRequest(request, logger), writer, http.StatusOK, balancesResponse{Data: []balanceResponse{formatted}})
	}))
}

func publicBalance(balance ledger.Balance) (balanceResponse, error) {
	if !balance.Valid() {
		return balanceResponse{}, ledger.ErrInvalid
	}
	available, err := trading.FormatDecimal(balance.AvailableAtoms, ledgerScale)
	if err != nil {
		return balanceResponse{}, err
	}
	locked, err := trading.FormatDecimal(balance.LockedAtoms, ledgerScale)
	if err != nil {
		return balanceResponse{}, err
	}
	total, err := trading.FormatDecimal(balance.Total(), ledgerScale)
	if err != nil {
		return balanceResponse{}, err
	}
	return balanceResponse{Asset: "USDT", Available: available, Locked: locked, Total: total}, nil
}

func writeLedgerError(writer http.ResponseWriter, request *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, ledger.ErrInvalid):
		writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, ledger.ErrConflict), errors.Is(err, identity.ErrConflict):
		writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusConflict, "idempotency_conflict", "idempotency key conflicts with a prior request")
	case errors.Is(err, ledger.ErrInsufficient):
		writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusConflict, "insufficient_balance", "insufficient balance")
	default:
		loggerForRequest(request, logger).Error("ledger operation failed")
		writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
