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

func NewNATSChecker(connection *nats.Conn, jetStream nats.JetStreamContext) Checker {
	return natsChecker{connection: connection, jetStream: jetStream}
}

func (natsChecker) Name() string { return "nats" }

func (c natsChecker) Check(ctx context.Context) error {
	if c.connection.Status() != nats.CONNECTED {
		return fmt.Errorf("nats connection is not connected")
	}
	_, err := c.jetStream.AccountInfo(nats.Context(ctx))
	return err
}
