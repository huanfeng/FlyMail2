package account

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"flymail/internal/oauth"
)

// idToken 拼一个只有载荷有意义的 id_token。
func idToken(email string) string {
	payload, _ := json.Marshal(map[string]string{"email": email})
	return "h." + base64.RawURLEncoding.EncodeToString(payload) + ".s"
}

// driveCallback 模拟浏览器访问 loopback 回调地址，完成授权码流程的用户侧动作。
func driveCallback(t *testing.T, authURL, code string) {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("授权地址不合法: %v", err)
	}
	q := u.Query()
	redirect := q.Get("redirect_uri")
	state := q.Get("state")
	if redirect == "" || state == "" {
		t.Fatalf("授权地址缺少 redirect_uri/state: %s", authURL)
	}
	resp, err := http.Get(redirect + "?state=" + url.QueryEscape(state) + "&code=" + url.QueryEscape(code))
	if err != nil {
		t.Fatalf("回调失败: %v", err)
	}
	resp.Body.Close()
}

// waitFlow 轮询直到流程离开 pending，超时即失败。
func waitFlow(t *testing.T, svc *Service, flowID string) *OAuthFlowStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st, err := svc.OAuthFlowStatus(flowID)
		if err != nil {
			t.Fatalf("查询流程失败: %v", err)
		}
		if st.Status != FlowPending {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待授权结果超时")
	return nil
}

// TestOAuthFlow_CodeCreatesAccount 授权码流程的完整闭环：发起 → 浏览器回调 →
// 换取令牌 → 用提供方预设建号，用户无需填写任何服务器地址。
func TestOAuthFlow_CodeCreatesAccount(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(form url.Values, w http.ResponseWriter) {
		if form.Get("code") != "the-code" {
			t.Errorf("授权码 = %q", form.Get("code"))
		}
		if form.Get("code_verifier") == "" {
			t.Error("必须提交 PKCE verifier")
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "refresh_token": "rt", "expires_in": 3600,
			"id_token": idToken("new@gmail.com"),
		})
	}
	svc, repo := newOAuthSvc(t, idp)

	start, err := svc.StartOAuth(StartOAuthRequest{Provider: oauth.ProviderGoogle, Email: "new@gmail.com"})
	if err != nil {
		t.Fatalf("发起授权失败: %v", err)
	}
	if start.Mode != FlowModeCode {
		t.Fatalf("默认应走授权码流程，实际 %q", start.Mode)
	}
	if !strings.Contains(start.AuthURL, "127.0.0.1") {
		t.Fatalf("回调地址应绑定回环: %s", start.AuthURL)
	}

	driveCallback(t, start.AuthURL, "the-code")
	if st := waitFlow(t, svc, start.FlowID); st.Status != FlowSuccess {
		t.Fatalf("流程状态 = %q，错误: %s", st.Status, st.Error)
	}

	resp, err := svc.CompleteOAuth(CompleteOAuthRequest{FlowID: start.FlowID})
	if err != nil {
		t.Fatalf("建号失败: %v", err)
	}
	if resp.Email != "new@gmail.com" {
		t.Fatalf("邮箱 = %q", resp.Email)
	}
	// 名称留空时取邮箱本地部分。
	if resp.Name != "new" {
		t.Fatalf("显示名 = %q，期望取邮箱本地部分", resp.Name)
	}
	if resp.AuthType != AuthTypeOAuth {
		t.Fatalf("认证方式 = %q", resp.AuthType)
	}
	// 服务器地址来自提供方预设，用户不必手填。
	if resp.IMAPHost != "imap.gmail.com" || resp.SMTPHost != "smtp.gmail.com" {
		t.Fatalf("服务器预设未生效: %+v", resp)
	}

	got, err := repo.GetByID(resp.ID)
	if err != nil {
		t.Fatalf("读账户失败: %v", err)
	}
	if got.OAuthTokenEnc == "" || got.OAuthProvider != oauth.ProviderGoogle {
		t.Fatalf("令牌未正确落库: %+v", got)
	}

	// 同一次授权不得被重复消费去建第二个账户。
	if _, err := svc.CompleteOAuth(CompleteOAuthRequest{FlowID: start.FlowID}); err == nil {
		t.Fatal("流程消费后应失效")
	}
}

