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

// NewNATSChecker 创建连接和 JetStream 就绪状态检查器。
func NewNATSChecker(connection *nats.Conn, jetStream nats.JetStreamContext) Checker {
	return natsChecker{connection: connection, jetStream: jetStream}
}

// Name 返回稳定的就绪检查依赖键。
func (natsChecker) Name() string { return "nats" }

// Check 校验连接存活且 JetStream 账户信息可读。
func (c natsChecker) Check(ctx context.Context) error {
	if c.connection.Status() != nats.CONNECTED {
		return fmt.Errorf("nats connection is not connected")
	}
	_, err := c.jetStream.AccountInfo(nats.Context(ctx))
	return err
}
