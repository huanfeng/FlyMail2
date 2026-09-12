package smtp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"flymail-core/logger"
	"flymail-core/types"

	"go.uber.org/zap"
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

// authFor 选择认证方式。tlsPlainAuth 跳过 net/smtp 的 TLS 自检。
//
// AccessToken 非空时走 XOAUTH2；要求链路已加密，未加密直接报错而非降级，
// 避免明文外发 Bearer 令牌（令牌等价于长期凭证）。
//
// 明文链路上交出凭证要同时满足两件事，缺一不可：
//
//  1. 用户选的是 SecurityNone（内网中继、自建 MTA、测试服务器）。
//     net/smtp 的 PlainAuth 在非加密链路上一律拒绝交凭证，这条保护对
//     「本该加密」的场景是对的，但套在一个显式选择明文的配置上，
//     就把界面里的 none 变成了一个选了也发不出信的死胡同。
//  2. **服务器根本没广告 STARTTLS。**
//     这一条很容易漏：本包里 SecurityNone 走的是**机会式 STARTTLS**（见 connect 的
//     default 分支），所以 none 的实际语义是「自动」而不是「我要明文」。
//     服务器广告了 STARTTLS 却升级失败时 secured 同样为 false——那不是
//     「用户选择了明文」，而是「说好了加密却没有」，正是主动 MITM 打断 TLS 握手
//     就能把凭证降级出来的那种情形。此时按降级处理，拒绝交凭证。
func (c *Client) authFor(secured, serverOffersTLS bool) (smtp.Auth, error) {
	if c.config.AccessToken != "" {
		if !secured {
			return nil, errors.New("XOAUTH2 requires an encrypted connection")
		}
		return &xoauth2Auth{c.config.Username, c.config.AccessToken, c.config.Host}, nil
	}
	if secured {
		return &tlsPlainAuth{"", c.config.Username, c.config.Password, c.config.Host}, nil
	}
	if c.config.Security == types.SecurityNone && !serverOffersTLS {
		logger.Warn("smtp: 按配置在未加密链路上发送凭证（服务器未提供 STARTTLS）",
			zap.String("host", c.config.Host), zap.Int("port", c.config.Port))
		return &tlsPlainAuth{"", c.config.Username, c.config.Password, c.config.Host}, nil
	}
	if c.config.Security == types.SecurityNone && serverOffersTLS {
		logger.Warn("smtp: 服务器提供了 STARTTLS 但升级未成功，按降级处理、不发送凭证",
			zap.String("host", c.config.Host), zap.Int("port", c.config.Port))
	}
	return smtp.PlainAuth("", c.config.Username, c.config.Password, c.config.Host), nil
}

// authenticate 在服务器广告了 AUTH 扩展时完成认证，否则跳过。
//
// 不能无条件 Auth：不要求认证的服务器（内网 postfix 中继、本地 MTA、
// 测试用的 GreenMail）根本不广告 AUTH 扩展，对它们发 AUTH 命令本身就是协议错误。
// 而在明文连接上，net/smtp 的 PlainAuth 会拒绝交出凭证——于是表现成
// 「SMTP auth failed: unencrypted connection」，一个既没要求认证、
// 也没法认证的服务器就此发不出任何邮件。RFC 4954 说得很清楚：
// AUTH 是扩展，先看 EHLO 的回应再决定要不要用。
//
// 服务器**广告了** AUTH 时行为不变：明文下 PlainAuth 仍然拒绝，
// 那是保护凭证不被明文送出，不是要绕过的东西。
// 同样，没配用户名就跳过——没有凭证可谈。
func (c *Client) authenticate(conn *smtp.Client, st connState) error {
	if c.config.Username == "" && c.config.AccessToken == "" {
		return nil
	}
	if !advertisesAuth(conn) {
		logger.Warn("smtp: 服务器未广告 AUTH 扩展，跳过认证",
			zap.String("host", c.config.Host), zap.Int("port", c.config.Port))
		return nil
	}
	// 加密处境一律取自 connect 那一次判断，不在这里重新问服务器（见 connState）
	auth, err := c.authFor(st.secured, st.offeredTLS)
	if err != nil {
		return err
	}
	if err := conn.Auth(auth); err != nil {
		return fmt.Errorf("SMTP auth failed: %w", err)
	}
	return nil
}

