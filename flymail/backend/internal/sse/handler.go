package sse

import (
	"fmt"
	"net/http"
	"time"
)

// NewHandler 返回 SSE 端点处理器。
// verify 校验 access_token 查询参数，返回非 nil 错误时响应 401。
func NewHandler(hub *Hub, verify func(token string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("access_token")
		if err := verify(token); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ch, cancel := hub.Subscribe()
		defer cancel()

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
			case msg, open := <-ch:
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
