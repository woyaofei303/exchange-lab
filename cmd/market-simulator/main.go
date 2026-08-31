// Command market-simulator 提供确定性的本地行情数据流。
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"exchange-lab/internal/config"
	"exchange-lab/internal/marketdata"
	"exchange-lab/internal/platform/websocketfeed"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.LoadMarketSimulator(os.LookupEnv)
	if err != nil {
		logger.Error("invalid simulator configuration", "error", err)
		os.Exit(1)
	}
	handler, err := websocketfeed.NewSimulatorHandler(cfg.Interval, func() (*marketdata.Simulator, error) {
		streamID, err := newStreamID()
		if err != nil {
			return nil, err
		}
		return marketdata.NewSimulator(marketdata.SimulatorConfig{
			StreamID: streamID, Symbol: "BTCUSDT-PERP", PriceScale: 2, QuantityScale: 3,
			InitialPriceAtoms: 5_000_000, TickAtoms: 10, QuantityAtoms: 1, Now: time.Now,
		})
	})
	if err != nil {
		logger.Error("create simulator handler")
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.Handle("/ws", handler)
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"ok"}`))
	})
	server := &http.Server{
		Addr: cfg.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serve(ctx, server, cfg.ShutdownTimeout); err != nil {
		logger.Error("market simulator stopped")
		os.Exit(1)
	}
}

func newStreamID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", errors.New("create simulator stream identity")
	}
	return "sim-" + hex.EncodeToString(random[:]), nil
}

func serve(ctx context.Context, server *http.Server, shutdownTimeout time.Duration) error {
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("serve market simulator")
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return errors.New("shutdown market simulator")
		}
		err := <-serverErrors
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("serve market simulator")
	}
}
