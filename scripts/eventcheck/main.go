// Command eventcheck verifies durable event delivery and idempotent effects.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/woyaofei303/exchange-lab/internal/eventing"
)

func main() {
	var eventID, orderID, correlation, natsURL string
	flag.StringVar(&eventID, "event-id", "", "stable event ID to redeliver")
	flag.StringVar(&orderID, "order-id", "", "order aggregate ID")
	flag.StringVar(&correlation, "correlation", "event-check", "correlation ID")
	flag.StringVar(&natsURL, "nats-url", "nats://127.0.0.1:4222", "NATS URL")
	flag.Parse()
	if eventID == "" || orderID == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "event check failed: event-id and order-id are required")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	connection, err := nats.Connect(natsURL, nats.Timeout(2*time.Second))
	if err != nil {
		fail()
	}
	defer connection.Close()
	js, err := jetstream.New(connection)
	if err != nil {
		fail()
	}
	stream, err := js.Stream(ctx, eventing.OrdersStream)
	if err != nil {
		fail()
	}
	consumer, err := stream.Consumer(ctx, eventing.AuditConsumer)
	if err != nil {
		fail()
	}
	info, err := consumer.Info(ctx)
	if err != nil || info.Config.AckPolicy != jetstream.AckExplicitPolicy || info.Config.MaxDeliver != 5 || info.Config.FilterSubject != "orders.>" {
		fail()
	}
	occurredAt := time.Now().UTC()
	data := []byte(fmt.Sprintf(`{"version":1,"order_id":%q,"user_id":1,"symbol":"BTCUSDT-PERP","side":"buy","type":"limit","status":"open","price":"1.00","quantity":"0.001","remaining":"0.001","leverage":1,"reserved_margin":"0.00100000","occurred_at":%q}`, orderID, occurredAt.Format(time.RFC3339Nano)))
	event := eventing.OutboxEvent{ID: eventID, AggregateType: "order", AggregateID: orderID, Subject: "orders.accepted", Version: 1, CorrelationID: correlation, Payload: data, CreatedAt: occurredAt}
	payload, err := eventing.Encode(event)
	if err != nil {
		fail()
	}
	for index := 1; index <= 2; index++ {
		message := &nats.Msg{Subject: event.Subject, Header: nats.Header{}, Data: payload}
		message.Header.Set(eventing.EventIDHeader, event.ID)
		message.Header.Set(eventing.DedupeHeader, fmt.Sprintf("%s-probe-%d-%d", event.ID, index, time.Now().UnixNano()))
		message.Header.Set(eventing.CorrelationHeader, event.CorrelationID)
		if _, err := js.PublishMsg(ctx, message); err != nil {
			fail()
		}
	}
	fmt.Println("event check probe: durable explicit-ack consumer inspected and duplicate event published")
}

func fail() {
	fmt.Fprintln(os.Stderr, "event check failed: JetStream contract mismatch")
	os.Exit(1)
}
