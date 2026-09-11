package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// newTestClient 起一个假令牌端点，handler 收到已解析的表单并自行写响应。
func newTestClient(t *testing.T, handler func(form url.Values, w http.ResponseWriter)) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("解析表单失败: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		handler(r.PostForm, w)
	}))
	t.Cleanup(srv.Close)

	p, _ := Lookup(ProviderGoogle, "")
	p.TokenURL = srv.URL
	p.DeviceURL = srv.URL
	return &Client{Provider: p, ClientID: "cid", HTTPClient: srv.Client()}, srv
}

// makeIDToken 拼一个只有载荷有意义的 id_token（签名部分不参与解析）。
func makeIDToken(claims map[string]string) string {
	payload, _ := json.Marshal(claims)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func TestNewPKCE(t *testing.T) {
	a, err := NewPKCE()
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	if a.Verifier == "" || a.Challenge == "" {
		t.Fatal("verifier/challenge 不应为空")
	}
	// RFC 7636 要求 verifier 长度在 43~128 之间。
	if len(a.Verifier) < 43 || len(a.Verifier) > 128 {
		t.Fatalf("verifier 长度 %d 超出 RFC 7636 范围", len(a.Verifier))
	}
	// challenge 必须是 verifier 的 S256，而不是 verifier 本身（plain 方法已被视为不安全）。
	if a.Challenge == a.Verifier {
		t.Fatal("challenge 不应等于 verifier")
	}
	b, _ := NewPKCE()
	if a.Verifier == b.Verifier {
		t.Fatal("两次生成不应相同")
	}
}

// TestAuthCodeURL_Google 校验 Google 特有的 access_type/prompt——缺了它们
// 第二次授权就拿不到 refresh_token。
func TestAuthCodeURL_Google(t *testing.T) {
	p, _ := Lookup(ProviderGoogle, "")
	c := &Client{Provider: p, ClientID: "cid"}
	raw := c.AuthCodeURL("http://127.0.0.1:1234/oauth/callback", "st4te", PKCE{Challenge: "chal"}, "me@gmail.com")

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("URL 不合法: %v", err)
	}
	q := u.Query()
	for k, want := range map[string]string{
		"client_id":             "cid",
		"response_type":         "code",
		"state":                 "st4te",
		"code_challenge":        "chal",
		"code_challenge_method": "S256",
		"access_type":           "offline",
		"prompt":                "consent",
		"login_hint":            "me@gmail.com",
		"redirect_uri":          "http://127.0.0.1:1234/oauth/callback",
	} {
		if got := q.Get(k); got != want {
			t.Errorf("%s = %q，期望 %q", k, got, want)
		}
	}
	if !strings.Contains(q.Get("scope"), "https://mail.google.com/") {
		t.Errorf("scope 缺少 Gmail IMAP/SMTP 权限: %q", q.Get("scope"))
	}
}

// TestAuthCodeURL_Microsoft 微软不加 access_type/prompt，但必须带 offline_access
// 才会下发 refresh_token；tenant 也要正确拼进端点。
func TestAuthCodeURL_Microsoft(t *testing.T) {
	p, ok := Lookup(ProviderMicrosoft, "consumers")
	if !ok {
		t.Fatal("microsoft 提供方应存在")
	}
	if !strings.Contains(p.AuthURL, "/consumers/") {
		t.Errorf("tenant 未拼入端点: %s", p.AuthURL)
	}
	if !strings.Contains(p.ScopeString(), "offline_access") {
		t.Errorf("scope 缺少 offline_access: %s", p.ScopeString())
	}
	c := &Client{Provider: p, ClientID: "cid"}
	q, _ := url.Parse(c.AuthCodeURL("http://127.0.0.1:1/oauth/callback", "s", PKCE{Challenge: "c"}, ""))
	if q.Query().Has("access_type") {
		t.Error("微软不应带 access_type")
	}
	if q.Query().Has("login_hint") {
		t.Error("空 loginHint 不应出现在查询串里")
	}
}

func TestLookup_Unknown(t *testing.T) {
	if _, ok := Lookup("yahoo", ""); ok {
		t.Fatal("未知提供方应返回 false")
	}
}

