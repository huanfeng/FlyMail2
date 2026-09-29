package account

import "time"

// ProxyDTO 可选代理配置（请求/响应共用，密码仅入站）。
type ProxyDTO struct {
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type CreateAccountRequest struct {
	Name         string    `json:"name" binding:"required"`
	Email        string    `json:"email" binding:"required,email"`
	Username     string    `json:"username,omitempty"`
	Password     string    `json:"password" binding:"required"`
	IMAPHost     string    `json:"imap_host" binding:"required"`
	IMAPPort     int       `json:"imap_port" binding:"required"`
	IMAPSecurity string    `json:"imap_security"`
	SMTPHost     string    `json:"smtp_host" binding:"required"`
	SMTPPort     int       `json:"smtp_port" binding:"required"`
	SMTPSecurity string    `json:"smtp_security"`
	Proxy        *ProxyDTO `json:"proxy,omitempty"`
}

type UpdateAccountRequest struct {
	Name         string    `json:"name" binding:"required"`
	Email        string    `json:"email" binding:"required,email"`
	Username     string    `json:"username,omitempty"`
	Password     string    `json:"password,omitempty"`
	IMAPHost     string    `json:"imap_host" binding:"required"`
	IMAPPort     int       `json:"imap_port" binding:"required"`
	IMAPSecurity string    `json:"imap_security"`
	SMTPHost     string    `json:"smtp_host" binding:"required"`
	SMTPPort     int       `json:"smtp_port" binding:"required"`
	SMTPSecurity string    `json:"smtp_security"`
	Proxy        *ProxyDTO `json:"proxy,omitempty"`
}

type AccountResponse struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username,omitempty"`
	AuthType string `json:"auth_type"`
	// OAuthProvider 让前端知道该用哪个提供方发起「重新授权」。
	OAuthProvider string     `json:"oauth_provider,omitempty"`
	IMAPHost      string     `json:"imap_host"`
	IMAPPort      int        `json:"imap_port"`
	IMAPSecurity  string     `json:"imap_security"`
	SMTPHost      string     `json:"smtp_host"`
	SMTPPort      int        `json:"smtp_port"`
	SMTPSecurity  string     `json:"smtp_security"`
	Proxy         *ProxyDTO  `json:"proxy,omitempty"`
	Enabled       bool       `json:"enabled"`
	Status        string     `json:"status"`
	LastSyncAt    *time.Time `json:"last_sync_at,omitempty"`
	// Health 运行时健康快照；健康账户为 nil（json 缺省）。
	Health *AccountHealthInfo `json:"health,omitempty"`
}

func toResponse(a *Account) AccountResponse {
	resp := AccountResponse{
		ID: a.ID, Name: a.Name, Email: a.Email, Username: a.Username,
		AuthType: a.AuthType, OAuthProvider: a.OAuthProvider,
		IMAPHost: a.IMAPHost, IMAPPort: a.IMAPPort, IMAPSecurity: a.IMAPSecurity,
		SMTPHost: a.SMTPHost, SMTPPort: a.SMTPPort, SMTPSecurity: a.SMTPSecurity,
		Enabled:    a.Enabled,
		Status:     a.Status,
		LastSyncAt: a.LastSyncAt,
	}
	if a.ProxyHost != "" {
		resp.Proxy = &ProxyDTO{Type: a.ProxyType, Host: a.ProxyHost, Port: a.ProxyPort, Username: a.ProxyUsername}
	}
	return resp
}

// AccountHealthInfo 是账户运行时健康快照。
// nil/缺省表示健康，非 nil 即有故障或需授权（前端据此渲染标记）。
type AccountHealthInfo struct {
	NeedsReauth bool   `json:"needs_reauth"`
	BreakerOpen bool   `json:"breaker_open"`
	Failures    int    `json:"failures"`
	LastError   string `json:"last_error,omitempty"`
}
