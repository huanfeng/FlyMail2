package sse

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// dial 起一个真实 HTTP 服务并按 query 连接 SSE 端点。
// 用真服务而不是 ResponseRecorder：长连接场景下 recorder 的 Body 会被 handler 协程
// 持续写入，测试再去读就是数据竞争（-race 必炸）。
func dial(t *testing.T, h http.HandlerFunc, query string) (*http.Response, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+query, nil)
	if err != nil {
		cancel()
		srv.Close()
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		srv.Close()
		t.Fatal(err)
	}
	return resp, func() {
		cancel()
		resp.Body.Close()
		srv.Close()
	}
}

// TestHandlerRejectsMissingTicket：没有票据一律 401（含空 ticket 参数）。
func TestHandlerRejectsMissingTicket(t *testing.T) {
	h := NewHandler(NewHub(), NewTicketStore(time.Minute).Consume)
	for _, q := range []string{"", "?ticket="} {
		resp, done := dial(t, h, q)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("query %q: status = %d, want 401", q, resp.StatusCode)
		}
		done()
	}
}

// TestHandlerRejectsAccessTokenQuery：不再接受 access_token query——KI-2 的要点就是
// 长期凭据不得出现在 URL 里，留一条兼容路径等于没修。
func TestHandlerRejectsAccessTokenQuery(t *testing.T) {
	h := NewHandler(NewHub(), NewTicketStore(time.Minute).Consume)
	resp, done := dial(t, h, "?access_token=any-access-token")
	defer done()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// TestHandlerAcceptsTicketOnce：有效票据连得上；同一张票再连必须被拒（握手时即核销）。
func TestHandlerAcceptsTicketOnce(t *testing.T) {
	store := NewTicketStore(time.Minute)
	h := NewHandler(NewHub(), store.Consume)

	tk, err := store.Issue()
	if err != nil {
		t.Fatal(err)
	}
	resp, done := dial(t, h, "?ticket="+tk)
	if resp.StatusCode != http.StatusOK {
		done()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q", ct)
	}
	done()

	resp2, done2 := dial(t, h, "?ticket="+tk)
	defer done2()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replay status = %d, want 401", resp2.StatusCode)
	}
}

// TestHandlerStreamsAfterTicketConsumed：票据在握手时核销，连接照常存续并继续收事件。
// SSE 是长连接，若把核销做成「连接期间反复校验」，推送会在第一条心跳后断掉。
func TestHandlerStreamsAfterTicketConsumed(t *testing.T) {
	store := NewTicketStore(time.Minute)
	hub := NewHub()
	h := NewHandler(hub, store.Consume)

	tk, err := store.Issue()
	if err != nil {
		t.Fatal(err)
	}
	resp, done := dial(t, h, "?ticket="+tk)
	defer done()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// 响应头到达即说明 handler 已完成订阅（实现里先 Subscribe 再写头），可以安全推送。
	hub.Publish([]byte(`{"type":"new_mail"}`))

	lines := make(chan string, 8)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()

	deadline := time.After(3 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("stream closed before event arrived")
			}
			if line == `data: {"type":"new_mail"}` {
				return
			}
		case <-deadline:
			t.Fatal("事件未在超时内送达")
		}
	}
}
