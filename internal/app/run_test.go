package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"exchange-lab/internal/config"
	"exchange-lab/internal/health"
	"github.com/nats-io/nats.go"
)

func TestConnectNATSReturnsPromptlyAndReadinessFailsWhileInitialConnectionRetries(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	connection, err := connectNATS("nats://" + address)
	if err != nil {
		t.Fatalf("connectNATS() error = %v", err)
	}
	defer connection.Close()
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("connectNATS() elapsed = %s, want no more than 1s", elapsed)
	}
	if status := connection.Status(); status != nats.RECONNECTING && status != nats.DISCONNECTED {
		t.Fatalf("connectNATS() status = %s, want reconnecting or disconnected", status)
	}

	jetStream, err := connection.JetStream()
	if err != nil {
		t.Fatalf("JetStream() error = %v", err)
	}
	if err := health.NewNATSChecker(connection, jetStream).Check(context.Background()); err == nil {
		t.Fatal("NATS readiness check error = nil while initial connection is unavailable")
	}
}

func TestConnectNATSReturnsSanitizedSynchronousError(t *testing.T) {
	const secret = "nats-config-secret"
	connection, err := connectNATS("nats://[" + secret)
	if connection != nil {
		connection.Close()
		t.Fatal("connectNATS() connection != nil")
	}
	if err == nil || err.Error() != "create nats client" {
		t.Fatalf("connectNATS() error = %v, want sanitized configuration error", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("connectNATS() leaked configuration detail: %v", err)
	}
}

func TestRedisCloseResultReturnsSanitizedCloseFailureWhenItIsSoleError(t *testing.T) {
	const secret = "redis-close-secret"
	err := redisCloseResult(nil, errors.New(secret))
	if err == nil || err.Error() != "close redis client" {
		t.Fatalf("redisCloseResult() error = %v, want sanitized close error", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("redisCloseResult() leaked close detail: %v", err)
	}
}

func TestRedisCloseResultPreservesPrimaryServeError(t *testing.T) {
	primary := errors.New("primary serve error")
	err := redisCloseResult(primary, errors.New("redis close error"))
	if !errors.Is(err, primary) {
		t.Fatalf("redisCloseResult() error = %v, want primary error", err)
	}
}

func TestRunReturnsSanitizedBindErrorBeforeInitializingDependencies(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	const secret = "bind-test-secret"
	cfg := config.Config{
		HTTPAddr:          occupied.Addr().String(),
		PostgresDSN:       "postgres://exchange:secret@127.0.0.1:1/exchange?sslmode=disable",
		RedisAddr:         "127.0.0.1:1",
		NATSURL:           "nats://" + secret + "@127.0.0.1:1",
		DependencyTimeout: 50 * time.Millisecond,
		ShutdownTimeout:   time.Second,
	}

	err = Run(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || err.Error() != "listen http server" {
		t.Fatalf("Run() error = %v, want sanitized listen error", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), cfg.HTTPAddr) {
		t.Fatalf("Run() leaked sensitive bind detail: %v", err)
	}
}

func TestDefaultTickerReaderProvidesBTCUSDTTicker(t *testing.T) {
	updatedAt := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	reader := defaultTickerReader(updatedAt)
	tickers, err := reader.Tickers(context.Background(), "")
	if err != nil {
		t.Fatalf("Tickers() error = %v", err)
	}
	if got, want := len(tickers), 1; got != want || tickers[0].Symbol != "BTCUSDT-PERP" || !tickers[0].UpdatedAt.Equal(updatedAt) {
		t.Fatalf("tickers = %#v", tickers)
	}
}

func TestServeDrainsActiveRequestAfterCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	requestReleased := false
	defer func() {
		if !requestReleased {
			close(releaseRequest)
		}
	}()
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseRequest
		w.WriteHeader(http.StatusNoContent)
	})}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	serverResult := make(chan error, 1)
	go func() {
		serverResult <- serve(ctx, server, listener, time.Second)
	}()

	requestResult := make(chan error, 1)
	go func() {
		client := http.Client{Timeout: time.Second}
		response, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			response.Body.Close()
		}
		requestResult <- err
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not reach server")
	}
	cancel()

	select {
	case err := <-serverResult:
		t.Fatalf("serve() returned before active request drained: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseRequest)
	requestReleased = true
	select {
	case err := <-requestResult:
		if err != nil {
			t.Fatalf("request error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not finish after release")
	}
	select {
	case err := <-serverResult:
		if err != nil {
			t.Fatalf("serve() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve() did not return after active request drained")
	}
}
