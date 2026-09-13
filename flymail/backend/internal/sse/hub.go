// Package sse 提供进程内的 Server-Sent Events 发布/订阅 Hub。
package sse

import "sync"

const (
	// eventBuffer 是**不可丢**事件（new_mail / notify）的每订阅者缓冲。
	eventBuffer = 16
	// progressBuffer 是**可丢**事件（同步进度）的每订阅者缓冲。
	// 它可以小：满了会挤掉最旧的一帧，而进度只有最新那帧有意义。
	progressBuffer = 8
)

// Hub 向所有订阅者广播字节负载（已序列化的事件）。
//
// ── 为什么要分两条队列 ───────────────────────────────────────────────────────
//
// 两类事件的丢弃代价完全不同：
//
//   - new_mail / notify：**丢了就没了**。丢 new_mail 的表现是「明明来了新邮件，
//     列表和未读数不动，手动刷新才出来」；丢 notify 是通知不弹、铃声不响。
//     而且用户无从察觉丢了什么。
//   - 同步进度：**只有最新一帧有意义**，中间帧丢光也只是进度条跳得粗一点。
//
// 同步进度的量远大于邮件事件——一轮全量同步是 2n+4 条（n 为可选文件夹数，
// 12 个文件夹就是 28 条），而一轮同步通常只有 0~n 条 new_mail。两者共用一条
// 16 槽的缓冲时，八个账户同时同步就能在同一窗口内推两百多条进度，
// 把邮件事件整个挤出去。
//
// 所以这里不是把缓冲调大（那只是把问题推后），而是把「这条能不能丢」这件事
// 交给调用方声明：Publish 不可丢，PublishProgress 可丢且后来居上。
type Hub struct {
	mu   sync.Mutex
	subs map[*subscriber]struct{}
}

type subscriber struct {
	events   chan []byte
	progress chan []byte
}

// Subscription 是一次订阅的两条出口与取消函数。
type Subscription struct {
	// Events 是不可丢事件。
	Events <-chan []byte
	// Progress 是可丢事件；缓冲满时最旧的一帧会被挤掉。
	Progress <-chan []byte
	// Cancel 幂等：移除订阅并关闭两条 channel。
	Cancel func()
}

// NewHub 创建并返回一个新的 Hub 实例。
func NewHub() *Hub {
	return &Hub{subs: map[*subscriber]struct{}{}}
}

// Subscribe 注册一个订阅者。
func (h *Hub) Subscribe() Subscription {
	s := &subscriber{
		events:   make(chan []byte, eventBuffer),
		progress: make(chan []byte, progressBuffer),
	}
	h.mu.Lock()
	h.subs[s] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, s)
			h.mu.Unlock()
			close(s.events)
			close(s.progress)
		})
	}
	return Subscription{Events: s.events, Progress: s.progress, Cancel: cancel}
}

// Publish 向所有订阅者非阻塞投递**不可丢**事件；缓冲已满仍会丢弃（尽力推送），
// 但它不再和进度事件抢同一格缓冲。
func (h *Hub) Publish(payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		select {
		case s.events <- payload:
		default:
		}
	}
}

// PublishProgress 投递**可丢**事件：缓冲满时挤掉最旧的一帧再放新的。
//
// 「后来居上」正是进度需要的语义——消费者慢的时候，该保住的是最新状态而不是
// 一串过期的中间帧。丢掉的那几帧对界面没有任何影响。
func (h *Hub) PublishProgress(payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		select {
		case s.progress <- payload:
			continue
		default:
		}
		// 满了：先挤掉最旧的一帧。
		// ⚠ 这里与消费者并发，drain 与 send 之间它可能又取走一条，
		// 于是第二次 send 仍可能失败——那时就丢掉这一帧。对进度无害，
		// 而为此上锁排队会让发布方（runner goroutine）被慢客户端拖住。
		select {
		case <-s.progress:
		default:
		}
		select {
		case s.progress <- payload:
		default:
		}
	}
}
