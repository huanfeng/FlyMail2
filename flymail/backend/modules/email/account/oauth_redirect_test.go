package account_test

import (
	"testing"

	"flymail/modules/email/account"
)

// TestRedirectURIIncludesAPIPrefix 钉住必须登记到服务商后台的那个地址。
//
// 期望值硬编码，不由 CallbackPath 拼出来：这个值要与 OAuth 应用后台里登记的
// 那一条逐字节一致，用实现里的常量拼期望值等于实现怎么错测试就怎么跟着错。
// 此前它漏了 /api/v1（回调路由实际挂在 api 分组下），服务商把浏览器重定向到一个
// 后端根本没有的路径，授权码永远送不回来——而 FlyMail 这侧一条日志都没有，
// 因为那个请求压根没到后端。
func TestRedirectURIIncludesAPIPrefix(t *testing.T) {
	svc, _, _ := newSvc(t)
	svc.SetOAuthSettings(account.OAuthSettings{
		GoogleClientID:  "cid",
		RedirectBaseURL: "https://mail.example.com",
	})
	const want = "https://mail.example.com/api/v1/accounts/oauth/callback"
	if got := svc.RedirectURI(); got != want {
		t.Fatalf("RedirectURI() = %q, want %q", got, want)
	}
}

// TestRedirectURITrimsTrailingSlash：填成 https://host/ 也要拼出同一个地址。
// 多一个斜杠，服务商就报 redirect_uri_mismatch。
func TestRedirectURITrimsTrailingSlash(t *testing.T) {
	svc, _, _ := newSvc(t)
	svc.SetOAuthSettings(account.OAuthSettings{RedirectBaseURL: "https://mail.example.com/"})
	const want = "https://mail.example.com/api/v1/accounts/oauth/callback"
	if got := svc.RedirectURI(); got != want {
		t.Fatalf("RedirectURI() = %q, want %q", got, want)
	}
}

// TestRedirectURIEmptyMeansLoopback：没配对外地址时返回空串。
// 空串是「走 loopback」的信号，设置页据此提示这台部署还不能用固定回调。
func TestRedirectURIEmptyMeansLoopback(t *testing.T) {
	svc, _, _ := newSvc(t)
	svc.SetOAuthSettings(account.OAuthSettings{GoogleClientID: "cid"})
	if got := svc.RedirectURI(); got != "" {
		t.Fatalf("RedirectURI() = %q, want 空", got)
	}
}

// TestOAuthSettingsProviderIsLive 钉住「改完即刻生效」：凭据每次用时现取。
// 退回启动时读一次的话，管理员在设置页改完必须重启容器，而配 OAuth 应用
// 恰恰是要反复试的事。
func TestOAuthSettingsProviderIsLive(t *testing.T) {
	svc, _, _ := newSvc(t)
	current := account.OAuthSettings{}
	svc.SetOAuthSettingsProvider(func() account.OAuthSettings { return current })

	if svc.OAuthConfigured("google") {
		t.Fatal("空凭据时不该报告已配置")
	}
	current.GoogleClientID = "cid"
	if !svc.OAuthConfigured("google") {
		t.Fatal("凭据变更后应立即生效，无需重启")
	}
}
