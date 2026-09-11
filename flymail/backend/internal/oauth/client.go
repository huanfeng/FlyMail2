package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Token 是一次授权或刷新的结果。
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	Expiry       time.Time `json:"expiry"`
	// Email 由 id_token 解析而来，用于建号时自动填邮箱；可能为空。
	Email string `json:"email,omitempty"`
}

// Valid 报告令牌在 leeway 之后是否仍然有效。Expiry 为零值视为「未知过期时间」，
// 按已过期处理——宁可多刷一次，也不要拿着废令牌去连 IMAP 触发账户锁定。
func (t *Token) Valid(now time.Time, leeway time.Duration) bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	if t.Expiry.IsZero() {
		return false
	}
	return now.Add(leeway).Before(t.Expiry)
}

// ErrAuthorizationPending 表示设备码尚未被用户批准，调用方应继续轮询。
var ErrAuthorizationPending = errors.New("authorization_pending")

// ErrSlowDown 表示轮询过快，调用方应增大轮询间隔后重试。
var ErrSlowDown = errors.New("slow_down")

// ErrInvalidGrant 表示授权已被用户撤销或 refresh_token 失效，必须重新走完整授权。
// 账户层据此把账户置为「需重新授权」，而不是无休止地重试。
var ErrInvalidGrant = errors.New("invalid_grant")

// Client 绑定一个提供方与一组客户端凭据，执行具体的 OAuth2 交互。
type Client struct {
	Provider     Provider
	ClientID     string
	ClientSecret string // 公共客户端（loopback + PKCE）可为空
	HTTPClient   *http.Client
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// PKCE 是一次授权流程的 code_verifier / code_challenge 对。
type PKCE struct {
	Verifier  string
	Challenge string
}

// NewPKCE 生成符合 RFC 7636 的 S256 验证串对。
//
// 为什么必须用 PKCE：loopback 回调（127.0.0.1）上任何本机进程都能抢先监听端口或
// 观察到回调 URL 里的 code，而公共客户端的 client_secret 无法真正保密。PKCE 让
// 截获 code 的一方在没有 verifier 的情况下换不到令牌。
func NewPKCE() (PKCE, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return PKCE{}, fmt.Errorf("生成 code_verifier 失败: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	return PKCE{
		Verifier:  verifier,
		Challenge: base64.RawURLEncoding.EncodeToString(sum[:]),
	}, nil
}

// RandomState 生成用于防 CSRF 的 state 值。
func RandomState() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("生成 state 失败: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// AuthCodeURL 构造用户需要在浏览器中打开的授权地址。
//
// loginHint 非空时作为 login_hint 传入，让用户在账号选择页直接定位到目标邮箱。
func (c *Client) AuthCodeURL(redirectURI, state string, pkce PKCE, loginHint string) string {
	q := url.Values{}
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", c.Provider.ScopeString())
	q.Set("state", state)
	q.Set("code_challenge", pkce.Challenge)
	q.Set("code_challenge_method", "S256")
	// access_type=offline + prompt=consent 是 Google 返回 refresh_token 的必要条件：
	// 同一账户第二次授权时若不强制同意页，Google 只回 access_token，刷新链就断了。
	if c.Provider.ID == ProviderGoogle {
		q.Set("access_type", "offline")
		q.Set("prompt", "consent")
	}
	if loginHint != "" {
		q.Set("login_hint", loginHint)
	}
	return c.Provider.AuthURL + "?" + q.Encode()
}

// Exchange 用授权码换取令牌。
func (c *Client) Exchange(ctx context.Context, code, redirectURI, verifier string) (*Token, error) {
	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", verifier)
	if c.ClientSecret != "" {
		form.Set("client_secret", c.ClientSecret)
	}
	return c.token(ctx, c.Provider.TokenURL, form, "")
}

// Refresh 用 refresh_token 换取新的访问令牌。
//
// 传入的 refreshToken 会在响应未回带新值时被沿用：Google 通常只在首次授权时下发
// refresh_token，之后的刷新响应里没有该字段；Microsoft 则每次刷新都轮换一个新的。
// 不做这层回填，Google 账户刷新一次就会丢掉刷新凭据。
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*Token, error) {
	if refreshToken == "" {
		return nil, ErrInvalidGrant
	}
	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("refresh_token", refreshToken)
	form.Set("grant_type", "refresh_token")
	// Google 刷新时不接受 scope 收窄，Microsoft 要求带上原 scope，统一带上最稳妥。
	form.Set("scope", c.Provider.ScopeString())
	if c.ClientSecret != "" {
		form.Set("client_secret", c.ClientSecret)
	}
	return c.token(ctx, c.Provider.TokenURL, form, refreshToken)
}

// DeviceAuth 是设备码流程第一步的结果，需展示给用户完成授权。
type DeviceAuth struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message,omitempty"`
}

