// Package realtime 扇出更新，同时防止慢订阅者阻塞其他订阅者。
package realtime

import (
	"context"
	"fmt"
	"sync"
)

// Hub 管理每个订阅者的有界队列，并移除慢消费者。
type Hub struct {
	mu          sync.Mutex
	bufferSize  int
	nextID      uint64
	subscribers map[uint64]chan []byte
}

// NewHub 校验每个订阅者的队列容量。
func NewHub(bufferSize int) (*Hub, error) {
	if bufferSize <= 0 || bufferSize > 1024 {
		return nil, fmt.Errorf("create realtime hub: invalid buffer size")
	}
	return &Hub{bufferSize: bufferSize, subscribers: make(map[uint64]chan []byte)}, nil
}

// Subscribe 注册一个持续有效直至明确关闭的订阅。
func (h *Hub) Subscribe() *Subscription {
	return h.SubscribeContext(context.Background())
}

// SubscribeContext 在上下文结束时关闭订阅。
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

// Publish 为每个订阅者复制载荷，并断开队列已满的订阅者。
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

// SubscriberCount 返回当前有效队列数量。
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

// Subscription 管理一个只接收更新队列和幂等清理操作。
type Subscription struct {
	id       uint64
	hub      *Hub
	messages chan []byte
	once     sync.Once
	closed   chan struct{}
}

// Messages 返回只接收更新流。
func (s *Subscription) Messages() <-chan []byte {
	return s.messages
}

// Close 仅执行一次订阅移除和关闭操作。
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