// TestOAuthFlow_DeviceMode 设备码流程：先给用户短码，再轮询到授权完成。
func TestOAuthFlow_DeviceMode(t *testing.T) {
	polled := false
	idp := newFakeIDP(t)
	idp.handler = func(form url.Values, w http.ResponseWriter) {
		if !form.Has("device_code") && !form.Has("grant_type") {
			json.NewEncoder(w).Encode(map[string]any{
				"device_code": "dc", "user_code": "ABCD-EFGH",
				"verification_uri": "https://microsoft.com/devicelogin",
				"expires_in":       900, "interval": 1,
			})
			return
		}
		if !polled {
			polled = true
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": "authorization_pending"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "refresh_token": "rt", "expires_in": 3600,
			"id_token": idToken("me@outlook.com"),
		})
	}
	svc, _ := newOAuthSvc(t, idp)

	start, err := svc.StartOAuth(StartOAuthRequest{
		Provider: oauth.ProviderMicrosoft, Mode: FlowModeDevice,
	})
	if err != nil {
		t.Fatalf("发起设备码失败: %v", err)
	}
	if start.UserCode != "ABCD-EFGH" || start.VerificationURI == "" {
		t.Fatalf("未返回用户码与验证地址: %+v", start)
	}
	if start.AuthURL != "" {
		t.Error("设备码流程不应返回 auth_url")
	}

	// interval=1 秒，首轮 pending 后第二轮成功。
	deadline := time.Now().Add(6 * time.Second)
	var st *OAuthFlowStatus
	for time.Now().Before(deadline) {
		st, _ = svc.OAuthFlowStatus(start.FlowID)
		if st.Status != FlowPending {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if st.Status != FlowSuccess {
		t.Fatalf("流程状态 = %q，错误: %s", st.Status, st.Error)
	}

	resp, err := svc.CompleteOAuth(CompleteOAuthRequest{FlowID: start.FlowID, Name: "工作邮箱"})
	if err != nil {
		t.Fatalf("建号失败: %v", err)
	}
	if resp.Email != "me@outlook.com" || resp.Name != "工作邮箱" {
		t.Fatalf("账户信息不符: %+v", resp)
	}
}

// TestOAuthFlow_DeviceUnsupported Google 不支持设备码，应在发起阶段就拒绝。
func TestOAuthFlow_DeviceUnsupported(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) {}
	svc, _ := newOAuthSvc(t, idp)

	if _, err := svc.StartOAuth(StartOAuthRequest{
		Provider: oauth.ProviderGoogle, Mode: FlowModeDevice,
	}); err == nil {
		t.Fatal("Google 应拒绝设备码流程")
	}
}

// TestOAuthFlow_NotConfigured 未配置凭据时不应放行，否则用户会被导到一个必然失败的授权页。
func TestOAuthFlow_NotConfigured(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) {}
	svc, _ := newOAuthSvc(t, idp)
	svc.SetOAuthSettings(OAuthSettings{})

	if _, err := svc.StartOAuth(StartOAuthRequest{Provider: oauth.ProviderGoogle}); err != ErrOAuthNotConfigured {
		t.Fatalf("期望 ErrOAuthNotConfigured，实际 %v", err)
	}
}

// TestOAuthFlow_UserDenied 用户在授权页点了拒绝。
func TestOAuthFlow_UserDenied(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) { t.Error("被拒绝时不应换取令牌") }
	svc, _ := newOAuthSvc(t, idp)

	start, err := svc.StartOAuth(StartOAuthRequest{Provider: oauth.ProviderGoogle})
	if err != nil {
		t.Fatalf("发起授权失败: %v", err)
	}
	u, _ := url.Parse(start.AuthURL)
	q := u.Query()
	resp, err := http.Get(q.Get("redirect_uri") + "?state=" + url.QueryEscape(q.Get("state")) + "&error=access_denied")
	if err != nil {
		t.Fatalf("回调失败: %v", err)
	}
	resp.Body.Close()

	st := waitFlow(t, svc, start.FlowID)
	if st.Status != FlowFailed {
		t.Fatalf("状态 = %q，期望 failed", st.Status)
	}
	if st.Error == "" {
		t.Fatal("失败原因不应为空")
	}
	// 未成功的流程不能用来建号。
	if _, err := svc.CompleteOAuth(CompleteOAuthRequest{FlowID: start.FlowID}); err == nil {
		t.Fatal("失败的流程不应能建号")
	}
}

// TestOAuthFlow_Reauthorize 重新授权走的是更新既有账户，而不是再建一个号。
func TestOAuthFlow_Reauthorize(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(form url.Values, w http.ResponseWriter) {
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "fresh-at", "refresh_token": "fresh-rt", "expires_in": 3600,
			"id_token": idToken("me@gmail.com"),
		})
	}
	svc, repo := newOAuthSvc(t, idp)
	a := seedOAuthAccount(t, svc, repo, time.Now().Add(time.Hour))
	if err := repo.UpdateFields(a.ID, map[string]any{"status": StatusNeedsReauth}); err != nil {
		t.Fatalf("置状态失败: %v", err)
	}

	start, err := svc.StartOAuth(StartOAuthRequest{Provider: oauth.ProviderGoogle, AccountID: a.ID})
	if err != nil {
		t.Fatalf("发起重新授权失败: %v", err)
	}
	driveCallback(t, start.AuthURL, "c")
	waitFlow(t, svc, start.FlowID)

	resp, err := svc.CompleteOAuth(CompleteOAuthRequest{FlowID: start.FlowID})
	if err != nil {
		t.Fatalf("重新授权失败: %v", err)
	}
	if resp.ID != a.ID {
		t.Fatalf("应更新既有账户 %d，实际 %d", a.ID, resp.ID)
	}
	if resp.Status != StatusOK {
		t.Fatalf("状态应恢复，实际 %q", resp.Status)
	}
	got, _ := repo.GetByID(a.ID)
	tok, _ := svc.loadToken(got)
	if tok.AccessToken != "fresh-at" {
		t.Fatalf("新令牌未写入，实际 %q", tok.AccessToken)
	}

	list, _ := repo.List()
	if len(list) != 1 {
		t.Fatalf("不应新建账户，现有 %d 个", len(list))
	}
}