// advertisesAuth 判断服务器是否广告了 AUTH 能力。
//
// 不能只查 `Extension("AUTH")`：net/smtp 按**空格**切分 EHLO 每一行取 key，
// 于是老式的 `250-AUTH=LOGIN PLAIN` 广告（部分 Exchange 与国内服务商仍在发）
// 存进去的 key 是整串 `AUTH=LOGIN`，精确查 "AUTH" 查不到。
// 漏判的后果是静默跳过认证，最终错在 MAIL FROM 上的 530，与真正的原因隔了一层。
//
// 名单只列了四个，**不是待补全的 TODO**：net/smtp 不导出 ext map，只能逐个精确查，
// 而漏掉的那些形式（AUTH=GSSAPI / AUTH=DIGEST-MD5 …）对应的机制本包根本执行不了
// （只实现了 PLAIN 与 XOAUTH2）——扫到了也只能拿 PLAIN 去撞，照样失败。
// 所以漏判的全部代价仅仅是错误信息从「认证失败」变成「MAIL FROM 530」，
// 而真正要救的那一类（只发 AUTH=LOGIN / AUTH=PLAIN 的老式 Exchange 与国内服务商）
// 已经在名单里。别再往下加了。
func advertisesAuth(conn *smtp.Client) bool {
	if ok, _ := conn.Extension("AUTH"); ok {
		return true
	}
	for _, k := range []string{"AUTH=LOGIN", "AUTH=PLAIN", "AUTH=CRAM-MD5", "AUTH=NTLM"} {
		if ok, _ := conn.Extension(k); ok {
			return true
		}
	}
	return false
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
	conn, st, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Quit()

	if err := c.authenticate(conn, st); err != nil {
		return err
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
	conn, st, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Quit()

	if err := c.authenticate(conn, st); err != nil {
		return err
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
	conn, st, err := c.connect()
	if err != nil {
		return err
	}
	defer conn.Quit()

	if err := c.authenticate(conn, st); err != nil {
		return err
	}
	return nil
}

// connState 描述一条建好的连接在加密上的处境。
//
// 两个字段必须出自**同一次**判断：secured 是「实际加密了吗」，offeredTLS 是
// 「服务器提供过加密吗」，authFor 要靠两者的组合区分「用户选了明文」与「本该加密却降级」。
// 各问各的会埋下一类隐蔽的错配——比如 net/smtp 的 Extension() 查表用大写、
// 而存 key 时保留 EHLO 原样，服务器回小写的 `250-starttls` 就会被漏判；
// 判定只有一处时，这种偏差至少是自洽的（连接层没升级，认证层也认为没提供），
// 将来要加兜底也只需改一个地方。
type connState struct {
	// secured 报告链路是否已加密（隐式 TLS 或 STARTTLS 升级成功）
	secured bool
	// offeredTLS 报告服务器是否广告了 STARTTLS（无论升级成没成功）
	offeredTLS bool
}

// connect establishes an SMTP connection respecting SecurityMode and proxy settings.
func (c *Client) connect() (*smtp.Client, connState, error) {
	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
	tlsConfig := &tls.Config{ServerName: c.config.Host}

	var zero connState
	switch c.config.Security {
	case types.SecuritySSL:
		rawConn, err := c.dial("tcp", addr)
		if err != nil {
			return nil, zero, fmt.Errorf("connect failed: %w", err)
		}
		tlsConn := tls.Client(rawConn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			rawConn.Close()
			return nil, zero, fmt.Errorf("TLS handshake failed: %w", err)
		}
		client, err := smtp.NewClient(tlsConn, c.config.Host)
		if err != nil {
			return nil, zero, err
		}
		return client, connState{secured: true, offeredTLS: true}, nil

	case types.SecurityStartTLS:
		rawConn, err := c.dial("tcp", addr)
		if err != nil {
			return nil, zero, fmt.Errorf("connect failed: %w", err)
		}
		client, err := smtp.NewClient(rawConn, c.config.Host)
		if err != nil {
			rawConn.Close()
			return nil, zero, err
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			client.Quit()
			return nil, zero, fmt.Errorf("STARTTLS failed: %w", err)
		}
		return client, connState{secured: true, offeredTLS: true}, nil

	default: // SecurityNone — try opportunistic STARTTLS
		rawConn, err := c.dial("tcp", addr)
		if err != nil {
			return nil, zero, fmt.Errorf("connect failed: %w", err)
		}
		client, err := smtp.NewClient(rawConn, c.config.Host)
		if err != nil {
			rawConn.Close()
			return nil, zero, err
		}
		secured := false
		offeredTLS, _ := client.Extension("STARTTLS")
		if offeredTLS {
			if err := client.StartTLS(tlsConfig); err != nil {
				// 不中断连接（仍可明文投递），但必须记出来：
				// 这条路径会让 authFor 按降级处理、拒绝发送凭证，
				// 吞掉错误的话，用户只会看到「认证失败」而不知道是 TLS 没握上手。
				logger.Warn("smtp: 服务器广告了 STARTTLS 但升级失败，继续以明文连接",
					zap.String("host", c.config.Host), zap.Int("port", c.config.Port),
					zap.Error(err))
			} else {
				secured = true
			}
		}
		return client, connState{secured: secured, offeredTLS: offeredTLS}, nil
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
