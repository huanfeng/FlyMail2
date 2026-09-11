package account

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"flymail/internal/crypto"
	"flymail/internal/oauth"

	coredb "flymail-core/database"
)

// fakeIDP 是一个假的授权服务器，记录收到的刷新请求并按脚本作答。
type fakeIDP struct {
	srv *httptest.Server
	// calls 刷新次数，用于断言并发刷新被合并。
	calls atomic.Int32
	// handler 由每个用例设置，收到已解析的表单。
	handler func(form url.Values, w http.ResponseWriter)
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()
	idp := &fakeIDP{}
	idp.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		idp.calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		idp.handler(r.PostForm, w)
	}))
	t.Cleanup(idp.srv.Close)
	return idp
}

// newOAuthSvc 构建一个把提供方端点指向假 IDP 的 Service。
func newOAuthSvc(t *testing.T, idp *fakeIDP) (*Service, *Repository) {
	t.Helper()
	// 这里只建 Account 表，而不用 internal/database.Migrate：那个包为了迁移全部模型
	// 反过来导入了本包，内部测试若再导入它就构成循环。
	// 用临时文件而非 :memory:：SQLite 的内存库是每连接独立的，连接池一旦换用另一条连接
	// 就会看到一个没有建过表的空库，表现为随机的 "no such column"。
	db, err := coredb.OpenSQLite(coredb.Options{Path: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if sqlDB, e := db.DB(); e == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := db.AutoMigrate(&Account{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	enc, err := crypto.New("a-test-encryption-key-32bytes!!")
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	repo := NewRepository(db)
	svc := NewService(repo, enc)
	svc.SetOAuthSettings(OAuthSettings{GoogleClientID: "cid", MicrosoftClientID: "mcid"})
	svc.providerLookup = func(id, tenant string) (oauth.Provider, bool) {
		p, ok := oauth.Lookup(id, tenant)
		if !ok {
			return p, false
		}
		p.TokenURL = idp.srv.URL
		p.DeviceURL = idp.srv.URL
		return p, true
	}
	return svc, repo
}

// seedOAuthAccount 建一个带令牌的 OAuth 账户，expiry 决定令牌是否已过期。
func seedOAuthAccount(t *testing.T, svc *Service, repo *Repository, expiry time.Time) *Account {
	t.Helper()
	a := &Account{
		Name: "n", Email: "me@gmail.com",
		AuthType: AuthTypeOAuth, OAuthProvider: oauth.ProviderGoogle,
		IMAPHost: "imap.gmail.com", IMAPPort: 993, IMAPSecurity: "ssl",
		SMTPHost: "smtp.gmail.com", SMTPPort: 465, SMTPSecurity: "ssl",
		Status: StatusOK, Enabled: true,
	}
	if err := repo.Create(a); err != nil {
		t.Fatalf("建号失败: %v", err)
	}
	tok := &oauth.Token{AccessToken: "old-at", RefreshToken: "old-rt", Expiry: expiry, Email: a.Email}
	if err := svc.saveToken(a.ID, oauth.ProviderGoogle, tok); err != nil {
		t.Fatalf("存令牌失败: %v", err)
	}
	got, err := repo.GetByID(a.ID)
	if err != nil {
		t.Fatalf("重读失败: %v", err)
	}
	return got
}

// TestAccessToken_ReusesValidToken 未临近过期时不应打扰授权服务器。
func TestAccessToken_ReusesValidToken(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) { t.Error("令牌仍有效时不应刷新") }
	svc, repo := newOAuthSvc(t, idp)
	a := seedOAuthAccount(t, svc, repo, time.Now().Add(time.Hour))

	tok, err := svc.AccessToken(a.ID)
	if err != nil {
		t.Fatalf("取令牌失败: %v", err)
	}
	if tok != "old-at" {
		t.Fatalf("应复用旧令牌，实际 %q", tok)
	}
}

// TestAccessToken_RefreshesNearExpiry 令牌落在提前量窗口内就该换新并落库。
func TestAccessToken_RefreshesNearExpiry(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(form url.Values, w http.ResponseWriter) {
		if form.Get("refresh_token") != "old-rt" {
			t.Errorf("应提交旧的 refresh_token，实际 %q", form.Get("refresh_token"))
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "new-at", "expires_in": 3600})
	}
	svc, repo := newOAuthSvc(t, idp)
	// 2 分钟后过期：仍未过期，但落在 5 分钟提前量内。
	a := seedOAuthAccount(t, svc, repo, time.Now().Add(2*time.Minute))

	tok, err := svc.AccessToken(a.ID)
	if err != nil {
		t.Fatalf("刷新失败: %v", err)
	}
	if tok != "new-at" {
		t.Fatalf("应返回新令牌，实际 %q", tok)
	}

	// 新令牌必须已加密落库，且过期时间被同步更新。
	got, _ := repo.GetByID(a.ID)
	if got.OAuthTokenEnc == "" {
		t.Fatal("令牌未落库")
	}
	if got.OAuthExpiresAt == nil || !got.OAuthExpiresAt.After(time.Now().Add(50*time.Minute)) {
		t.Fatalf("过期时间未更新: %v", got.OAuthExpiresAt)
	}
	stored, err := svc.loadToken(got)
	if err != nil {
		t.Fatalf("读回令牌失败: %v", err)
	}
	if stored.AccessToken != "new-at" {
		t.Fatalf("落库的访问令牌 = %q", stored.AccessToken)
	}
	// 刷新响应没回带 refresh_token（Google 的常态），必须沿用旧值。
	if stored.RefreshToken != "old-rt" {
		t.Fatalf("刷新令牌应沿用旧值，实际 %q", stored.RefreshToken)
	}
	// 邮箱不在刷新响应里，要从旧令牌沿用，否则诊断信息会变空。
	if stored.Email != "me@gmail.com" {
		t.Fatalf("邮箱应沿用旧值，实际 %q", stored.Email)
	}
}

// TestAccessToken_TokenIsEncryptedAtRest 令牌不得以明文落库。
func TestAccessToken_TokenIsEncryptedAtRest(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) {}
	svc, repo := newOAuthSvc(t, idp)
	a := seedOAuthAccount(t, svc, repo, time.Now().Add(time.Hour))

	got, _ := repo.GetByID(a.ID)
	if got.OAuthTokenEnc == "" {
		t.Fatal("令牌列为空")
	}
	for _, secret := range []string{"old-at", "old-rt"} {
		if contains(got.OAuthTokenEnc, secret) {
			t.Fatalf("密文中出现了明文 %q: %s", secret, got.OAuthTokenEnc)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

// TestAccessToken_ConcurrentRefreshRefreshesOnce 这是 Microsoft 令牌轮换下的关键保证：
// 并发请求必须合并成一次刷新，否则慢的那个会拿着已被作废的 refresh_token 写回数据库。
func TestAccessToken_ConcurrentRefreshRefreshesOnce(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(form url.Values, w http.ResponseWriter) {
		// 拖慢响应，放大竞态窗口。
		time.Sleep(50 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-at", "refresh_token": "rotated-rt", "expires_in": 3600,
		})
	}
	svc, repo := newOAuthSvc(t, idp)
	a := seedOAuthAccount(t, svc, repo, time.Now().Add(-time.Minute))

	const n = 8
	var wg sync.WaitGroup
	results := make([]string, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = svc.AccessToken(a.ID)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("第 %d 个调用失败: %v", i, err)
		}
		if results[i] != "new-at" {
			t.Fatalf("第 %d 个调用拿到 %q", i, results[i])
		}
	}
	if got := idp.calls.Load(); got != 1 {
		t.Fatalf("应只刷新一次，实际 %d 次", got)
	}
	stored, _ := repo.GetByID(a.ID)
	tok, _ := svc.loadToken(stored)
	if tok.RefreshToken != "rotated-rt" {
		t.Fatalf("轮换后的刷新令牌应落库，实际 %q", tok.RefreshToken)
	}
}

// TestAccessToken_InvalidGrantMarksNeedsReauth 授权被撤销时账户要进入明确的可恢复状态，
// 并且必须通知用户——否则同步只会静默地一直失败。
func TestAccessToken_InvalidGrantMarksNeedsReauth(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(form url.Values, w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "invalid_grant"})
	}
	svc, repo := newOAuthSvc(t, idp)

	var gotEvent, gotTitle string
	svc.SetEmitter(func(eventType string, accountID, messageID uint, title, body string) {
		gotEvent, gotTitle = eventType, title
	})
	a := seedOAuthAccount(t, svc, repo, time.Now().Add(-time.Minute))

	if _, err := svc.AccessToken(a.ID); err != ErrNeedsReauth {
		t.Fatalf("期望 ErrNeedsReauth，实际 %v", err)
	}
	got, _ := repo.GetByID(a.ID)
	if got.Status != StatusNeedsReauth {
		t.Fatalf("状态 = %q，期望 %q", got.Status, StatusNeedsReauth)
	}
	// 账户不能被禁用：禁用后用户在界面上找不到它，反而无法自助修复。
	if !got.Enabled {
		t.Fatal("不应禁用账户")
	}
	if gotEvent != "account_status" {
		t.Fatalf("应发出 account_status 通知，实际 %q", gotEvent)
	}
	if gotTitle == "" {
		t.Fatal("通知标题不应为空")
	}
}