// TestExchange 授权码换令牌：验证 PKCE verifier 与 grant_type 被正确提交，
// 且 id_token 里的邮箱被解析出来。
func TestExchange(t *testing.T) {
	var got url.Values
	c, _ := newTestClient(t, func(form url.Values, w http.ResponseWriter) {
		got = form
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at",
			"refresh_token": "rt",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"id_token":      makeIDToken(map[string]string{"email": "me@gmail.com"}),
		})
	})

	tok, err := c.Exchange(context.Background(), "the-code", "http://127.0.0.1:1/oauth/callback", "the-verifier")
	if err != nil {
		t.Fatalf("交换失败: %v", err)
	}
	if got.Get("grant_type") != "authorization_code" {
		t.Errorf("grant_type = %q", got.Get("grant_type"))
	}
	if got.Get("code_verifier") != "the-verifier" {
		t.Errorf("code_verifier 未提交: %q", got.Get("code_verifier"))
	}
	if got.Has("client_secret") {
		t.Error("ClientSecret 为空时不应提交该字段（公共客户端）")
	}
	if tok.AccessToken != "at" || tok.RefreshToken != "rt" {
		t.Errorf("令牌解析错误: %+v", tok)
	}
	if tok.Email != "me@gmail.com" {
		t.Errorf("邮箱应从 id_token 解析，实际 %q", tok.Email)
	}
	if !tok.Valid(time.Now(), 5*time.Minute) {
		t.Error("expires_in=3600 的令牌应判定为有效")
	}
}

func TestExchange_WithSecret(t *testing.T) {
	var got url.Values
	c, _ := newTestClient(t, func(form url.Values, w http.ResponseWriter) {
		got = form
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "expires_in": 60})
	})
	c.ClientSecret = "shh"
	if _, err := c.Exchange(context.Background(), "c", "r", "v"); err != nil {
		t.Fatalf("交换失败: %v", err)
	}
	if got.Get("client_secret") != "shh" {
		t.Errorf("client_secret 应被提交，实际 %q", got.Get("client_secret"))
	}
}

// TestRefresh_KeepsOldRefreshToken 这是最容易踩的坑：Google 刷新响应里通常没有
// refresh_token，若不沿用旧值，账户刷新一次就永久失去刷新能力。
func TestRefresh_KeepsOldRefreshToken(t *testing.T) {
	c, _ := newTestClient(t, func(form url.Values, w http.ResponseWriter) {
		if form.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", form.Get("grant_type"))
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "new-at", "expires_in": 3600})
	})

	tok, err := c.Refresh(context.Background(), "old-rt")
	if err != nil {
		t.Fatalf("刷新失败: %v", err)
	}
	if tok.RefreshToken != "old-rt" {
		t.Fatalf("响应未回带 refresh_token 时应沿用旧值，实际 %q", tok.RefreshToken)
	}
}

// TestRefresh_RotatedToken 微软每次刷新都轮换 refresh_token，新值必须覆盖旧值。
func TestRefresh_RotatedToken(t *testing.T) {
	c, _ := newTestClient(t, func(form url.Values, w http.ResponseWriter) {
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-at", "refresh_token": "rotated-rt", "expires_in": 3600,
		})
	})
	tok, err := c.Refresh(context.Background(), "old-rt")
	if err != nil {
		t.Fatalf("刷新失败: %v", err)
	}
	if tok.RefreshToken != "rotated-rt" {
		t.Fatalf("应采用轮换后的新值，实际 %q", tok.RefreshToken)
	}
}

func TestRefresh_EmptyToken(t *testing.T) {
	c, _ := newTestClient(t, func(url.Values, http.ResponseWriter) {
		t.Error("没有 refresh_token 时不应发起请求")
	})
	if _, err := c.Refresh(context.Background(), ""); err != ErrInvalidGrant {
		t.Fatalf("期望 ErrInvalidGrant，实际 %v", err)
	}
}

// TestRefresh_InvalidGrant 授权被撤销时必须映射为 ErrInvalidGrant，
// 账户层据此转入「需重新授权」而不是无限重试。注意这条错误是以 400 返回的。
func TestRefresh_InvalidGrant(t *testing.T) {
	c, _ := newTestClient(t, func(form url.Values, w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "invalid_grant", "error_description": "Token has been revoked.",
		})
	})
	if _, err := c.Refresh(context.Background(), "rt"); err != ErrInvalidGrant {
		t.Fatalf("期望 ErrInvalidGrant，实际 %v", err)
	}
}

