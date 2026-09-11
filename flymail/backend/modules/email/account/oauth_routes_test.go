package account_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flymail/modules/email/account"

	"github.com/gin-gonic/gin"
)

// TestOAuthRoutes_NoConflictWithIDParam 新增的 /accounts/oauth/* 与既有的 /accounts/:id
// 处在同一层级。gin 的路由树允许静态段与通配符共存，但这依赖具体版本的实现——
// 一旦不兼容，注册时就会 panic，整个服务起不来。这条用例把它钉死在编译期之外的第一道关。
func TestOAuthRoutes_NoConflictWithIDParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, _ := newSvc(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("路由注册 panic（很可能是 /accounts/oauth 与 /accounts/:id 冲突）: %v", r)
		}
	}()

	r := gin.New()
	account.RegisterRoutes(&r.RouterGroup, svc)

	// 静态段必须优先于 :id 匹配：否则 "oauth" 会被当成账户 ID。
	req := httptest.NewRequest(http.MethodGet, "/accounts/oauth/providers", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("providers 返回 %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "google") {
		t.Fatalf("响应应包含提供方清单: %s", w.Body.String())
	}

	// :id 路由不受影响。
	req = httptest.NewRequest(http.MethodGet, "/accounts/123", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound && strings.Contains(w.Body.String(), "404 page not found") {
		t.Fatal("/accounts/:id 路由被破坏")
	}
}

// TestOAuthStart_NotConfigured 未配置凭据时返回 501，与「请求参数错误」区分开，
// 前端据此提示部署方去补配置而不是让用户反复重试。
func TestOAuthStart_NotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, _ := newSvc(t)

	r := gin.New()
	account.RegisterRoutes(&r.RouterGroup, svc)

	req := httptest.NewRequest(http.MethodPost, "/accounts/oauth/start",
		strings.NewReader(`{"provider":"google"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("状态码 = %d，期望 501: %s", w.Code, w.Body.String())
	}
}

// TestOAuthFlowStatus_Unknown 未知流程返回 404 而不是 5xx。
func TestOAuthFlowStatus_Unknown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, _ := newSvc(t)

	r := gin.New()
	account.RegisterRoutes(&r.RouterGroup, svc)

	req := httptest.NewRequest(http.MethodGet, "/accounts/oauth/flows/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404", w.Code)
	}
}