// TestAccessToken_TransientErrorKeepsStatus 5xx 是临时故障，不能把账户打成需重新授权，
// 否则一次服务端抖动就要求用户重新走一遍授权。
func TestAccessToken_TransientErrorKeepsStatus(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(form url.Values, w http.ResponseWriter) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	svc, repo := newOAuthSvc(t, idp)
	a := seedOAuthAccount(t, svc, repo, time.Now().Add(-time.Minute))

	if _, err := svc.AccessToken(a.ID); err == nil {
		t.Fatal("应报错")
	} else if err == ErrNeedsReauth {
		t.Fatal("临时故障不应判定为授权失效")
	}
	got, _ := repo.GetByID(a.ID)
	if got.Status != StatusOK {
		t.Fatalf("状态不应改变，实际 %q", got.Status)
	}
}

// TestAccessToken_RecoversStatusAfterReauth 刷新成功应解除「需重新授权」。
func TestAccessToken_RecoversStatusAfterReauth(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(form url.Values, w http.ResponseWriter) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "new-at", "expires_in": 3600})
	}
	svc, repo := newOAuthSvc(t, idp)
	a := seedOAuthAccount(t, svc, repo, time.Now().Add(-time.Minute))
	if err := repo.UpdateFields(a.ID, map[string]any{"status": StatusNeedsReauth}); err != nil {
		t.Fatalf("置状态失败: %v", err)
	}

	if _, err := svc.AccessToken(a.ID); err != nil {
		t.Fatalf("刷新失败: %v", err)
	}
	got, _ := repo.GetByID(a.ID)
	if got.Status != StatusOK {
		t.Fatalf("状态 = %q，期望恢复为 %q", got.Status, StatusOK)
	}
}

