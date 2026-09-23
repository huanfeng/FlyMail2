package account

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"flymail/internal/oauth"
)

// tokenLeeway 是刷新提前量：令牌在这个窗口内到期就提前换新。
//
// 取 5 分钟而不是贴着过期时间：一次完整同步可能持续数分钟，若在开始时刚好有效、
// 中途过期，IMAP 会在半程被断开，写回队列还得重来。
const tokenLeeway = 5 * time.Minute

// ErrNeedsReauth 表示该账户的授权已失效，必须由用户重新走一次授权流程。
var ErrNeedsReauth = errors.New("账户需要重新授权")

// ErrOAuthNotConfigured 表示部署方尚未配置该提供方的 client_id。
var ErrOAuthNotConfigured = errors.New("未配置该提供方的 OAuth 客户端凭据")

// OAuthSettings 是部署级的 OAuth 客户端凭据。
//
// 由 app 层从配置注入，account 包因此不必依赖 config 包。client_secret 允许为空：
// loopback + PKCE 属于公共客户端，Google 的「桌面应用」类型虽然仍会签发 secret，
// 但规范上不视其为机密，Microsoft 公共客户端则根本不需要。
type OAuthSettings struct {
	GoogleClientID        string
	GoogleClientSecret    string
	MicrosoftClientID     string
	MicrosoftClientSecret string
	MicrosoftTenant       string
	// RedirectBaseURL 非空时改用固定回调地址（该 URL + CallbackPath），
	// 供浏览器与后端不同机的远程部署使用；留空则走 loopback。
	RedirectBaseURL string
}

// SetOAuthSettings 注入一份固定的 OAuth 客户端凭据（进程启动时读一次）。
func (s *Service) SetOAuthSettings(cfg OAuthSettings) { s.oauthCfg = cfg }

// SetOAuthSettingsProvider 注入「每次用时现取」的凭据来源，取代 SetOAuthSettings。
//
// 凭据现在可以由管理员在设置页里改（存数据库），而进程启动时读一次的话，
// 改完必须重启容器才生效——配 OAuth 应用恰恰是要反复试的事（回调地址填错、
// 测试用户没加、secret 复制漏一位），每试一次重启一次不可接受。
//
// 与 syncDepthFn / SetPollIntervalProvider 是同一个模式：account 包不依赖 setting 包，
// 由 app 层把「怎么取」注入进来。
func (s *Service) SetOAuthSettingsProvider(fn func() OAuthSettings) { s.oauthCfgFn = fn }

// oauthSettings 返回当前生效的凭据。全包唯一的读取出口。
func (s *Service) oauthSettings() OAuthSettings {
	if s.oauthCfgFn != nil {
		return s.oauthCfgFn()
	}
	return s.oauthCfg
}

// OAuthConfigured 报告某个提供方是否已配置凭据，供前端决定是否展示入口。
func (s *Service) OAuthConfigured(provider string) bool {
	cfg := s.oauthSettings()
	switch provider {
	case oauth.ProviderGoogle:
		return cfg.GoogleClientID != ""
	case oauth.ProviderMicrosoft:
		return cfg.MicrosoftClientID != ""
	default:
		return false
	}
}

// lookupProvider 解析提供方定义。默认走内置表，测试可替换为指向 httptest 的假端点。
func (s *Service) lookupProvider(id string) (oauth.Provider, bool) {
	tenant := s.oauthSettings().MicrosoftTenant
	if s.providerLookup != nil {
		return s.providerLookup(id, tenant)
	}
	return oauth.Lookup(id, tenant)
}

// oauthClient 按提供方组装一个协议客户端。
func (s *Service) oauthClient(provider string) (*oauth.Client, error) {
	p, ok := s.lookupProvider(provider)
	if !ok {
		return nil, fmt.Errorf("未知的 OAuth 提供方: %s", provider)
	}
	cfg := s.oauthSettings()
	c := &oauth.Client{Provider: p}
	switch provider {
	case oauth.ProviderGoogle:
		c.ClientID, c.ClientSecret = cfg.GoogleClientID, cfg.GoogleClientSecret
	case oauth.ProviderMicrosoft:
		c.ClientID, c.ClientSecret = cfg.MicrosoftClientID, cfg.MicrosoftClientSecret
	}
	if c.ClientID == "" {
		return nil, ErrOAuthNotConfigured
	}
	return c, nil
}

