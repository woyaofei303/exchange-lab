// Command candlecheck verifies the running candle pipeline end to end.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/nats-io/nats.go"
	"github.com/woyaofei303/exchange-lab/internal/candle"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("candle pipeline: trade -> PostgreSQL -> REST + WebSocket verified")
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	natsURL := environmentOr("NATS_URL", "nats://127.0.0.1:4222")
	natsConnection, err := nats.Connect(natsURL, nats.Name("exchange-lab-candle-check"), nats.Timeout(2*time.Second))
	if err != nil {
		return errors.New("candle check: connect NATS")
	}
	defer natsConnection.Close()
	subscription, err := natsConnection.SubscribeSync("candles.*.1m")
	if err != nil {
		return errors.New("candle check: subscribe updates")
	}
	defer subscription.Unsubscribe()
	if err := natsConnection.FlushWithContext(ctx); err != nil {
		return errors.New("candle check: activate subscription")
	}

	baseURL := environmentOr("BASE_URL", "http://127.0.0.1:8080")
	webSocketURL := strings.Replace(baseURL, "http://", "ws://", 1)
	webSocketURL = strings.Replace(webSocketURL, "https://", "wss://", 1)
	webSocketURL += "/ws/v1/candles?symbol=BTCUSDT-PERP&interval=1m"
	connection, _, err := websocket.Dial(ctx, webSocketURL, nil)
	if err != nil {
		return errors.New("candle check: connect WebSocket")
	}
	defer connection.CloseNow()

	message, err := subscription.NextMsgWithContext(ctx)
	if err != nil {
		return errors.New("candle check: wait for NATS candle")
	}
	update, err := validateCandleMessage(message.Subject, message.Data)
	if err != nil {
		return err
	}
	messageType, webSocketPayload, err := connection.Read(ctx)
	if err != nil || messageType != websocket.MessageText {
		return errors.New("candle check: read WebSocket update")
	}
	webSocketUpdate, err := candle.DecodeUpdate(webSocketPayload)
	if err != nil || webSocketUpdate.Data.Symbol != update.Data.Symbol || webSocketUpdate.Data.Interval != "1m" {
		return errors.New("candle check: invalid WebSocket update")
	}

	historyURL, err := url.Parse(baseURL + "/v1/candles")
	if err != nil {
		return errors.New("candle check: invalid API URL")
	}
	query := historyURL.Query()
	query.Set("symbol", update.Data.Symbol)
	query.Set("interval", update.Data.Interval)
	query.Set("start", update.Data.OpenTime.UTC().Format(time.RFC3339))
	query.Set("end", update.Data.CloseTime.UTC().Format(time.RFC3339))
	query.Set("limit", "1")
	historyURL.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, historyURL.String(), nil)
	if err != nil {
		return errors.New("candle check: create history request")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return errors.New("candle check: request history")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("candle check: history status mismatch")
	}
	var history struct {
		Data []candle.View `json:"data"`
	}
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&history); err != nil || len(history.Data) != 1 || history.Data[0].Symbol != update.Data.Symbol || !history.Data[0].OpenTime.Equal(update.Data.OpenTime) {
		return errors.New("candle check: history body mismatch")
	}
	return nil
}

func validateCandleMessage(subject string, payload []byte) (candle.Update, error) {
	update, err := candle.DecodeUpdate(payload)
	if err != nil {
		return candle.Update{}, errors.New("candle check: invalid update")
	}
	wantSubject, err := candle.UpdateSubject(update)
	if err != nil || wantSubject != subject {
		return candle.Update{}, errors.New("candle check: subject mismatch")
	}
	return update, nil
}

func environmentOr(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}
