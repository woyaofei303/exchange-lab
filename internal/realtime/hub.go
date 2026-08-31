// Package realtime fans updates out without letting slow subscribers block peers.
package realtime

import (
	"context"
	"fmt"
	"sync"
)

// Hub owns bounded per-subscriber queues and removes slow consumers.
type Hub struct {
	mu          sync.Mutex
	bufferSize  int
	nextID      uint64
	subscribers map[uint64]chan []byte
}

// NewHub validates the per-subscriber queue capacity.
func NewHub(bufferSize int) (*Hub, error) {
	if bufferSize <= 0 || bufferSize > 1024 {
		return nil, fmt.Errorf("create realtime hub: invalid buffer size")
	}
	return &Hub{bufferSize: bufferSize, subscribers: make(map[uint64]chan []byte)}, nil
}

// Subscribe registers a subscription that remains active until explicitly closed.
func (h *Hub) Subscribe() *Subscription {
	return h.SubscribeContext(context.Background())
}

// SubscribeContext closes the subscription when its context ends.
func (h *Hub) SubscribeContext(ctx context.Context) *Subscription {
	h.mu.Lock()
	h.nextID++
	id := h.nextID
	messages := make(chan []byte, h.bufferSize)
	h.subscribers[id] = messages
	h.mu.Unlock()
	subscription := &Subscription{id: id, hub: h, messages: messages, closed: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			subscription.Close()
		case <-subscription.closedSignal():
		}
	}()
	return subscription
}

// Publish copies payload per subscriber and disconnects any full queue.
func (h *Hub) Publish(payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, messages := range h.subscribers {
		copyPayload := append([]byte(nil), payload...)
		select {
		case messages <- copyPayload:
		default:
			delete(h.subscribers, id)
			close(messages)
		}
	}
}

// SubscriberCount returns the current number of active queues.
func (h *Hub) SubscriberCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subscribers)
}

func (h *Hub) remove(id uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if messages, ok := h.subscribers[id]; ok {
		delete(h.subscribers, id)
		close(messages)
	}
}

// Subscription owns one receive-only update queue and idempotent cleanup.
type Subscription struct {
	id       uint64
	hub      *Hub
	messages chan []byte
	once     sync.Once
	closed   chan struct{}
}

// Messages returns the receive-only update stream.
func (s *Subscription) Messages() <-chan []byte {
	return s.messages
}

// Close removes and closes the subscription exactly once.
func (s *Subscription) Close() {
	s.once.Do(func() {
		if s.closed != nil {
			close(s.closed)
		}
		s.hub.remove(s.id)
	})
}

func (s *Subscription) closedSignal() <-chan struct{} {
	return s.closed
}
