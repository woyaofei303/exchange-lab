package eventing

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryOutbox struct {
	events    []OutboxEvent
	published []string
	failed    []string
}

func (m *memoryOutbox) Claim(context.Context, int, time.Duration) ([]OutboxEvent, error) {
	events := m.events
	m.events = nil
	return events, nil
}
func (m *memoryOutbox) MarkPublished(_ context.Context, id string, _ time.Time) error {
	m.published = append(m.published, id)
	return nil
}
func (m *memoryOutbox) MarkFailed(_ context.Context, event OutboxEvent, _ string, _ time.Time) error {
	m.failed = append(m.failed, event.ID)
	return nil
}

type memoryTransport struct {
	events []OutboxEvent
	fail   bool
}

func (m *memoryTransport) Publish(_ context.Context, event OutboxEvent) error {
	if m.fail {
		return errors.New("transport unavailable")
	}
	m.events = append(m.events, event)
	return nil
}

func TestEnvelopeValidationAndRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	event := OutboxEvent{ID: "out_12345678", AggregateType: "order", AggregateID: "ord_12345678", Subject: "orders.accepted", Version: 1, CorrelationID: "request-12345678", Payload: []byte(`{"order_id":"ord_12345678"}`), CreatedAt: now}
	payload, err := Encode(event)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ID != event.ID || decoded.Type != event.Subject || decoded.AggregateID != event.AggregateID || string(decoded.Data) != string(event.Payload) {
		t.Fatalf("envelope = %#v", decoded)
	}
	if _, err := Decode([]byte(`{"version":2}`)); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("Decode(invalid) error = %v", err)
	}
}

func TestPublisherMarksSuccessAndBoundsFailure(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	event := OutboxEvent{ID: "out_12345678", AggregateType: "order", AggregateID: "ord_12345678", Subject: "orders.accepted", Version: 1, CorrelationID: "request-12345678", Payload: []byte(`{"ok":true}`), CreatedAt: now}
	store := &memoryOutbox{events: []OutboxEvent{event}}
	transport := &memoryTransport{}
	publisher, err := NewPublisher(store, transport, PublisherConfig{BatchSize: 10, Lease: time.Second, OperationTimeout: time.Second, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if count, err := publisher.Tick(context.Background()); err != nil || count != 1 || len(store.published) != 1 || len(transport.events) != 1 {
		t.Fatalf("Tick() = %d, published=%v transport=%v error=%v", count, store.published, transport.events, err)
	}
	store.events = []OutboxEvent{event}
	transport.fail = true
	if count, err := publisher.Tick(context.Background()); err == nil || count != 0 || len(store.failed) != 1 {
		t.Fatalf("Tick(fail) = %d, failed=%v error=%v", count, store.failed, err)
	}
}