func TestToken_ServerError(t *testing.T) {
	c, _ := newTestClient(t, func(form url.Values, w http.ResponseWriter) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if _, err := c.Refresh(context.Background(), "rt"); err == nil {
		t.Fatal("5xx 应报错")
	} else if err == ErrInvalidGrant {
		t.Fatal("5xx 是临时故障，不能当成授权失效")
	}
}

func TestToken_MissingAccessToken(t *testing.T) {
	c, _ := newTestClient(t, func(form url.Values, w http.ResponseWriter) {
		json.NewEncoder(w).Encode(map[string]any{"token_type": "Bearer"})
	})
	if _, err := c.Exchange(context.Background(), "c", "r", "v"); err == nil {
		t.Fatal("缺少 access_token 应报错")
	}
}

// TestDeviceCode 设备码全流程：起码 → 未批准 → 放慢 → 成功。
func TestDeviceCode(t *testing.T) {
	step := 0
	c, _ := newTestClient(t, func(form url.Values, w http.ResponseWriter) {
		if form.Has("scope") && !form.Has("device_code") && !form.Has("grant_type") {
			json.NewEncoder(w).Encode(map[string]any{
				"device_code": "dc", "user_code": "ABCD-EFGH",
				"verification_uri": "https://microsoft.com/devicelogin", "expires_in": 900,
			})
			return
		}
		step++
		switch step {
		case 1:
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": "authorization_pending"})
		case 2:
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": "slow_down"})
		default:
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "at", "refresh_token": "rt", "expires_in": 3600,
				"id_token": makeIDToken(map[string]string{"preferred_username": "me@outlook.com"}),
			})
		}
	})
	c.Provider.ID = ProviderMicrosoft

	da, err := c.StartDeviceCode(context.Background())
	if err != nil {
		t.Fatalf("发起设备码失败: %v", err)
	}
	if da.UserCode != "ABCD-EFGH" {
		t.Errorf("user_code = %q", da.UserCode)
	}
	if da.Interval != 5 {
		t.Errorf("响应未给 interval 时应回退到 5，实际 %d", da.Interval)
	}

	if _, err := c.PollDeviceCode(context.Background(), "dc"); err != ErrAuthorizationPending {
		t.Fatalf("首轮应为 ErrAuthorizationPending，实际 %v", err)
	}
	if _, err := c.PollDeviceCode(context.Background(), "dc"); err != ErrSlowDown {
		t.Fatalf("次轮应为 ErrSlowDown，实际 %v", err)
	}
	tok, err := c.PollDeviceCode(context.Background(), "dc")
	if err != nil {
		t.Fatalf("末轮应成功，实际 %v", err)
	}
	// 个人账户的 id_token 常常没有 email，只有 preferred_username。
	if tok.Email != "me@outlook.com" {
		t.Errorf("应回退到 preferred_username，实际 %q", tok.Email)
	}
}

// TestStartDeviceCode_Unsupported Google 不支持设备码，应直接拒绝而不是发无效请求。
func TestStartDeviceCode_Unsupported(t *testing.T) {
	p, _ := Lookup(ProviderGoogle, "")
	c := &Client{Provider: p, ClientID: "cid"}
	if _, err := c.StartDeviceCode(context.Background()); err == nil {
		t.Fatal("Google 应拒绝设备码流程")
	}
}

func TestEmailFromIDToken(t *testing.T) {
	cases := []struct {
		name, token, want string
	}{
		{"空串", "", ""},
		{"段数不对", "a.b", ""},
		{"载荷非 base64", "a.!!!.c", ""},
		{"载荷非 JSON", "a." + base64.RawURLEncoding.EncodeToString([]byte("nope")) + ".c", ""},
		{"email 优先", makeIDToken(map[string]string{"email": "a@x.com", "upn": "b@x.com"}), "a@x.com"},
		{"回退 upn", makeIDToken(map[string]string{"upn": "b@x.com"}), "b@x.com"},
		{"非邮箱值被忽略", makeIDToken(map[string]string{"preferred_username": "justaname"}), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := emailFromIDToken(tc.token); got != tc.want {
				t.Fatalf("= %q，期望 %q", got, tc.want)
			}
		})
	}
}

// TestToken_Valid 过期判定：零值 Expiry 必须当作已过期，否则会拿废令牌去连 IMAP。
func TestToken_Valid(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		tok  *Token
		want bool
	}{
		{"nil", nil, false},
		{"空令牌", &Token{Expiry: now.Add(time.Hour)}, false},
		{"零值过期时间", &Token{AccessToken: "a"}, false},
		{"已过期", &Token{AccessToken: "a", Expiry: now.Add(-time.Minute)}, false},
		{"落在提前量内", &Token{AccessToken: "a", Expiry: now.Add(2 * time.Minute)}, false},
		{"有效", &Token{AccessToken: "a", Expiry: now.Add(time.Hour)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.tok.Valid(now, 5*time.Minute); got != tc.want {
				t.Fatalf("= %v，期望 %v", got, tc.want)
			}
		})
	}
}
