// Package natspublisher 将 NATS Core 适配到有界的领域消息发布。
package natspublisher

import (
	"context"
	"fmt"
)

type connection interface {
	Publish(string, []byte) error
	FlushWithContext(context.Context) error
}

// CorePublisher 发布带刷新截止时间的临时 NATS Core 消息。
type CorePublisher struct {
	connection connection
}

// New 校验 NATS 连接。
func New(natsConnection connection) (*CorePublisher, error) {
	if natsConnection == nil {
		return nil, fmt.Errorf("create NATS publisher: connection is required")
	}
	return &CorePublisher{connection: natsConnection}, nil
}

// Publish 发送一条消息，并等待受上下文限制的刷新完成。
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
