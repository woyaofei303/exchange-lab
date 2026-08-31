// Package eventing 定义持久化事件信封、Outbox 发布和投递机制。
package eventing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"
)

// ErrInvalidEnvelope 表示事件不受支持、格式错误或大小超限。
var ErrInvalidEnvelope = errors.New("invalid event envelope")

var subjectPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{2,127}$`)

// OutboxEvent 是已租用、等待可靠发布的 PostgreSQL 事实记录。
type OutboxEvent struct {
	ID            string
	AggregateType string
	AggregateID   string
	Subject       string
	Version       int
	CorrelationID string
	Payload       json.RawMessage
	Attempts      int
	CreatedAt     time.Time
}

// Envelope 是事件生产者和消费者共享的不可变版本化契约。
type Envelope struct {
	Version       int             `json:"version"`
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	CorrelationID string          `json:"correlation_id"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Data          json.RawMessage `json:"data"`
}

// Encode 校验 Outbox 事件并序列化其公开信封。
func Encode(event OutboxEvent) ([]byte, error) {
	if !event.valid() {
		return nil, ErrInvalidEnvelope
	}
	return json.Marshal(Envelope{Version: event.Version, ID: event.ID, Type: event.Subject, AggregateType: event.AggregateType, AggregateID: event.AggregateID, CorrelationID: event.CorrelationID, OccurredAt: event.CreatedAt.UTC(), Data: event.Payload})
}

// Decode 严格解析一个大小受限的 v1 事件信封。
func Decode(payload []byte) (Envelope, error) {
	if len(payload) == 0 || len(payload) > 1<<20 {
		return Envelope{}, ErrInvalidEnvelope
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var envelope Envelope
	if err := decoder.Decode(&envelope); err != nil {
		return Envelope{}, ErrInvalidEnvelope
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || !envelope.valid() {
		return Envelope{}, ErrInvalidEnvelope
	}
	return envelope, nil
}

func (e Envelope) valid() bool {
	return e.Version == 1 && len(e.ID) >= 8 && len(e.ID) <= 128 && len(e.AggregateType) >= 1 && len(e.AggregateType) <= 64 && len(e.AggregateID) >= 1 && len(e.AggregateID) <= 128 && validCorrelation(e.CorrelationID) && subjectPattern.MatchString(e.Type) && !e.OccurredAt.IsZero() && validJSONObject(e.Data)
}

func (e OutboxEvent) valid() bool {
	return e.Version == 1 && len(e.ID) >= 8 && len(e.ID) <= 128 && len(e.AggregateType) >= 1 && len(e.AggregateType) <= 64 && len(e.AggregateID) >= 1 && len(e.AggregateID) <= 128 && validCorrelation(e.CorrelationID) && subjectPattern.MatchString(e.Subject) && !e.CreatedAt.IsZero() && validJSONObject(e.Payload)
}

func validCorrelation(value string) bool {
	return len(value) >= 1 && len(value) <= 128
}

func validJSONObject(payload []byte) bool {
	var object map[string]any
	return len(payload) <= 1<<20 && json.Unmarshal(payload, &object) == nil && object != nil
}

// OutboxStore 租用事件并记录最终发布结果。
type OutboxStore interface {
	Claim(context.Context, int, time.Duration) ([]OutboxEvent, error)
	MarkPublished(context.Context, string, time.Time) error
	MarkFailed(context.Context, OutboxEvent, string, time.Time) error
}

// Transport 可靠发布一个 Outbox 事件。
type Transport interface {
	Publish(context.Context, OutboxEvent) error
}

// PublisherConfig 限制每轮轮询和发布操作。
type PublisherConfig struct {
	BatchSize        int
	Lease            time.Duration
	OperationTimeout time.Duration
	Now              func() time.Time
}

// Publisher 处理已租用的 Outbox 事件，但不会删除发布失败的事实记录。
type Publisher struct {
	store     OutboxStore
	transport Transport
	config    PublisherConfig
}

// NewPublisher 校验有界的 Outbox 发布器配置。
func NewPublisher(store OutboxStore, transport Transport, config PublisherConfig) (*Publisher, error) {
	if store == nil || transport == nil || config.BatchSize < 1 || config.BatchSize > 1000 || config.Lease <= 0 || config.OperationTimeout <= 0 || config.Now == nil {
		return nil, fmt.Errorf("create outbox publisher: invalid configuration")
	}
	return &Publisher{store: store, transport: transport, config: config}, nil
}

// Tick 最多处理一个配置批次，并返回发布数量。
func (p *Publisher) Tick(ctx context.Context) (int, error) {
	operation, cancel := context.WithTimeout(ctx, p.config.OperationTimeout)
	events, err := p.store.Claim(operation, p.config.BatchSize, p.config.Lease)
	cancel()
	if err != nil {
		return 0, fmt.Errorf("publish outbox: claim: %w", err)
	}
	var failures []error
	published := 0
	for _, event := range events {
		operation, cancel := context.WithTimeout(ctx, p.config.OperationTimeout)
		err := p.transport.Publish(operation, event)
		cancel()
		now := p.config.Now().UTC()
		if err == nil {
			operation, cancel = context.WithTimeout(ctx, p.config.OperationTimeout)
			err = p.store.MarkPublished(operation, event.ID, now)
			cancel()
			if err == nil {
				published++
				continue
			}
			failures = append(failures, errors.New("mark outbox published"))
			continue
		}
		operation, cancel = context.WithTimeout(ctx, p.config.OperationTimeout)
		markErr := p.store.MarkFailed(operation, event, "publish_failed", now)
		cancel()
		failures = append(failures, errors.New("publish outbox event"))
		if markErr != nil {
			failures = append(failures, errors.New("mark outbox failed"))
		}
	}
	return published, errors.Join(failures...)
}
