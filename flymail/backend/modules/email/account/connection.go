package account

import (
	coreimap "flymail-core/imap"
	coresmtp "flymail-core/smtp"
	"flymail-core/types"
)

// TestConnectionRequest 测试连接请求（明文密码，用于保存前测试）。
type TestConnectionRequest struct {
	Email        string    `json:"email" binding:"required"`
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

func (r TestConnectionRequest) login() string {
	if r.Username != "" {
		return r.Username
	}
	return r.Email
}

// parseSecurity 把字符串安全模式映射为 core 类型，空值回退 none。
func parseSecurity(s string) types.SecurityMode {
	switch s {
	case "ssl":
		return types.SecuritySSL
	case "starttls":
		return types.SecurityStartTLS
	default:
		return types.SecurityNone
	}
}

func proxyFromDTO(p *ProxyDTO) *types.ProxyConfig {
	if p == nil || p.Host == "" {
		return nil
	}
	return &types.ProxyConfig{Type: p.Type, Host: p.Host, Port: p.Port, Username: p.Username, Password: p.Password}
}

// TestConnection 测试 IMAP 与 SMTP 连接/认证，返回结构化结果（不抛错）。
func (s *Service) TestConnection(req TestConnectionRequest) types.ConnectionTestResult {
	res := types.ConnectionTestResult{}
	proxy := proxyFromDTO(req.Proxy)

	imapCfg := types.IMAPConfig{
		Host:         req.IMAPHost,
		Port:         req.IMAPPort,
		Username:     req.login(),
		Password:     req.Password,
		Security:     parseSecurity(req.IMAPSecurity),
		Proxy:        proxy,
		ClientName:   "FlyMail",
		ClientVendor: "FlyMail",
	}
	if sess, err := coreimap.Dial(imapCfg); err != nil {
		res.IMAPError = err.Error()
	} else {
		res.IMAP = true
		res.SupportsIDLE = sess.SupportsIDLE
		res.Capabilities = sess.Capabilities
		res.SecurityMode = sess.SecurityMode
		_ = sess.Close()
	}

	smtpCfg := types.SMTPConfig{
		Host:     req.SMTPHost,
		Port:     req.SMTPPort,
		Username: req.login(),
		Password: req.Password,
		Security: parseSecurity(req.SMTPSecurity),
		Proxy:    proxy,
	}
	if err := coresmtp.NewClient(smtpCfg).TestConnection(); err != nil {
		res.SMTPError = err.Error()
	} else {
		res.SMTP = true
	}
	return res
}
