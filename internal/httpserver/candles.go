package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/woyaofei303/exchange-lab/internal/candle"
	"github.com/woyaofei303/exchange-lab/internal/realtime"
)

type candlesResponse struct {
	Data []candle.View `json:"data"`
}

func registerCandleRoutes(mux *http.ServeMux, reader CandleReader, hub *realtime.Hub, logger *slog.Logger) {
	mux.HandleFunc("/v1/candles", getOnly(logger, func(writer http.ResponseWriter, request *http.Request) {
		requestLogger := loggerForRequest(request, logger)
		query, err := candleHistoryQuery(request)
		if err != nil {
			writeAPIError(requestLogger, writer, request, http.StatusBadRequest, "invalid_request", "invalid request")
			return
		}
		items, err := reader.List(request.Context(), query)
		if err != nil {
			writeMarketReadError(requestLogger, writer, request, "list candles failed", err)
			return
		}
		views := make([]candle.View, 0, len(items))
		for _, item := range items {
			view, err := item.View()
			if err != nil {
				writeMarketReadError(requestLogger, writer, request, "encode candle failed", err)
				return
			}
			views = append(views, view)
		}
		writeResponse(requestLogger, writer, http.StatusOK, candlesResponse{Data: views})
	}))
	mux.HandleFunc("/ws/v1/candles", getOnly(logger, func(writer http.ResponseWriter, request *http.Request) {
		symbol, interval, err := candleStreamQuery(request)
		if err != nil {
			writeAPIError(loggerForRequest(request, logger), writer, request, http.StatusBadRequest, "invalid_request", "invalid request")
			return
		}
		serveCandleStream(writer, request, hub, symbol, interval)
	}))
}

func candleHistoryQuery(request *http.Request) (candle.Query, error) {
	query := request.URL.Query()
	if len(query) < 4 || len(query) > 5 {
		return candle.Query{}, errInvalidQuery
	}
	for key := range query {
		if key != "symbol" && key != "interval" && key != "start" && key != "end" && key != "limit" {
			return candle.Query{}, errInvalidQuery
		}
		if len(query[key]) != 1 {
			return candle.Query{}, errInvalidQuery
		}
	}
	symbol := query.Get("symbol")
	if !symbolPattern.MatchString(symbol) {
		return candle.Query{}, errInvalidQuery
	}
	interval, err := candle.ParseInterval(query.Get("interval"))
	if err != nil {
		return candle.Query{}, errInvalidQuery
	}
	start, err := time.Parse(time.RFC3339, query.Get("start"))
	if err != nil {
		return candle.Query{}, errInvalidQuery
	}
	end, err := time.Parse(time.RFC3339, query.Get("end"))
	if err != nil || !start.Before(end) {
		return candle.Query{}, errInvalidQuery
	}
	limit := 100
	if value := query.Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil {
			return candle.Query{}, errInvalidQuery
		}
	}
	if limit < 1 || limit > 1000 || end.Sub(start) > time.Duration(limit)*interval.Duration() {
		return candle.Query{}, errInvalidQuery
	}
	return candle.Query{Symbol: symbol, Interval: interval, Start: start.UTC(), End: end.UTC(), Limit: limit}, nil
}

func candleStreamQuery(request *http.Request) (string, candle.Interval, error) {
	query := request.URL.Query()
	if len(query) != 2 || len(query["symbol"]) != 1 || len(query["interval"]) != 1 || !symbolPattern.MatchString(query.Get("symbol")) {
		return "", 0, errInvalidQuery
	}
	interval, err := candle.ParseInterval(query.Get("interval"))
	if err != nil {
		return "", 0, errInvalidQuery
	}
	return query.Get("symbol"), interval, nil
}

func serveCandleStream(writer http.ResponseWriter, request *http.Request, hub *realtime.Hub, symbol string, interval candle.Interval) {
	connection, err := websocket.Accept(writer, request, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer connection.CloseNow()
	lifecycle := connection.CloseRead(request.Context())
	subscription := hub.SubscribeContext(lifecycle)
	defer subscription.Close()
	for {
		select {
		case <-lifecycle.Done():
			return
		case payload, ok := <-subscription.Messages():
			if !ok {
				_ = connection.Close(websocket.StatusPolicyViolation, "slow client")
				return
			}
			if !matchesCandle(payload, symbol, interval) {
				continue
			}
			writeContext, cancel := context.WithTimeout(lifecycle, 2*time.Second)
			err := connection.Write(writeContext, websocket.MessageText, payload)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func matchesCandle(payload []byte, symbol string, interval candle.Interval) bool {
	var update candle.Update
	if err := json.Unmarshal(payload, &update); err != nil {
		return false
	}
	return update.Version == 1 && update.Type == "candle" && update.Data.Symbol == symbol && update.Data.Interval == interval.String()
}
