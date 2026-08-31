package health

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
)

type natsChecker struct {
	connection *nats.Conn
	jetStream  nats.JetStreamContext
}

// NewNATSChecker constructs a checker for connection and JetStream readiness.
func NewNATSChecker(connection *nats.Conn, jetStream nats.JetStreamContext) Checker {
	return natsChecker{connection: connection, jetStream: jetStream}
}

// Name returns the stable readiness dependency key.
func (natsChecker) Name() string { return "nats" }

// Check verifies a live connection and readable JetStream account information.
func (c natsChecker) Check(ctx context.Context) error {
	if c.connection.Status() != nats.CONNECTED {
		return fmt.Errorf("nats connection is not connected")
	}
	_, err := c.jetStream.AccountInfo(nats.Context(ctx))
	return err
}
