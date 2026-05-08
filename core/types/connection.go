package types

// SecurityMode represents the TLS mode for a connection.
type SecurityMode string

const (
	SecurityNone     SecurityMode = "none"
	SecuritySSL      SecurityMode = "ssl"
	SecurityStartTLS SecurityMode = "starttls"
)

// ProxyConfig defines proxy settings for network connections.
type ProxyConfig struct {
	Type     string `json:"type"`               // socks5, http
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"` // plaintext (caller handles encryption)
}

// Enabled returns true if the proxy is configured.
func (p *ProxyConfig) Enabled() bool {
	return p != nil && p.Host != "" && p.Port > 0
}

// IMAPConfig holds IMAP connection parameters.
type IMAPConfig struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	Username     string        `json:"username"`
	Password     string        `json:"-"`
	AccessToken  string        `json:"-"` // when non-empty, use XOAUTH2 instead of password
	Security     SecurityMode  `json:"security"`
	Proxy        *ProxyConfig  `json:"proxy,omitempty"`
	ClientName   string        `json:"client_name,omitempty"`   // for IMAP ID extension (e.g. "Mail2IM", "FlyMail")
	ClientVendor string        `json:"client_vendor,omitempty"` // for IMAP ID extension
}

// SMTPConfig holds SMTP connection parameters.
type SMTPConfig struct {
	Host     string       `json:"host"`
	Port     int          `json:"port"`
	Username string       `json:"username"`
	Password string       `json:"-"`
	Security SecurityMode `json:"security"`
	Proxy    *ProxyConfig `json:"proxy,omitempty"`
}

// ConnectionTestResult reports the outcome of testing IMAP and SMTP connections.
type ConnectionTestResult struct {
	IMAP         bool     `json:"imap"`
	SMTP         bool     `json:"smtp"`
	SupportsIDLE bool     `json:"supports_idle"`
	Capabilities []string `json:"capabilities,omitempty"`
	SecurityMode string   `json:"security_mode,omitempty"` // actual negotiated mode
	IMAPError    string   `json:"imap_error,omitempty"`
	SMTPError    string   `json:"smtp_error,omitempty"`
}
