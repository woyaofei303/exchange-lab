package realtime_test

import (
	"context"
	"testing"
	"time"

	"exchange-lab/internal/realtime"
)

func TestHubFanoutDoesNotBlockOnSlowSubscriber(t *testing.T) {
	t.Parallel()

	hub, err := realtime.NewHub(1)
	if err != nil {
		t.Fatal(err)
	}
	fast := hub.Subscribe()
	defer fast.Close()
	slow := hub.Subscribe()
	defer slow.Close()

	hub.Publish([]byte("first"))
	select {
	case got := <-fast.Messages():
		if string(got) != "first" {
			t.Fatalf("message = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("fast subscriber did not receive first message")
	}

	started := time.Now()
	hub.Publish([]byte("second"))
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("Publish() blocked for %s", elapsed)
	}
	if hub.SubscriberCount() != 1 {
		t.Fatalf("subscriber count = %d, want slow subscriber removed", hub.SubscriberCount())
	}
	select {
	case got := <-fast.Messages():
		if string(got) != "second" {
			t.Fatalf("message = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("fast subscriber did not receive second message")
	}
}

func TestSubscriptionClosesOnContextCancellation(t *testing.T) {
	t.Parallel()

	hub, err := realtime.NewHub(2)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	subscription := hub.SubscribeContext(ctx)
	cancel()
	select {
	case _, ok := <-subscription.Messages():
		if ok {
			t.Fatal("subscription channel remains open")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription was not closed")
	}
}
