// Package sse 提供进程内的 Server-Sent Events 发布/订阅 Hub。
package sse

import "sync"

const subscriberBuffer = 16

// Hub 向所有订阅者广播字节负载（已序列化的事件）。
type Hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

// NewHub 创建并返回一个新的 Hub 实例。
func NewHub() *Hub {
	return &Hub{subs: map[chan []byte]struct{}{}}
}

// Subscribe 注册一个订阅者，返回只读 channel 与取消函数。
// 取消函数幂等：移除订阅并关闭 channel。
func (h *Hub) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, subscriberBuffer)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, ch)
			h.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

// Publish 向所有订阅者非阻塞投递；订阅者缓冲已满则丢弃该消息（尽力推送）。
func (h *Hub) Publish(payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- payload:
		default:
		}
	}
}
