package sync

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// capturePub 记录收到的不可丢事件（进度事件与本用例无关）。
type capturePub struct{ events [][]byte }

func (p *capturePub) Publish(payload []byte)         { p.events = append(p.events, payload) }
func (p *capturePub) PublishProgress(payload []byte) {}

// serveMailState 把 mailStateNotify 挂在一个返回 status 的假 handler 前面，
// 发一次带 X-Client-Id 的请求，返回被广播出去的事件。
func serveMailState(t *testing.T, status int, clientID string) (*capturePub, int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	pub := &capturePub{}
	svc := NewService(nil, nil, nil)
	svc.SetPublisher(pub)
	h := &handler{svc: svc}

	r := gin.New()
	g := r.Group("", h.mailStateNotify())
	g.POST("/op", func(c *gin.Context) { c.JSON(status, gin.H{"status": "x"}) })

	req := httptest.NewRequest(http.MethodPost, "/op", nil)
	if clientID != "" {
		req.Header.Set("X-Client-Id", clientID)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return pub, rec.Code
}

// TestMailStateBroadcastOnSuccess：写操作成功后广播一条 mail_state，并带上发起方标识。
// origin 是发起方用来忽略自己那条回声的唯一依据——丢了它，每次标已读都会让
// 发起方自己再白跑一轮失效（folders ×账户数 + 两个计数接口）。
func TestMailStateBroadcastOnSuccess(t *testing.T) {
	pub, code := serveMailState(t, http.StatusOK, "tab-A")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if len(pub.events) != 1 {
		t.Fatalf("广播 %d 条，want 1", len(pub.events))
	}
	var ev MailStateEvent
	if err := json.Unmarshal(pub.events[0], &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Type != "mail_state" || ev.Origin != "tab-A" {
		t.Fatalf("event = %+v, want type=mail_state origin=tab-A", ev)
	}
}

// TestMailStateSilentOnFailure：处理失败时不广播。
// 广播的语义是「本地状态变了，手里那份不作数」——请求失败时状态没变，
// 发出去只会让所有界面白重取一轮。
func TestMailStateSilentOnFailure(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusInternalServerError} {
		pub, _ := serveMailState(t, status, "tab-A")
		if len(pub.events) != 0 {
			t.Errorf("status %d 仍广播了 %d 条", status, len(pub.events))
		}
	}
}

// TestMailStateWithoutPublisher：没注入 Publisher（单测与嵌入式用法）时静默跳过，不 panic。
func TestMailStateWithoutPublisher(t *testing.T) {
	NewService(nil, nil, nil).PublishMailState("tab-A")
}

// TestMailStateOriginTruncated：超长的 X-Client-Id 被截断。
// origin 只参与一次相等比较，长一个字节都没用；不截断的话，一个畸形客户端能拿
// Go 默认允许的 1MB 请求头让每条事件都是 1MB，再乘订阅者数量扇出。
func TestMailStateOriginTruncated(t *testing.T) {
	long := strings.Repeat("x", 4096)
	pub, _ := serveMailState(t, http.StatusOK, long)
	if len(pub.events) != 1 {
		t.Fatalf("广播 %d 条，want 1", len(pub.events))
	}
	var ev MailStateEvent
	if err := json.Unmarshal(pub.events[0], &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(ev.Origin) != maxOriginLen {
		t.Fatalf("origin 长度 = %d，want %d", len(ev.Origin), maxOriginLen)
	}
}

// TestMailStateWithoutClientID：没有 X-Client-Id 时 origin 为空，事件照发。
// 空 origin 不会等于任何界面的 CLIENT_ID，于是所有界面（含发起方）都会重取——
// 多一轮请求，但不会漏掉刷新。
func TestMailStateWithoutClientID(t *testing.T) {
	pub, _ := serveMailState(t, http.StatusOK, "")
	if len(pub.events) != 1 {
		t.Fatalf("广播 %d 条，want 1", len(pub.events))
	}
	var ev MailStateEvent
	if err := json.Unmarshal(pub.events[0], &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Origin != "" {
		t.Fatalf("origin = %q, want 空", ev.Origin)
	}
}
