// Package oauth 实现 FlyMail 接入 Gmail / Outlook 所需的 OAuth2 协议层。
//
// 这一层刻意保持「纯协议」：不碰数据库、不碰账户模型、所有端点与 HTTP 客户端均可注入，
// 因此在没有真实 Google / Azure 凭据的情况下也能用 httptest 完整覆盖授权码、PKCE、
// 刷新与设备码四条路径。持久化、加解密与账户状态流转由 modules/email/account 负责。
package oauth

import "strings"

// provider 标识。对外以字符串形式出现在 API 与数据库中，不要随意改名。
const (
	ProviderGoogle    = "google"
	ProviderMicrosoft = "microsoft"
)

// ServerPreset 是某个服务商的 IMAP/SMTP 接入参数，用于 OAuth 建号时免去用户手填。
type ServerPreset struct {
	Host     string
	Port     int
	Security string // ssl / starttls / none
}

// Provider 描述一个 OAuth2 身份提供方的端点与授权范围。
type Provider struct {
	ID        string
	Name      string
	AuthURL   string
	TokenURL  string
	DeviceURL string // 设备码端点；为空表示该提供方不支持设备码流程
	Scopes    []string
	IMAP      ServerPreset
	SMTP      ServerPreset

	// PasswordAuth 报告该服务商的 IMAP/SMTP 是否还接受「用户名 + 密码」。
	//
	// 这决定了「没配 OAuth 凭据」对用户意味着什么：Gmail 那边只是多绕一步
	// （申请个应用专用密码照样能用），Outlook 那边则是此路不通。
	// 界面要据此给出不同的退路，给错了比不给更糟——让人照着一条死路试半天。
	PasswordAuth bool
}

// SupportsDeviceCode 报告该提供方是否可走设备码流程。
func (p Provider) SupportsDeviceCode() bool { return p.DeviceURL != "" }

// ScopeString 把 scope 列表拼成协议要求的空格分隔形式。
func (p Provider) ScopeString() string { return strings.Join(p.Scopes, " ") }

// Google 的授权范围说明：
//   - https://mail.google.com/ 是 Gmail 唯一能同时授权 IMAP 与 SMTP 的 scope，
//     更细粒度的 gmail.readonly 等只对 Gmail API 生效，走不通 IMAP。
//   - openid/email 用于从 id_token 里取回邮箱地址，省掉一次 userinfo 请求。
func googleProvider() Provider {
	return Provider{
		ID:       ProviderGoogle,
		Name:     "Gmail",
		AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
		// Google 已于 2022 年停用 OAuth 设备码流程对 Gmail scope 的支持，故留空。
		DeviceURL: "",
		Scopes:    []string{"https://mail.google.com/", "openid", "email"},
		IMAP:      ServerPreset{Host: "imap.gmail.com", Port: 993, Security: "ssl"},
		SMTP:      ServerPreset{Host: "smtp.gmail.com", Port: 465, Security: "ssl"},
		// Google 在 2024-09 停掉了「不够安全的应用」那种明文口令，但**应用专用密码
		// 仍然可用**（前提是账号开了两步验证）。Google 已放话要逐步淘汰，
		// 真的关掉那天，这里改成 false 即可，界面文案会跟着变。
		PasswordAuth: true,
	}
}

// Microsoft 的授权范围说明：
//   - IMAP.AccessAsUser.All 与 SMTP.Send 是 Exchange Online 对应 IMAP/SMTP 的专用 scope。
//   - offline_access 必须显式申请，否则拿不到 refresh_token，用户每小时都要重新授权。
//   - tenant 决定受众：common 同时接受个人与工作账户，organizations 仅工作账户，
//     也可填具体租户 ID 把登录限制在单一组织内。
func microsoftProvider(tenant string) Provider {
	if tenant == "" {
		tenant = "common"
	}
	base := "https://login.microsoftonline.com/" + tenant + "/oauth2/v2.0/"
	return Provider{
		ID:        ProviderMicrosoft,
		Name:      "Outlook",
		AuthURL:   base + "authorize",
		TokenURL:  base + "token",
		DeviceURL: base + "devicecode",
		Scopes: []string{
			"https://outlook.office.com/IMAP.AccessAsUser.All",
			"https://outlook.office.com/SMTP.Send",
			"offline_access",
			"openid",
			"email",
		},
		IMAP: ServerPreset{Host: "outlook.office365.com", Port: 993, Security: "ssl"},
		SMTP: ServerPreset{Host: "smtp.office365.com", Port: 587, Security: "starttls"},
		// Microsoft 于 2024-09-16 对个人 Outlook.com/Hotmail/Live 账户彻底关闭了
		// 基本认证，连应用专用密码这条退路都没有——Outlook 只能走 OAuth。
		PasswordAuth: false,
	}
}

// Lookup 按 ID 返回内置提供方定义，tenant 仅对 microsoft 生效。
func Lookup(id, tenant string) (Provider, bool) {
	switch id {
	case ProviderGoogle:
		return googleProvider(), true
	case ProviderMicrosoft:
		return microsoftProvider(tenant), true
	default:
		return Provider{}, false
	}
}
