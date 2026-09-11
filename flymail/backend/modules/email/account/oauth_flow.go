package account

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"flymail/internal/oauth"
)

// flowTTL 是一次授权流程的存活上限。超时后流程被标记失败并释放 loopback 端口。
//
// 10 分钟对齐服务商侧授权码的有效期量级：拖得更久也换不到令牌，只是白占端口。
const flowTTL = 10 * time.Minute

// 授权流程状态。
const (
	FlowPending = "pending"
	FlowSuccess = "success"
	FlowFailed  = "failed"
)

// 授权流程类型。
const (
	FlowModeCode   = "code"   // 授权码 + PKCE + loopback 回调
	FlowModeDevice = "device" // 设备码，用户在另一台设备上输码
)

// oauthFlow 是一次进行中的授权流程。
//
// 流程刻意只存在于内存中：它至多存活 10 分钟，且持有 code_verifier 这类
// 一次性机密，落库既无必要也扩大了泄露面。进程重启后未完成的授权自然作废，
// 用户重新点一次即可。
type oauthFlow struct {
	id       string
	provider string
	mode     string
	// accountID 非零表示这是对既有账户的重新授权，而不是新建账户。
	accountID uint
	expiresAt time.Time

	// 设备码流程需要展示给用户的信息。
	userCode        string
	verificationURI string

	// authURL 是授权码流程要在浏览器中打开的地址。
	authURL string

	cancel   context.CancelFunc
	loopback *oauth.LoopbackServer

	// 以下三项仅在「固定回调地址」模式下使用（见 startCodeFlow）：
	// 回调打到后端自身的公开端点，需要凭 state 找回流程并完成令牌交换。
	state       string
	verifier    string
	redirectURI string

	mu     sync.Mutex
	status string
	errMsg string
	token  *oauth.Token
	email  string
}

// snapshot 在持锁状态下读出可变字段，避免把锁泄露给调用方。
func (f *oauthFlow) snapshot() (status, errMsg, email string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, f.errMsg, f.email
}

func (f *oauthFlow) succeed(tok *oauth.Token) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status != FlowPending {
		return
	}
	f.status, f.token, f.email = FlowSuccess, tok, tok.Email
}

func (f *oauthFlow) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status != FlowPending {
		return
	}
	f.status, f.errMsg = FlowFailed, err.Error()
}

// finish 释放流程占用的资源，可重复调用。
func (f *oauthFlow) finish() {
	if f.cancel != nil {
		f.cancel()
	}
	if f.loopback != nil {
		f.loopback.Close()
	}
}

// StartOAuthRequest 发起授权的入参。
type StartOAuthRequest struct {
	Provider string `json:"provider" binding:"required"`
	// Mode 留空时自动选择：支持设备码的提供方也默认走授权码，体验更顺。
	Mode string `json:"mode,omitempty"`
	// Email 作为 login_hint，帮用户在账号选择页直接定位。
	Email string `json:"email,omitempty"`
	// AccountID 非零表示重新授权既有账户。
	AccountID uint `json:"account_id,omitempty"`
}

// StartOAuthResponse 发起授权的结果。
type StartOAuthResponse struct {
	FlowID    string    `json:"flow_id"`
	Provider  string    `json:"provider"`
	Mode      string    `json:"mode"`
	ExpiresAt time.Time `json:"expires_at"`
	// 授权码流程：需要在浏览器中打开的地址。
	AuthURL string `json:"auth_url,omitempty"`
	// Loopback 标明回调打的是本机临时端口。为真时要求浏览器与后端同机，
	// 前端据此在远程访问的场景下提示部署方去配置 oauth.redirect_base_url——
	// 否则用户授权完会跳回自己的 127.0.0.1，流程静默地停在 pending。
	Loopback bool `json:"loopback,omitempty"`
	// 设备码流程：需要用户输入的短码与验证地址。
	UserCode        string `json:"user_code,omitempty"`
	VerificationURI string `json:"verification_uri,omitempty"`
}

