package sse

import (
	"fmt"
	"net/http"
	"time"
)

// NewHandler 返回 SSE 端点处理器。
//
// 鉴权用一次性连接票据（?ticket=），不再接受 access token：EventSource 设不了请求头，
// 凭据只能走 URL，而 URL 会留在代理日志与浏览器历史里——留在那里的必须是一张
// 一分钟后就作废的票，不能是能开整个账号的长期凭据。票据由 NewTicketHandler 在
// 受保护端点签发。
//
// consume 校验并作废票据（通常是 TicketStore.Consume），返回 false 时响应 401。
func NewHandler(hub *Hub, consume func(ticket string) bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 票据在握手时一次性核销。SSE 是长连接，连上之后不再复查：
		// 连接的存续期由 TCP 与 ctx 决定，与票据寿命无关。
		if !consume(r.URL.Query().Get("ticket")) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// 先订阅再写响应头：反过来的话，从客户端收到头到这里完成订阅之间推送的事件
		// 会静默丢失（前端刚连上就漏掉一封新邮件，且无从察觉）。
		sub := hub.Subscribe()
		defer sub.Cancel()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		heartbeat := time.NewTicker(25 * time.Second)
		defer heartbeat.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeat.C:
				if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
					return // 写失败说明连接已断，退出避免卡在 Flush。
				}
				flusher.Flush()
			case msg, open := <-sub.Events:
				if !open {
					return
				}
				if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
					return
				}
				flusher.Flush()
			case msg, open := <-sub.Progress:
				// 与 Events 分开一条 case：进度事件量远大于邮件事件，
				// 共用一条缓冲时会把后者挤掉（见 hub.go 的头注释）。
				if !open {
					return
				}
				if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}
