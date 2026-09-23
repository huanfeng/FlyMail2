package account

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"flymail/internal/oauth"
)

// newFixedCallbackSvc 构建一个走固定回调地址（而非 loopback）的 Service。
func newFixedCallbackSvc(t *testing.T, idp *fakeIDP) (*Service, *Repository) {
	t.Helper()
	svc, repo := newOAuthSvc(t, idp)
	svc.SetOAuthSettings(OAuthSettings{
		GoogleClientID:  "cid",
		RedirectBaseURL: "https://mail.example.com",
	})
	return svc, repo
}

// startFixed 发起一次固定回调模式的授权，返回流程与授权地址里的 state。
func startFixed(t *testing.T, svc *Service) (*StartOAuthResponse, string) {
	t.Helper()
	start, err := svc.StartOAuth(StartOAuthRequest{Provider: oauth.ProviderGoogle})
	if err != nil {
		t.Fatalf("发起授权失败: %v", err)
	}
	u, err := url.Parse(start.AuthURL)
	if err != nil {
		t.Fatalf("授权地址不合法: %v", err)
	}
	return start, u.Query().Get("state")
}

// TestFixedCallback_UsesPublicRedirect 配置了公开地址后，回调必须指向后端自身的端点，
// 而不是浏览器根本连不上的服务端回环地址——这是远程部署能用 OAuth 的前提。
func TestFixedCallback_UsesPublicRedirect(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) {}
	svc, _ := newFixedCallbackSvc(t, idp)

	start, _ := startFixed(t, svc)
	u, _ := url.Parse(start.AuthURL)
	redirect := u.Query().Get("redirect_uri")

	if strings.Contains(redirect, "127.0.0.1") {
		t.Fatalf("固定回调模式不应使用回环地址: %s", redirect)
	}
	// 硬编码而不是复述 CallbackPath 的拼法：这个值要与登记在服务商后台的那个
	// 逐字节一致，用实现里的常量拼出期望值，等于实现怎么错测试就怎么跟着错。
	want := "https://mail.example.com/api/v1/accounts/oauth/callback"
	if redirect != want {
		t.Fatalf("回调地址 = %q，期望 %q", redirect, want)
	}
}

// TestFixedCallback_Success 回调打到后端后应完成令牌交换并推进流程。
func TestFixedCallback_Success(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(form url.Values, w http.ResponseWriter) {
		if form.Get("code_verifier") == "" {
			t.Error("必须提交 PKCE verifier")
		}
		if form.Get("redirect_uri") != "https://mail.example.com/api/v1/accounts/oauth/callback" {
			t.Errorf("交换时的 redirect_uri 必须与授权时一致，实际 %q", form.Get("redirect_uri"))
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "refresh_token": "rt", "expires_in": 3600,
			"id_token": idToken("me@gmail.com"),
		})
	}
	svc, _ := newFixedCallbackSvc(t, idp)
	start, state := startFixed(t, svc)

	if err := svc.HandleCallback(state, "the-code", "", ""); err != nil {
		t.Fatalf("处理回调失败: %v", err)
	}
	st, err := svc.OAuthFlowStatus(start.FlowID)
	if err != nil {
		t.Fatalf("查询流程失败: %v", err)
	}
	if st.Status != FlowSuccess {
		t.Fatalf("状态 = %q，错误: %s", st.Status, st.Error)
	}
	if st.Email != "me@gmail.com" {
		t.Fatalf("邮箱 = %q", st.Email)
	}
}

// TestFixedCallback_StateIsSingleUse state 用过即弃：否则截获回调 URL 的人可以重放，
// 拿同一个授权码反复换令牌。
func TestFixedCallback_StateIsSingleUse(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(form url.Values, w http.ResponseWriter) {
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "expires_in": 3600, "id_token": idToken("me@gmail.com"),
		})
	}
	svc, _ := newFixedCallbackSvc(t, idp)
	_, state := startFixed(t, svc)

	if err := svc.HandleCallback(state, "c", "", ""); err != nil {
		t.Fatalf("首次回调应成功: %v", err)
	}
	if err := svc.HandleCallback(state, "c", "", ""); err == nil {
		t.Fatal("同一个 state 不应被二次使用")
	}
}

// TestFixedCallback_UnknownState 伪造或过期的 state 必须被拒绝。
func TestFixedCallback_UnknownState(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) { t.Error("state 未通过时不应换取令牌") }
	svc, _ := newFixedCallbackSvc(t, idp)
	startFixed(t, svc)

	if err := svc.HandleCallback("forged", "c", "", ""); err == nil {
		t.Fatal("未知 state 应被拒绝")
	}
	if err := svc.HandleCallback("", "c", "", ""); err == nil {
		t.Fatal("空 state 应被拒绝")
	}
}

// TestFixedCallback_UserDenied 用户拒绝授权时流程应记录原因而不是一直 pending。
func TestFixedCallback_UserDenied(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) { t.Error("被拒绝时不应换取令牌") }
	svc, _ := newFixedCallbackSvc(t, idp)
	start, state := startFixed(t, svc)

	if err := svc.HandleCallback(state, "", "access_denied", "用户取消了授权"); err == nil {
		t.Fatal("应返回错误")
	}
	st, _ := svc.OAuthFlowStatus(start.FlowID)
	if st.Status != FlowFailed {
		t.Fatalf("状态 = %q，期望 failed", st.Status)
	}
	if !strings.Contains(st.Error, "用户取消了授权") {
		t.Fatalf("应保留服务商给出的原因，实际 %q", st.Error)
	}
}

// TestFixedCallback_MissingCode 回调既没有 code 也没有 error 时判为失败。
func TestFixedCallback_MissingCode(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) { t.Error("无授权码时不应换取令牌") }
	svc, _ := newFixedCallbackSvc(t, idp)
	start, state := startFixed(t, svc)

	if err := svc.HandleCallback(state, "", "", ""); err == nil {
		t.Fatal("缺少授权码应报错")
	}
	st, _ := svc.OAuthFlowStatus(start.FlowID)
	if st.Status != FlowFailed {
		t.Fatalf("状态 = %q，期望 failed", st.Status)
	}
}

// TestFixedCallback_NoLoopbackPortOpened 固定回调模式下不应再占用本地端口。
func TestFixedCallback_NoLoopbackPortOpened(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) {}
	svc, _ := newFixedCallbackSvc(t, idp)

	start, _ := startFixed(t, svc)
	v, _ := svc.flows.Load(start.FlowID)
	if f := v.(*oauthFlow); f.loopback != nil {
		t.Fatal("固定回调模式不应启动 loopback 监听")
	}
}