// accountLock 返回该账户专属的互斥锁。
//
// 为什么必须按账户串行：同步引擎、发送流程与手动触发可能同时发现令牌过期并各自去刷新。
// Microsoft 每次刷新都会轮换 refresh_token 并作废上一枚，两个并发刷新里慢的那个会拿着
// 已作废的凭据写回数据库，把账户推进「需重新授权」——一个纯粹由竞态制造的故障。
func (s *Service) accountLock(id uint) *sync.Mutex {
	v, _ := s.tokenLocks.LoadOrStore(id, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// loadToken 解密并反序列化账户上存储的令牌。
func (s *Service) loadToken(a *Account) (*oauth.Token, error) {
	if a.OAuthTokenEnc == "" {
		return nil, ErrNeedsReauth
	}
	raw, err := s.enc.Decrypt(a.OAuthTokenEnc)
	if err != nil {
		return nil, fmt.Errorf("解密令牌失败: %w", err)
	}
	var tok oauth.Token
	if err := json.Unmarshal([]byte(raw), &tok); err != nil {
		return nil, fmt.Errorf("解析令牌失败: %w", err)
	}
	return &tok, nil
}

// saveToken 加密并写回令牌，同时更新明文过期时间。
//
// 走列级更新而不是整行 Save：刷新可能与用户正在编辑账户设置并发，整行覆盖会把对方的
// 改动回滚掉；Save 还会连带重写 CreatedAt。
func (s *Service) saveToken(id uint, provider string, tok *oauth.Token) error {
	raw, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	enc, err := s.enc.Encrypt(string(raw))
	if err != nil {
		return fmt.Errorf("加密令牌失败: %w", err)
	}
	expiry := tok.Expiry
	fields := map[string]any{
		"auth_type":        AuthTypeOAuth,
		"oauth_provider":   provider,
		"oauth_token_enc":  enc,
		"oauth_expires_at": &expiry,
	}
	return s.repo.UpdateFields(id, fields)
}

// AccessToken 返回该账户当前可用的访问令牌，必要时先刷新。
//
// 这是 OAuth 账户凭据的唯一出口：IMAPConfig 与 SMTPConfig 都经由它取令牌，
// 因此「过期前自动刷新」只需在这一处实现，所有调用方无感。
func (s *Service) AccessToken(id uint) (string, error) {
	mu := s.accountLock(id)
	mu.Lock()
	defer mu.Unlock()

	a, err := s.repo.GetByID(id)
	if err != nil {
		return "", err
	}
	if !a.IsOAuth() {
		return "", fmt.Errorf("账户 %d 不是 OAuth 账户", id)
	}
	tok, err := s.loadToken(a)
	if err != nil {
		return "", err
	}
	// 持锁期间重新读库并复检有效性：并发调用中先到的那个可能已经刷过了，
	// 此时直接复用，避免把刚拿到的新令牌又刷一遍（Microsoft 会因此作废前一枚）。
	if tok.Valid(time.Now(), tokenLeeway) {
		return tok.AccessToken, nil
	}

	client, err := s.oauthClient(a.OAuthProvider)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fresh, err := client.Refresh(ctx, tok.RefreshToken)
	if err != nil {
		if errors.Is(err, oauth.ErrInvalidGrant) {
			s.markNeedsReauth(a)
			return "", ErrNeedsReauth
		}
		// 网络抖动等临时故障不改账户状态，让同步引擎按既有策略重试即可。
		return "", fmt.Errorf("刷新令牌失败: %w", err)
	}
	// 刷新响应不携带 id_token，邮箱字段要从旧令牌沿用，否则诊断信息会莫名其妙变空。
	if fresh.Email == "" {
		fresh.Email = tok.Email
	}
	if err := s.saveToken(a.ID, a.OAuthProvider, fresh); err != nil {
		return "", err
	}
	// 刷新成功意味着之前的「需重新授权」已被解除。
	if a.Status == StatusNeedsReauth {
		_ = s.repo.UpdateFields(a.ID, map[string]any{"status": StatusOK})
	}
	return fresh.AccessToken, nil
}

// markNeedsReauth 把账户置为需重新授权并通知用户。
//
// 只改状态、不禁用账户：禁用会让用户在界面上找不到这个账户，反而更难修复；
// 保留可见并给出明确状态，配合前端的「重新授权」入口才是可操作的路径。
func (s *Service) markNeedsReauth(a *Account) {
	if a.Status == StatusNeedsReauth {
		return
	}
	if err := s.repo.UpdateFields(a.ID, map[string]any{"status": StatusNeedsReauth}); err != nil {
		return
	}
	if s.emit != nil {
		s.emit("account_status", a.ID, 0, "账户需要重新授权",
			fmt.Sprintf("%s 的授权已失效，请在账户设置中重新授权。", a.Email))
	}
}