// OAuthFlowStatus 是轮询授权进度的结果。
type OAuthFlowStatus struct {
	FlowID   string `json:"flow_id"`
	Provider string `json:"provider"`
	Mode     string `json:"mode"`
	Status   string `json:"status"`
	Email    string `json:"email,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ProviderInfo 描述一个可用的 OAuth 提供方，供前端渲染入口。
type ProviderInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	DeviceCode bool   `json:"device_code"`
	IMAPHost   string `json:"imap_host"`
	SMTPHost   string `json:"smtp_host"`
}

// OAuthProviders 返回内置提供方及其配置状态。
//
// 未配置凭据的提供方也一并返回：前端据此把入口置灰并提示「管理员尚未配置」，
// 比直接隐藏更容易让部署方意识到还缺一步配置。
func (s *Service) OAuthProviders() []ProviderInfo {
	out := make([]ProviderInfo, 0, 2)
	for _, id := range []string{oauth.ProviderGoogle, oauth.ProviderMicrosoft} {
		p, _ := s.lookupProvider(id)
		out = append(out, ProviderInfo{
			ID: p.ID, Name: p.Name,
			Configured: s.OAuthConfigured(id),
			DeviceCode: p.SupportsDeviceCode(),
			IMAPHost:   p.IMAP.Host, SMTPHost: p.SMTP.Host,
		})
	}
	return out
}

func newFlowID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// StartOAuth 发起一次授权流程，立即返回用户需要执行的动作，后台异步等待结果。
func (s *Service) StartOAuth(req StartOAuthRequest) (*StartOAuthResponse, error) {
	client, err := s.oauthClient(req.Provider)
	if err != nil {
		return nil, err
	}
	// 重新授权时先确认账户存在，避免授权完成后才发现无处安放。
	if req.AccountID != 0 {
		if _, err := s.repo.GetByID(req.AccountID); err != nil {
			return nil, err
		}
	}
	mode := req.Mode
	if mode == "" {
		mode = FlowModeCode
	}
	if mode == FlowModeDevice && !client.Provider.SupportsDeviceCode() {
		return nil, fmt.Errorf("%s 不支持设备码流程", client.Provider.Name)
	}
	if mode != FlowModeCode && mode != FlowModeDevice {
		return nil, fmt.Errorf("未知的授权方式: %s", mode)
	}

	id, err := newFlowID()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), flowTTL)
	f := &oauthFlow{
		id: id, provider: req.Provider, mode: mode,
		accountID: req.AccountID,
		expiresAt: time.Now().Add(flowTTL),
		status:    FlowPending,
		cancel:    cancel,
	}

	if mode == FlowModeCode {
		err = s.startCodeFlow(ctx, f, client, req.Email)
	} else {
		err = s.startDeviceFlow(ctx, f, client)
	}
	if err != nil {
		f.finish()
		return nil, err
	}

	s.flows.Store(id, f)
	// 到期清理：流程结束后条目仍会被前端读一次状态，所以延后一小段再删。
	time.AfterFunc(flowTTL+time.Minute, func() {
		if v, ok := s.flows.LoadAndDelete(id); ok {
			v.(*oauthFlow).finish()
		}
	})

	return &StartOAuthResponse{
		FlowID: id, Provider: req.Provider, Mode: mode,
		ExpiresAt:       f.expiresAt,
		AuthURL:         f.authURL,
		Loopback:        f.loopback != nil,
		UserCode:        f.userCode,
		VerificationURI: f.verificationURI,
	}, nil
}

// CallbackPath 是固定回调模式下后端暴露的回调路径（相对 API 根）。
const CallbackPath = "/accounts/oauth/callback"

// startCodeFlow 构造授权地址并安排接收回调。
//
// 两种回调形态：
//   - 默认 loopback：在 127.0.0.1 的随机端口上临时监听。要求后端与浏览器同机，
//     适用于桌面端和本机自用，无需向服务商登记任何地址。
//   - 固定回调：部署方配置了 oauth.redirect_base_url 时改走后端自身的公开端点。
//     远程部署（如 Docker）下浏览器打不到服务端的 127.0.0.1，只能用这条路径；
//     代价是该地址必须在服务商后台登记为重定向 URI。
func (s *Service) startCodeFlow(ctx context.Context, f *oauthFlow, client *oauth.Client, loginHint string) error {
	state, err := oauth.RandomState()
	if err != nil {
		return err
	}
	pkce, err := oauth.NewPKCE()
	if err != nil {
		return err
	}
	if base := s.oauthCfg.RedirectBaseURL; base != "" {
		f.state, f.verifier = state, pkce.Verifier
		f.redirectURI = strings.TrimRight(base, "/") + CallbackPath
		f.authURL = client.AuthCodeURL(f.redirectURI, state, pkce, loginHint)
		s.states.Store(state, f.id)
		// 流程结束时清掉索引，避免 state 无限堆积。
		context.AfterFunc(ctx, func() { s.states.Delete(state) })
		return nil
	}
	lb, err := oauth.StartLoopback(state)
	if err != nil {
		return err
	}
	f.loopback = lb
	redirectURI := lb.RedirectURI()
	f.authURL = client.AuthCodeURL(redirectURI, state, pkce, loginHint)

	go func() {
		defer f.finish()
		select {
		case res := <-lb.Results():
			if res.Err != nil {
				f.fail(res.Err)
				return
			}
			tok, err := client.Exchange(ctx, res.Code, redirectURI, pkce.Verifier)
			if err != nil {
				f.fail(fmt.Errorf("换取令牌失败: %w", err))
				return
			}
			f.succeed(tok)
		case <-ctx.Done():
			f.fail(errors.New("授权超时，请重新发起"))
		}
	}()
	return nil
}

// startDeviceFlow 取回设备码并在后台按服务商给出的间隔轮询。
func (s *Service) startDeviceFlow(ctx context.Context, f *oauthFlow, client *oauth.Client) error {
	da, err := client.StartDeviceCode(ctx)
	if err != nil {
		return err
	}
	f.userCode, f.verificationURI = da.UserCode, da.VerificationURI

	go func() {
		defer f.finish()
		interval := time.Duration(da.Interval) * time.Second
		for {
			select {
			case <-ctx.Done():
				f.fail(errors.New("授权超时，请重新发起"))
				return
			case <-time.After(interval):
			}
			tok, err := client.PollDeviceCode(ctx, da.DeviceCode)
			switch {
			case err == nil:
				f.succeed(tok)
				return
			case errors.Is(err, oauth.ErrAuthorizationPending):
				// 用户还没输码，继续等。
			case errors.Is(err, oauth.ErrSlowDown):
				// 服务商要求降速；协议建议每次递增 5 秒。
				interval += 5 * time.Second
			default:
				f.fail(err)
				return
			}
		}
	}()
	return nil
}

// HandleCallback 处理固定回调模式下打到后端的授权回调。
//
// 这个端点必须免鉴权：它由服务商重定向用户浏览器直接访问，此时请求里没有 JWT。
// 安全性由 state 承担——它是一次性的高熵随机值，只有发起流程的人手里有。
func (s *Service) HandleCallback(state, code, errCode, errDesc string) error {
	if state == "" {
		return errors.New("回调缺少 state")
	}
	v, ok := s.states.Load(state)
	if !ok {
		return errors.New("回调校验失败：state 未知或已过期")
	}
	fv, ok := s.flows.Load(v.(string))
	if !ok {
		return errors.New("授权流程不存在或已过期")
	}
	f := fv.(*oauthFlow)
	// state 一次性：用过即删，堵掉授权码重放。
	s.states.Delete(state)

	if errCode != "" {
		desc := errDesc
		if desc == "" {
			desc = errCode
		}
		err := fmt.Errorf("授权被拒绝: %s", desc)
		f.fail(err)
		return err
	}
	if code == "" {
		err := errors.New("回调缺少授权码")
		f.fail(err)
		return err
	}
	client, err := s.oauthClient(f.provider)
	if err != nil {
		f.fail(err)
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tok, err := client.Exchange(ctx, code, f.redirectURI, f.verifier)
	if err != nil {
		err = fmt.Errorf("换取令牌失败: %w", err)
		f.fail(err)
		return err
	}
	f.succeed(tok)
	return nil
}

// OAuthFlowStatus 查询授权进度。
func (s *Service) OAuthFlowStatus(flowID string) (*OAuthFlowStatus, error) {
	v, ok := s.flows.Load(flowID)
	if !ok {
		return nil, errors.New("授权流程不存在或已过期")
	}
	f := v.(*oauthFlow)
	status, errMsg, email := f.snapshot()
	return &OAuthFlowStatus{
		FlowID: f.id, Provider: f.provider, Mode: f.mode,
		Status: status, Email: email, Error: errMsg,
	}, nil
}

// CancelOAuthFlow 主动放弃一次授权流程，释放本地端口。
func (s *Service) CancelOAuthFlow(flowID string) {
	if v, ok := s.flows.LoadAndDelete(flowID); ok {
		v.(*oauthFlow).finish()
	}
}

// CompleteOAuthRequest 用授权结果建号或续期的入参。
type CompleteOAuthRequest struct {
	FlowID string `json:"flow_id" binding:"required"`
	// Name 账户显示名，留空时用邮箱本地部分。
	Name string `json:"name,omitempty"`
	// Email 在服务商未回带邮箱时由用户补填。
	Email string `json:"email,omitempty"`
}

// CompleteOAuth 消费一次成功的授权：新建 OAuth 账户，或为既有账户续上新令牌。
func (s *Service) CompleteOAuth(req CompleteOAuthRequest) (*AccountResponse, error) {
	v, ok := s.flows.Load(req.FlowID)
	if !ok {
		return nil, errors.New("授权流程不存在或已过期")
	}
	f := v.(*oauthFlow)

	f.mu.Lock()
	status, tok := f.status, f.token
	f.mu.Unlock()
	if status != FlowSuccess || tok == nil {
		return nil, errors.New("授权尚未完成")
	}

	email := strings.TrimSpace(req.Email)
	if email == "" {
		email = tok.Email
	}
	if email == "" {
		return nil, errors.New("未能获取邮箱地址，请手动填写")
	}

	var resp *AccountResponse
	var err error
	if f.accountID != 0 {
		resp, err = s.reauthorize(f, tok, email)
	} else {
		resp, err = s.createOAuthAccount(f, tok, email, req.Name)
	}
	if err != nil {
		return nil, err
	}
	// 令牌已落库，流程随即作废——避免同一次授权被重复消费去建第二个账户。
	s.CancelOAuthFlow(req.FlowID)
	return resp, nil
}

// reauthorize 为既有账户写入新令牌并解除「需重新授权」状态。
func (s *Service) reauthorize(f *oauthFlow, tok *oauth.Token, email string) (*AccountResponse, error) {
	a, err := s.repo.GetByID(f.accountID)
	if err != nil {
		return nil, err
	}
	// 校验授权的邮箱与账户一致：用户在服务商页面上很容易选错账号，
	// 若不拦住，这个账户会顶着 A 的地址去同步 B 的邮箱。
	if !strings.EqualFold(a.Email, email) {
		return nil, fmt.Errorf("授权的邮箱 %s 与账户 %s 不一致，请用正确的账号重新授权", email, a.Email)
	}
	if err := s.saveToken(a.ID, f.provider, tok); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateFields(a.ID, map[string]any{"status": StatusOK}); err != nil {
		return nil, err
	}
	a, err = s.repo.GetByID(a.ID)
	if err != nil {
		return nil, err
	}
	resp := toResponse(a)
	return &resp, nil
}

// createOAuthAccount 用提供方预设建号，服务器地址无需用户填写。
func (s *Service) createOAuthAccount(f *oauthFlow, tok *oauth.Token, email, name string) (*AccountResponse, error) {
	p, ok := s.lookupProvider(f.provider)
	if !ok {
		return nil, fmt.Errorf("未知的 OAuth 提供方: %s", f.provider)
	}
	if name == "" {
		if i := strings.Index(email, "@"); i > 0 {
			name = email[:i]
		} else {
			name = email
		}
	}
	a := &Account{
		Name: name, Email: email,
		AuthType:      AuthTypeOAuth,
		OAuthProvider: f.provider,
		IMAPHost:      p.IMAP.Host, IMAPPort: p.IMAP.Port, IMAPSecurity: p.IMAP.Security,
		SMTPHost: p.SMTP.Host, SMTPPort: p.SMTP.Port, SMTPSecurity: p.SMTP.Security,
		Status:  StatusNew,
		Enabled: true,
	}
	if err := s.repo.Create(a); err != nil {
		return nil, err
	}
	if err := s.saveToken(a.ID, f.provider, tok); err != nil {
		return nil, err
	}
	a, err := s.repo.GetByID(a.ID)
	if err != nil {
		return nil, err
	}
	resp := toResponse(a)
	return &resp, nil
}
