package natspublisher_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/platform/natspublisher"
)

func TestCorePublisherPublishesAndFlushesWithinContext(t *testing.T) {
	t.Parallel()

	connection := &natsConnection{}
	publisher, err := natspublisher.New(connection)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := publisher.Publish(ctx, "market.BTCUSDT-PERP.ticker", []byte(`{"version":1}`)); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if connection.subject != "market.BTCUSDT-PERP.ticker" || connection.flushes != 1 {
		t.Fatalf("connection = %#v", connection)
	}
}

func TestCorePublisherSanitizesConnectionFailures(t *testing.T) {
	t.Parallel()

	secret := errors.New("nats://user:secret@example.test")
	connection := &natsConnection{publishErr: secret}
	publisher, err := natspublisher.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = publisher.Publish(ctx, "market.BTCUSDT-PERP.ticker", []byte(`{}`))
	if err == nil || err.Error() != "publish NATS message" {
		t.Fatalf("Publish() error = %v", err)
	}
}

type natsConnection struct {
	subject    string
	flushes    int
	publishErr error
	flushErr   error
}

func (c *natsConnection) Publish(subject string, _ []byte) error {
	c.subject = subject
	return c.publishErr
}

func (c *natsConnection) FlushWithContext(context.Context) error {
	c.flushes++
	return c.flushErr
}