// StartDeviceCode 发起设备码授权，返回需要用户输入的短码与验证地址。
//
// 用途：Microsoft 个人账户（outlook.com / hotmail.com）在部分租户策略下走不通
// loopback 重定向，设备码是官方给出的替代路径。
func (c *Client) StartDeviceCode(ctx context.Context) (*DeviceAuth, error) {
	if !c.Provider.SupportsDeviceCode() {
		return nil, fmt.Errorf("%s 不支持设备码流程", c.Provider.Name)
	}
	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("scope", c.Provider.ScopeString())

	body, err := c.postForm(ctx, c.Provider.DeviceURL, form)
	if err != nil {
		return nil, err
	}
	var da DeviceAuth
	if err := json.Unmarshal(body, &da); err != nil {
		return nil, fmt.Errorf("解析设备码响应失败: %w", err)
	}
	if da.DeviceCode == "" || da.UserCode == "" {
		return nil, errors.New("设备码响应缺少 device_code/user_code")
	}
	if da.Interval <= 0 {
		da.Interval = 5
	}
	return &da, nil
}

// PollDeviceCode 轮询一次设备码令牌端点。
//
// 返回 ErrAuthorizationPending 表示用户尚未完成授权，ErrSlowDown 表示需要放慢；
// 调用方负责按 DeviceAuth.Interval 控制节奏，本函数不自行睡眠，便于测试与取消。
func (c *Client) PollDeviceCode(ctx context.Context, deviceCode string) (*Token, error) {
	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	return c.token(ctx, c.Provider.TokenURL, form, "")
}

// tokenResponse 是令牌端点的原始响应结构。
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
	IDToken      string `json:"id_token"`

	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// token 执行一次令牌端点请求并归一化结果。fallbackRefresh 在响应未回带
// refresh_token 时被沿用（见 Refresh 的说明）。
func (c *Client) token(ctx context.Context, endpoint string, form url.Values, fallbackRefresh string) (*Token, error) {
	body, err := c.postForm(ctx, endpoint, form)
	if err != nil {
		return nil, err
	}
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("解析令牌响应失败: %w", err)
	}
	if tr.Error != "" {
		return nil, mapOAuthError(tr.Error, tr.ErrorDescription)
	}
	if tr.AccessToken == "" {
		return nil, errors.New("令牌响应缺少 access_token")
	}
	tok := &Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
		Scope:        tr.Scope,
		Email:        emailFromIDToken(tr.IDToken),
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = fallbackRefresh
	}
	if tr.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return tok, nil
}

// postForm 发送表单请求并返回响应体。
//
// 注意：非 2xx 也要读出响应体——OAuth2 的错误语义（authorization_pending、
// invalid_grant）恰恰藏在 4xx 响应的 JSON 里，直接按状态码报错会丢失关键分支。
func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("授权服务器返回 %d", resp.StatusCode)
	}
	return body, nil
}

// mapOAuthError 把协议错误码映射为调用方可分支处理的哨兵错误。
func mapOAuthError(code, desc string) error {
	switch code {
	case "authorization_pending":
		return ErrAuthorizationPending
	case "slow_down":
		return ErrSlowDown
	case "invalid_grant", "expired_token":
		return ErrInvalidGrant
	}
	if desc != "" {
		return fmt.Errorf("授权失败: %s (%s)", desc, code)
	}
	return fmt.Errorf("授权失败: %s", code)
}

// idTokenClaims 只取我们关心的身份字段。
type idTokenClaims struct {
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
	UPN               string `json:"upn"`
}

// emailFromIDToken 从 id_token 载荷中取回邮箱地址，取不到返回空串。
//
// 这里不校验签名：id_token 是我们自己通过 TLS 直连令牌端点换回来的，不经过第三方传递，
// 属于 OIDC 规范明确豁免签名校验的场景（Core 3.1.3.7）。它只用于预填邮箱，
// 不承担任何鉴权职责——真正的权限边界在 access_token 上。
func emailFromIDToken(idToken string) string {
	if idToken == "" {
		return ""
	}
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims idTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	// Microsoft 个人账户的 id_token 常常没有 email，只有 preferred_username。
	for _, v := range []string{claims.Email, claims.PreferredUsername, claims.UPN} {
		if strings.Contains(v, "@") {
			return v
		}
	}
	return ""
}