// TestAccessToken_PasswordAccountRejected 密码账户走不到令牌通道。
func TestAccessToken_PasswordAccountRejected(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) {}
	svc, repo := newOAuthSvc(t, idp)
	a := &Account{Name: "n", Email: "p@x.com", AuthType: AuthTypePassword, Enabled: true}
	if err := repo.Create(a); err != nil {
		t.Fatalf("建号失败: %v", err)
	}
	if _, err := svc.AccessToken(a.ID); err == nil {
		t.Fatal("密码账户应报错")
	}
}

// TestAccessToken_MissingToken 没有令牌的 OAuth 账户直接判为需重新授权。
func TestAccessToken_MissingToken(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) {}
	svc, repo := newOAuthSvc(t, idp)
	a := &Account{Name: "n", Email: "o@x.com", AuthType: AuthTypeOAuth, Enabled: true}
	if err := repo.Create(a); err != nil {
		t.Fatalf("建号失败: %v", err)
	}
	if _, err := svc.AccessToken(a.ID); err != ErrNeedsReauth {
		t.Fatalf("期望 ErrNeedsReauth，实际 %v", err)
	}
}

// TestIMAPSMTPConfig_UsesAccessToken OAuth 账户必须以 AccessToken 而非 Password 出配置——
// 这是 core 侧切到 XOAUTH2 的开关。
func TestIMAPSMTPConfig_UsesAccessToken(t *testing.T) {
	idp := newFakeIDP(t)
	idp.handler = func(url.Values, http.ResponseWriter) { t.Error("令牌有效时不应刷新") }
	svc, repo := newOAuthSvc(t, idp)
	a := seedOAuthAccount(t, svc, repo, time.Now().Add(time.Hour))

	imapCfg, err := svc.IMAPConfig(a.ID)
	if err != nil {
		t.Fatalf("IMAPConfig 失败: %v", err)
	}
	if imapCfg.AccessToken != "old-at" {
		t.Fatalf("IMAP AccessToken = %q", imapCfg.AccessToken)
	}
	if imapCfg.Password != "" {
		t.Fatalf("OAuth 账户不应带密码，实际 %q", imapCfg.Password)
	}

	smtpCfg, err := svc.SMTPConfig(a.ID)
	if err != nil {
		t.Fatalf("SMTPConfig 失败: %v", err)
	}
	if smtpCfg.AccessToken != "old-at" {
		t.Fatalf("SMTP AccessToken = %q", smtpCfg.AccessToken)
	}
	if smtpCfg.Password != "" {
		t.Fatalf("OAuth 账户不应带密码，实际 %q", smtpCfg.Password)
	}
}

// TestOAuthConfigured 未配置 client_id 的提供方不应对外开放入口。
func TestOAuthConfigured(t *testing.T) {
	svc := &Service{}
	if svc.OAuthConfigured(oauth.ProviderGoogle) {
		t.Error("未配置时应为 false")
	}
	svc.SetOAuthSettings(OAuthSettings{GoogleClientID: "cid"})
	if !svc.OAuthConfigured(oauth.ProviderGoogle) {
		t.Error("已配置 Google 应为 true")
	}
	if svc.OAuthConfigured(oauth.ProviderMicrosoft) {
		t.Error("未配置 Microsoft 应为 false")
	}
	if svc.OAuthConfigured("yahoo") {
		t.Error("未知提供方应为 false")
	}
	if _, err := svc.oauthClient(oauth.ProviderMicrosoft); err != ErrOAuthNotConfigured {
		t.Fatalf("期望 ErrOAuthNotConfigured，实际 %v", err)
	}
}
