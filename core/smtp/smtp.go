package smtp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"flymail-core/types"
)

// tlsPlainAuth 是 net/smtp PlainAuth 的变体：当我们已自行建立隐式 TLS（SSL/465）后，
// net/smtp.Client 内部的 tls 标志仍为 false，标准 PlainAuth 会误判"连接未加密"而拒发凭证。
// 本变体在连接确实已加密（由调用方保证）时跳过该检查，仅保留主机名核对。
type tlsPlainAuth struct {
	identity, username, password, host string
}

func (a *tlsPlainAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if server.Name != a.host {
		return "", nil, errors.New("wrong host name")
	}
	resp := []byte(a.identity + "\x00" + a.username + "\x00" + a.password)
	return "PLAIN", resp, nil
}

func (a *tlsPlainAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("unexpected server challenge")
	}
	return nil, nil
}

// authFor 依据连接是否已加密选择认证方式：已加密（SSL 隐式 TLS 或 STARTTLS 升级）用
// tlsPlainAuth 跳过 net/smtp 的 TLS 自检；未加密仍用标准 PlainAuth（其对非 localhost 会拒绝，保安全）。
func (c *Client) authFor(secured bool) smtp.Auth {
	if secured {
		return &tlsPlainAuth{"", c.config.Username, c.config.Password, c.config.Host}
	}
	return smtp.PlainAuth("", c.config.Username, c.config.Password, c.config.Host)
}

// Client wraps SMTP operations with support for SSL/STARTTLS and proxy.
type Client struct {
	config types.SMTPConfig
}

// NewClient creates an SMTP client from config.
func NewClient(cfg types.SMTPConfig) *Client {
	return &Client{config: cfg}
}

// SendEmail sends an email through the configured SMTP server.
func (c *Client) SendEmail(from string, to, cc, bcc []string, subject, body, contentType string) error {
	conn, secured, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Quit()

	if err := conn.Auth(c.authFor(secured)); err != nil {
		return fmt.Errorf("SMTP auth failed: %w", err)
	}

	// Build recipients
	recipients := make([]string, 0, len(to)+len(cc)+len(bcc))
	recipients = append(recipients, to...)
	recipients = append(recipients, cc...)
	recipients = append(recipients, bcc...)

	// Build message
	if contentType == "" {
		contentType = "text/plain; charset=UTF-8"
	}
	msg := buildMessage(from, to, cc, subject, body, contentType)

	// Send
	if err := conn.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM failed: %w", err)
	}
	for _, rcpt := range recipients {
		if err := conn.Rcpt(rcpt); err != nil {
			return fmt.Errorf("RCPT TO %s failed: %w", rcpt, err)
		}
	}

	w, err := conn.Data()
	if err != nil {
		return fmt.Errorf("DATA failed: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("write body failed: %w", err)
	}
	return w.Close()
}

// SendRaw sends a pre-built RFC 5322 message. The caller builds `raw` (headers + body);
// recipients includes To/Cc/Bcc (Bcc must NOT appear in raw headers).
func (c *Client) SendRaw(from string, recipients []string, raw []byte) error {
	conn, secured, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Quit()

	if err := conn.Auth(c.authFor(secured)); err != nil {
		return fmt.Errorf("SMTP auth failed: %w", err)
	}
	if err := conn.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM failed: %w", err)
	}
	for _, rcpt := range recipients {
		if err := conn.Rcpt(rcpt); err != nil {
			return fmt.Errorf("RCPT TO %s failed: %w", rcpt, err)
		}
	}
	w, err := conn.Data()
	if err != nil {
		return fmt.Errorf("DATA failed: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	return w.Close()
}

// TestConnection verifies the SMTP connection and authentication.
func (c *Client) TestConnection() error {
	conn, secured, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Quit()

	if err := conn.Auth(c.authFor(secured)); err != nil {
		return fmt.Errorf("SMTP auth failed: %w", err)
	}
	return nil
}

// connect establishes an SMTP connection respecting SecurityMode and proxy settings.
// The returned bool reports whether the transport is encrypted (implicit TLS or STARTTLS-upgraded),
// which callers pass to authFor so PLAIN auth can proceed on manually-wrapped TLS connections.
func (c *Client) connect() (*smtp.Client, bool, error) {
	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
	tlsConfig := &tls.Config{ServerName: c.config.Host}

	switch c.config.Security {
	case types.SecuritySSL:
		rawConn, err := c.dial("tcp", addr)
		if err != nil {
			return nil, false, fmt.Errorf("connect failed: %w", err)
		}
		tlsConn := tls.Client(rawConn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			rawConn.Close()
			return nil, false, fmt.Errorf("TLS handshake failed: %w", err)
		}
		client, err := smtp.NewClient(tlsConn, c.config.Host)
		if err != nil {
			return nil, false, err
		}
		return client, true, nil

	case types.SecurityStartTLS:
		rawConn, err := c.dial("tcp", addr)
		if err != nil {
			return nil, false, fmt.Errorf("connect failed: %w", err)
		}
		client, err := smtp.NewClient(rawConn, c.config.Host)
		if err != nil {
			rawConn.Close()
			return nil, false, err
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			client.Quit()
			return nil, false, fmt.Errorf("STARTTLS failed: %w", err)
		}
		return client, true, nil

	default: // SecurityNone — try opportunistic STARTTLS
		rawConn, err := c.dial("tcp", addr)
		if err != nil {
			return nil, false, fmt.Errorf("connect failed: %w", err)
		}
		client, err := smtp.NewClient(rawConn, c.config.Host)
		if err != nil {
			rawConn.Close()
			return nil, false, err
		}
		secured := false
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				// Log but don't fail — server advertised STARTTLS but it didn't work
				_ = err
			} else {
				secured = true
			}
		}
		return client, secured, nil
	}
}

// dial connects via proxy if configured, otherwise direct.
func (c *Client) dial(network, addr string) (net.Conn, error) {
	if c.config.Proxy != nil && c.config.Proxy.Enabled() {
		return dialProxy(c.config.Proxy, network, addr)
	}
	return net.DialTimeout(network, addr, 10*time.Second)
}

func buildMessage(from string, to, cc []string, subject, body, contentType string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "From: %s\r\n", from)
	fmt.Fprintf(&sb, "To: %s\r\n", strings.Join(to, ", "))
	if len(cc) > 0 {
		fmt.Fprintf(&sb, "Cc: %s\r\n", strings.Join(cc, ", "))
	}
	fmt.Fprintf(&sb, "Subject: %s\r\n", subject)
	fmt.Fprintf(&sb, "Content-Type: %s\r\n", contentType)
	sb.WriteString("\r\n")
	sb.WriteString(body)
	return sb.String()
}
