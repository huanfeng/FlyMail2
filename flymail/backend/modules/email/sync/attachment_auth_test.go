package sync

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// serveAttachment 挂上 AttachmentHandler 并发一次请求，返回记录到的凭据来源。
// svc 传 nil：这些用例全部在鉴权阶段返回 401，不会走到取附件内容那一步。
func serveAttachment(t *testing.T, url string, header string) (gotToken string, gotFromQuery bool, code int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/messages/:id/attachments/:idx", AttachmentHandler(nil,
		func(token string, messageID uint, fromQuery bool) error {
			gotToken, gotFromQuery = token, fromQuery
			return errors.New("stop here: 用例只关心凭据取自哪里")
		}))

	req := httptest.NewRequest(http.MethodGet, url, nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return gotToken, gotFromQuery, rec.Code
}

// TestAttachmentCredentialFromQuery：URL 里的凭据参数叫 ticket，并被标记为「来自 URL」，
// 由校验方按更严的规则处理（只认限定单封的附件令牌）。
func TestAttachmentCredentialFromQuery(t *testing.T) {
	tok, fromQuery, code := serveAttachment(t, "/api/v1/messages/5/attachments/0?ticket=ATT", "")
	if tok != "ATT" || !fromQuery {
		t.Fatalf("token=%q fromQuery=%v, want \"ATT\" true", tok, fromQuery)
	}
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
}

// TestAttachmentAccessTokenQueryIgnored：旧的 ?access_token= 兼容路径已移除。
// 保留它等于让长期凭据继续出现在会被写进邮件文档的 URL 里（见 KI-2 的 CSS 外泄路径）。
func TestAttachmentAccessTokenQueryIgnored(t *testing.T) {
	tok, _, code := serveAttachment(t, "/api/v1/messages/5/attachments/0?access_token=LEAKY", "")
	if tok == "LEAKY" {
		t.Fatal("access_token query 仍被当作凭据读取")
	}
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", code)
	}
}

// TestAttachmentCredentialFromHeader：没有 query 时取 Bearer 头，并标记为「非 URL 来源」，
// 用户主动下载（axios blob）走的就是这条路，access token 只在这里被接受。
func TestAttachmentCredentialFromHeader(t *testing.T) {
	tok, fromQuery, _ := serveAttachment(t, "/api/v1/messages/5/attachments/0", "Bearer ACCESS")
	if tok != "ACCESS" || fromQuery {
		t.Fatalf("token=%q fromQuery=%v, want \"ACCESS\" false", tok, fromQuery)
	}
}

// TestAttachmentInvalidMessageID：路径参数非法时不进鉴权，直接 400。
func TestAttachmentInvalidMessageID(t *testing.T) {
	_, _, code := serveAttachment(t, "/api/v1/messages/abc/attachments/0?ticket=ATT", "")
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
}
