package sse

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// TicketTTL 是连接票据的有效期。
// 只需覆盖「取票 → 立刻 new EventSource」这一个来回，给到 60 秒是为了容忍
// 桌面端唤醒、标签页恢复这类被推迟的重连；再长就没有意义，票据的价值全在于短命。
const TicketTTL = 60 * time.Second

// TicketStore 是 SSE 连接票据的进程内存储：一次性、短时效、并发安全。
//
// 为什么需要它：浏览器原生 EventSource 无法设置 Authorization 头，凭据只能走 URL。
// 而 URL 会落进反向代理与浏览器的访问日志、也会留在历史记录里——把 access token
// 放进去，等于让一个能开整个账号的长期凭据散布到一堆不受控的地方。
// 票据把这条路径上的凭据换成「只能换一条 SSE 连接、60 秒后作废、用一次就没」的东西。
//
// 存储放在进程内而不是库里：本项目是单进程自托管形态，票据本身活不过一分钟，
// 重启后连接反正也要重建，落库只会换来一张迟早要清理的表。
type TicketStore struct {
	ttl time.Duration

	mu    sync.Mutex
	items map[string]time.Time // ticket → 过期时刻
	// now 可注入，供测试推进时间而不必真的等待。
	now func() time.Time
}

// NewTicketStore 创建票据存储，ttl 为票据有效期。
func NewTicketStore(ttl time.Duration) *TicketStore {
	return &TicketStore{ttl: ttl, items: map[string]time.Time{}, now: time.Now}
}

// Issue 签发一张新票据。
func (s *TicketStore) Issue() (string, error) {
	var buf [32]byte
	// 票据是纯凭据，可猜即等于无鉴权：只用密码学随机源，取不到就报错而不是退化。
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	t := hex.EncodeToString(buf[:])

	s.mu.Lock()
	defer s.mu.Unlock()
	// 顺带清一遍过期项：票据只在签发时增加，把回收挂在这里就不需要后台 goroutine
	// 和随之而来的生命周期管理（谁来 Stop、桌面形态怎么退出）。
	now := s.now()
	for k, exp := range s.items {
		if now.After(exp) {
			delete(s.items, k)
		}
	}
	s.items[t] = now.Add(s.ttl)
	return t, nil
}

// Consume 校验并立即作废票据；有效返回 true。
// 查找与删除在同一把锁内完成：分成「先查后删」两步的话，两个并发请求会同时看到
// 同一张票有效，一次性也就不成立了。
func (s *TicketStore) Consume(ticket string) bool {
	if ticket == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.items[ticket]
	if !ok {
		return false
	}
	delete(s.items, ticket)
	return !s.now().After(exp)
}

// len 返回当前留存的票据数（测试用）。
func (s *TicketStore) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

// NewTicketHandler 返回票据签发端点。
// ⚠ 必须挂在需要 Bearer 的受保护路由组下：它是把长期凭据换成短票的唯一入口，
// 一旦裸奔，SSE 端点就等于完全不鉴权。
func NewTicketHandler(store *TicketStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, err := store.Issue()
		if err != nil {
			http.Error(w, "ticket unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		// 票据本身就是凭据，不允许任何中间层缓存这个响应。
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ticket":     t,
			"expires_in": int(store.ttl / time.Second),
		})
	}
}
