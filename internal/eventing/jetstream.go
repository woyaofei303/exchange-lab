package eventing

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Stream, consumer and header names are stable parts of the version-1 event contract.
const (
	OrdersStream      = "ORDERS"
	TradesStream      = "TRADES"
	AuditConsumer     = "order-audit-v1"
	EventIDHeader     = "X-Event-ID"
	DedupeHeader      = "Nats-Msg-Id"
	CorrelationHeader = "X-Correlation-ID"
)

// Configure creates bounded ORDERS/TRADES streams and the audit consumer.
func Configure(ctx context.Context, js jetstream.JetStream) (jetstream.Consumer, error) {
	if js == nil {
		return nil, fmt.Errorf("configure event streams: nil JetStream")
	}
	orders, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: OrdersStream, Subjects: []string{"orders.>"}, Retention: jetstream.LimitsPolicy,
		MaxMsgs: 1_000_000, MaxBytes: 1 << 30, MaxAge: 7 * 24 * time.Hour, MaxMsgSize: 1 << 20,
		Storage: jetstream.FileStorage, Replicas: 1, Duplicates: 2 * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("configure ORDERS stream: %w", err)
	}
	if _, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: TradesStream, Subjects: []string{"trades.>"}, Retention: jetstream.LimitsPolicy,
		MaxMsgs: 1_000_000, MaxBytes: 1 << 30, MaxAge: 7 * 24 * time.Hour, MaxMsgSize: 1 << 20,
		Storage: jetstream.FileStorage, Replicas: 1, Duplicates: 2 * time.Minute,
	}); err != nil {
		return nil, fmt.Errorf("configure TRADES stream: %w", err)
	}
	consumer, err := orders.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name: AuditConsumer, Durable: AuditConsumer, FilterSubject: "orders.>",
		AckPolicy: jetstream.AckExplicitPolicy, AckWait: 5 * time.Second, MaxDeliver: 5,
		BackOff:       []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second},
		MaxAckPending: 256,
	})
	if err != nil {
		return nil, fmt.Errorf("configure order audit consumer: %w", err)
	}
	return consumer, nil
}

// ConfigureMatching creates the single-inflight durable order consumer.
func ConfigureMatching(ctx context.Context, js jetstream.JetStream) (jetstream.Consumer, error) {
	if js == nil {
		return nil, fmt.Errorf("configure matching consumer: nil JetStream")
	}
	stream, err := js.Stream(ctx, OrdersStream)
	if err != nil {
		return nil, fmt.Errorf("configure matching consumer: ORDERS stream: %w", err)
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name: "matching-v1", Durable: "matching-v1", FilterSubject: "orders.>",
		AckPolicy: jetstream.AckExplicitPolicy, AckWait: 5 * time.Second, MaxDeliver: 5,
		BackOff:       []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second},
		MaxAckPending: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("configure matching consumer: %w", err)
	}
	return consumer, nil
}

// ConfigurePosition creates the single-inflight durable trade consumer.
func ConfigurePosition(ctx context.Context, js jetstream.JetStream) (jetstream.Consumer, error) {
	if js == nil {
		return nil, fmt.Errorf("configure position consumer: nil JetStream")
	}
	stream, err := js.Stream(ctx, TradesStream)
	if err != nil {
		return nil, fmt.Errorf("configure position consumer: TRADES stream: %w", err)
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name: "position-v1", Durable: "position-v1", FilterSubject: "trades.executed",
		AckPolicy: jetstream.AckExplicitPolicy, AckWait: 5 * time.Second, MaxDeliver: 5,
		BackOff: []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second}, MaxAckPending: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("configure position consumer: %w", err)
	}
	return consumer, nil
}

// NATSTransport publishes validated Outbox events through JetStream acknowledgements.
type NATSTransport struct {
	js jetstream.JetStream
}

// NewNATSTransport rejects a nil JetStream context.
func NewNATSTransport(js jetstream.JetStream) (*NATSTransport, error) {
	if js == nil {
		return nil, fmt.Errorf("create JetStream transport: nil context")
	}
	return &NATSTransport{js: js}, nil
}

// Publish sends one event with stable deduplication and correlation headers.
func (t *NATSTransport) Publish(ctx context.Context, event OutboxEvent) error {
	payload, err := Encode(event)
	if err != nil {
		return err
	}
	message := &nats.Msg{Subject: event.Subject, Header: nats.Header{}, Data: payload}
	message.Header.Set(EventIDHeader, event.ID)
	message.Header.Set(DedupeHeader, event.ID)
	message.Header.Set(CorrelationHeader, event.CorrelationID)
	if _, err := t.js.PublishMsg(ctx, message); err != nil {
		return fmt.Errorf("publish JetStream event: %w", err)
	}
	return nil
}
