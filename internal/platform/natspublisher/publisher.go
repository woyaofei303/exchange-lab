// Package natspublisher adapts NATS Core to bounded domain publication.
package natspublisher

import (
	"context"
	"fmt"
)

type connection interface {
	Publish(string, []byte) error
	FlushWithContext(context.Context) error
}

// CorePublisher publishes ephemeral NATS Core messages with a flush deadline.
type CorePublisher struct {
	connection connection
}

// New validates the NATS connection.
func New(natsConnection connection) (*CorePublisher, error) {
	if natsConnection == nil {
		return nil, fmt.Errorf("create NATS publisher: connection is required")
	}
	return &CorePublisher{connection: natsConnection}, nil
}

// Publish sends one message and waits for a context-bounded flush.
func (p *CorePublisher) Publish(ctx context.Context, subject string, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if subject == "" || len(payload) == 0 {
		return fmt.Errorf("publish NATS message: invalid input")
	}
	if err := p.connection.Publish(subject, payload); err != nil {
		return fmt.Errorf("publish NATS message")
	}
	if err := p.connection.FlushWithContext(ctx); err != nil {
		return fmt.Errorf("flush NATS message")
	}
	return nil
}
