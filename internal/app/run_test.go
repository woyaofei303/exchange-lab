package app

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/config"
)

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