// TestOAuthFlow_ReauthorizeWrongMailbox 用户在服务商页面上选错账号时必须拦住：
// 否则这个账户会顶着 A 的地址去同步 B 的邮箱。
func TestOAuthFlow_ReauthorizeWrongMailbox(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(form url.Values, w http.ResponseWriter) {
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "refresh_token": "rt", "expires_in": 3600,
			"id_token": idToken("someone-else@gmail.com"),
		})
	}
	svc, repo := newOAuthSvc(t, idp)
	a := seedOAuthAccount(t, svc, repo, time.Now().Add(time.Hour))

	start, err := svc.StartOAuth(StartOAuthRequest{Provider: oauth.ProviderGoogle, AccountID: a.ID})
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	driveCallback(t, start.AuthURL, "c")
	waitFlow(t, svc, start.FlowID)

	if _, err := svc.CompleteOAuth(CompleteOAuthRequest{FlowID: start.FlowID}); err == nil {
		t.Fatal("邮箱不一致时应拒绝")
	}
	// 原令牌必须原封不动。
	got, _ := repo.GetByID(a.ID)
	tok, _ := svc.loadToken(got)
	if tok.AccessToken != "old-at" {
		t.Fatalf("原令牌被改写为 %q", tok.AccessToken)
	}
}

// TestOAuthFlow_ReauthorizeMissingAccount 账户不存在时应在发起阶段失败，
// 而不是等用户授权完才发现无处安放。
func TestOAuthFlow_ReauthorizeMissingAccount(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) {}
	svc, _ := newOAuthSvc(t, idp)

	if _, err := svc.StartOAuth(StartOAuthRequest{
		Provider: oauth.ProviderGoogle, AccountID: 9999,
	}); err != ErrAccountNotFound {
		t.Fatalf("期望 ErrAccountNotFound，实际 %v", err)
	}
}

// TestOAuthFlow_Cancel 取消后流程消失，占用的本地端口随之释放。
func TestOAuthFlow_Cancel(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) {}
	svc, _ := newOAuthSvc(t, idp)

	start, err := svc.StartOAuth(StartOAuthRequest{Provider: oauth.ProviderGoogle})
	if err != nil {
		t.Fatalf("发起失败: %v", err)
	}
	u, _ := url.Parse(start.AuthURL)
	redirect := u.Query().Get("redirect_uri")

	svc.CancelOAuthFlow(start.FlowID)
	if _, err := svc.OAuthFlowStatus(start.FlowID); err == nil {
		t.Fatal("取消后不应还能查到流程")
	}
	if resp, err := http.Get(redirect); err == nil {
		resp.Body.Close()
		t.Fatal("取消后本地回调端口应已释放")
	}
}

// TestOAuthFlow_UnknownFlow 未知流程 ID 不应 panic。
func TestOAuthFlow_UnknownFlow(t *testing.T) {
	svc := &Service{}
	if _, err := svc.OAuthFlowStatus("nope"); err == nil {
		t.Fatal("未知流程应报错")
	}
	if _, err := svc.CompleteOAuth(CompleteOAuthRequest{FlowID: "nope"}); err == nil {
		t.Fatal("未知流程应报错")
	}
	svc.CancelOAuthFlow("nope")
}

// TestOAuthProviders 提供方清单要如实反映配置状态，让前端能把未配置的入口置灰。
func TestOAuthProviders(t *testing.T) {
	svc := &Service{}
	svc.SetOAuthSettings(OAuthSettings{GoogleClientID: "cid"})

	list := svc.OAuthProviders()
	if len(list) != 2 {
		t.Fatalf("应返回两个提供方，实际 %d", len(list))
	}
	byID := map[string]ProviderInfo{}
	for _, p := range list {
		byID[p.ID] = p
	}
	if !byID[oauth.ProviderGoogle].Configured {
		t.Error("Google 应为已配置")
	}
	if byID[oauth.ProviderMicrosoft].Configured {
		t.Error("Microsoft 应为未配置")
	}
	if byID[oauth.ProviderGoogle].DeviceCode {
		t.Error("Google 不支持设备码")
	}
	if !byID[oauth.ProviderMicrosoft].DeviceCode {
		t.Error("Microsoft 支持设备码")
	}
}
