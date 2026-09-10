package sse

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestTicketIssueConsumeOnce：票据只能核销一次——第二次连接必须失败。
func TestTicketIssueConsumeOnce(t *testing.T) {
	s := NewTicketStore(time.Minute)
	tk, err := s.Issue()
	if err != nil {
		t.Fatal(err)
	}
	if tk == "" {
		t.Fatal("issued empty ticket")
	}
	if !s.Consume(tk) {
		t.Fatal("first consume should succeed")
	}
	if s.Consume(tk) {
		t.Fatal("ticket must be single-use")
	}
}

// TestTicketIssueUnique：两次签发不能撞号（随机源接错会静默退化成固定值）。
func TestTicketIssueUnique(t *testing.T) {
	s := NewTicketStore(time.Minute)
	seen := make(map[string]bool, 64)
	for i := 0; i < 64; i++ {
		tk, err := s.Issue()
		if err != nil {
			t.Fatal(err)
		}
		if seen[tk] {
			t.Fatalf("duplicate ticket %q at %d", tk, i)
		}
		seen[tk] = true
	}
}

// TestTicketConsumeExpired：过了 TTL 的票据一律拒绝，且不再占着内存。
func TestTicketConsumeExpired(t *testing.T) {
	s := NewTicketStore(30 * time.Second)
	now := time.Now()
	s.now = func() time.Time { return now }

	tk, err := s.Issue()
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(31 * time.Second)
	if s.Consume(tk) {
		t.Fatal("expired ticket must be rejected")
	}
	if n := s.len(); n != 0 {
		t.Fatalf("expired ticket left behind: len=%d", n)
	}
}

// TestTicketConsumeUnknown：空串与伪造值都不能通过（空串尤其重要：
// 前端漏传参数时 Consume("") 若返回 true，等于端点彻底不鉴权）。
func TestTicketConsumeUnknown(t *testing.T) {
	s := NewTicketStore(time.Minute)
	if s.Consume("") {
		t.Fatal("empty ticket must be rejected")
	}
	if s.Consume("deadbeef") {
		t.Fatal("forged ticket must be rejected")
	}
}

// TestTicketGCDropsExpired：签发时顺带清理过期项，票据表不会随重连次数无限增长。
func TestTicketGCDropsExpired(t *testing.T) {
	s := NewTicketStore(30 * time.Second)
	now := time.Now()
	s.now = func() time.Time { return now }

	for i := 0; i < 10; i++ {
		if _, err := s.Issue(); err != nil {
			t.Fatal(err)
		}
	}
	now = now.Add(31 * time.Second)
	if _, err := s.Issue(); err != nil {
		t.Fatal(err)
	}
	if n := s.len(); n != 1 {
		t.Fatalf("len=%d, want 1（只剩刚签发的那张）", n)
	}
}

// TestTicketConcurrentIssueConsume：并发签发/核销无竞态，且同一张票在并发核销下
// 只有一个赢家——「一次性」如果靠 map 读写两步实现，这里会露馅（用 -race 跑）。
func TestTicketConcurrentIssueConsume(t *testing.T) {
	s := NewTicketStore(time.Minute)

	const n = 50
	tickets := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tk, err := s.Issue()
			if err != nil {
				t.Error(err)
				return
			}
			tickets[i] = tk
		}(i)
	}
	wg.Wait()

	var mu sync.Mutex
	wins := 0
	for _, tk := range tickets {
		for j := 0; j < 4; j++ { // 每张票 4 个并发核销者
			wg.Add(1)
			go func(tk string) {
				defer wg.Done()
				if s.Consume(tk) {
					mu.Lock()
					wins++
					mu.Unlock()
				}
			}(tk)
		}
	}
	wg.Wait()

	if wins != n {
		t.Fatalf("成功核销次数 = %d, want %d（每张票恰好一次）", wins, n)
	}
}

// TestTicketHandlerIssues：签发端点返回票据与有效期，且该票据可用于连接。
// 端点本身的鉴权由受保护路由组（Bearer 中间件）负责，此处只验证契约。
func TestTicketHandlerIssues(t *testing.T) {
	s := NewTicketStore(time.Minute)
	rec := httptest.NewRecorder()
	NewTicketHandler(s)(rec, httptest.NewRequest(http.MethodPost, "/api/v1/events/ticket", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Ticket    string `json:"ticket"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if body.Ticket == "" {
		t.Fatal("empty ticket in response")
	}
	if body.ExpiresIn != 60 {
		t.Fatalf("expires_in = %d, want 60", body.ExpiresIn)
	}
	if !s.Consume(body.Ticket) {
		t.Fatal("issued ticket not accepted")
	}
}
