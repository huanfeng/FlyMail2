package notify_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"flymail/modules/system/notify"
)

func newRouter(t *testing.T) (*gin.Engine, *notify.Service) {
	t.Helper()
	svc, _ := newSvc(t)
	svc.SetDispatcher(func(*notify.Channel, notify.Event) error { return nil })
	gin.SetMode(gin.TestMode)
	r := gin.New()
	grp := r.Group("")
	notify.RegisterRoutes(grp, svc)
	return r, svc
}

func do(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandlerChannelCRUD(t *testing.T) {
	r, _ := newRouter(t)

	// 创建
	w := do(r, http.MethodPost, "/notify/channels",
		`{"name":"wh","kind":"webhook","url":"http://x","events":["mail_new"],"enabled":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("create code=%d body=%s", w.Code, w.Body.String())
	}
	var created notify.ChannelDTO
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.ID == 0 || created.Name != "wh" || len(created.Events) != 1 {
		t.Fatalf("created 错误: %+v", created)
	}

	// 列表
	w = do(r, http.MethodGet, "/notify/channels", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "\"wh\"") {
		t.Fatalf("list code=%d body=%s", w.Code, w.Body.String())
	}

	// 非法创建（缺 url）→ 400
	w = do(r, http.MethodPost, "/notify/channels", `{"name":"x","kind":"webhook"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("非法创建应 400，得 %d", w.Code)
	}

	// 测试投递（dispatcher mock 成功）→ 200
	w = do(r, http.MethodPost, "/notify/channels/1/test", "")
	if w.Code != http.StatusOK {
		t.Errorf("test code=%d body=%s", w.Code, w.Body.String())
	}

	// 删除 → 200，列表为空
	w = do(r, http.MethodDelete, "/notify/channels/1", "")
	if w.Code != http.StatusOK {
		t.Errorf("delete code=%d", w.Code)
	}
	w = do(r, http.MethodGet, "/notify/channels", "")
	if strings.Contains(w.Body.String(), "\"wh\"") {
		t.Errorf("删除后列表不应再含该渠道: %s", w.Body.String())
	}
}

func TestHandlerNotificationsReadAll(t *testing.T) {
	r, svc := newRouter(t)
	svc.Emit(notify.Event{Type: notify.EventSyncFailed, Title: "失败1"})
	svc.Emit(notify.Event{Type: notify.EventSyncFailed, Title: "失败2"})

	w := do(r, http.MethodGet, "/notifications", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list code=%d", w.Code)
	}
	var resp struct {
		Notifications []notify.Notification `json:"notifications"`
		UnreadCount   int                   `json:"unread_count"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Notifications) != 2 || resp.UnreadCount != 2 {
		t.Fatalf("应有 2 条未读: %+v", resp)
	}

	// 全部已读
	if w = do(r, http.MethodPost, "/notifications/read-all", ""); w.Code != http.StatusOK {
		t.Fatalf("read-all code=%d", w.Code)
	}
	w = do(r, http.MethodGet, "/notifications", "")
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.UnreadCount != 0 {
		t.Errorf("全部已读后未读应为 0，得 %d", resp.UnreadCount)
	}
}
