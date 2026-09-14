package imap

import (
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	imapv2 "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"golang.org/x/net/proxy"

	"flymail-core/types"
)

// Session represents an authenticated IMAP connection.
// All IMAP operations go through a Session. The caller is responsible for
// calling Close() when done.
type Session struct {
	Client       *imapclient.Client
	Config       types.IMAPConfig
	Capabilities []string
	SupportsIDLE bool
	SecurityMode string

	mu          sync.Mutex
	idleHandler func(event IDLEEvent) // set via SetIDLEHandler
}

// IDLEEvent represents an unsolicited server update during IDLE.
type IDLEEvent struct {
	Kind    string // "expunge", "mailbox", "exists"
	SeqNum  uint32
	NumMsgs *uint32 // new message count (for "exists" / mailbox updates)
}

// Dial connects to the IMAP server, authenticates, and returns a ready Session.
func Dial(cfg types.IMAPConfig) (*Session, error) {
	s := &Session{Config: cfg}

	rawConn, err := dial(cfg)
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}

	security := cfg.Security
	if security == "" {
		security = types.SecurityNone
	}
	s.SecurityMode = string(security)

	opts := &imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Expunge: func(seqNum uint32) {
				s.dispatchIDLE(IDLEEvent{Kind: "expunge", SeqNum: seqNum})
			},
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				ev := IDLEEvent{Kind: "mailbox"}
				if data != nil && data.NumMessages != nil {
					ev.NumMsgs = data.NumMessages
				}
				s.dispatchIDLE(ev)
			},
		},
	}

	tlsConfig := &tls.Config{ServerName: cfg.Host}

	switch security {
	case types.SecuritySSL:
		tlsConn := tls.Client(rawConn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			rawConn.Close()
			return nil, fmt.Errorf("TLS handshake failed: %w", err)
		}
		s.Client = imapclient.New(tlsConn, opts)

	case types.SecurityStartTLS:
		optsWithTLS := *opts
		optsWithTLS.TLSConfig = tlsConfig
		c, err := imapclient.NewStartTLS(rawConn, &optsWithTLS)
		if err != nil {
			rawConn.Close()
			return nil, fmt.Errorf("STARTTLS failed: %w", err)
		}
		s.Client = c
		s.SecurityMode = "starttls"

	default:
		s.Client = imapclient.New(rawConn, opts)
	}

	if err := s.Client.WaitGreeting(); err != nil {
		s.Client.Close()
		return nil, fmt.Errorf("greeting failed: %w", err)
	}

	// Login
	if cfg.AccessToken != "" {
		saslClient := newXOAuth2Client(cfg.Username, cfg.AccessToken)
		if err := s.Client.Authenticate(saslClient); err != nil {
			s.Client.Close()
			return nil, fmt.Errorf("XOAUTH2 auth failed: %w", err)
		}
	} else {
		if err := s.Client.Login(cfg.Username, cfg.Password).Wait(); err != nil {
			s.Client.Close()
			return nil, fmt.Errorf("login failed: %w", err)
		}
	}

	// Send IMAP ID for 163/126/yeah servers
	if is163Server(cfg.Host) {
		s.sendIMAPID()
	}

	// Read capabilities
	s.readCapabilities()

	return s, nil
}

// CanIDLE 报告服务器是否支持 IDLE（基于已读取的 capabilities）。
func (s *Session) CanIDLE() bool { return s.SupportsIDLE }

// Close logs out and closes the connection.
// Noop 发一条 NOOP，用来确认这条连接**此刻还活着**。
//
// 为什么需要它：IMAP 连接会被服务端、NAT 网关、负载均衡静默掐掉，而 TCP 那侧
// 不一定立刻有反馈——go-imap 在读到 EOF 后会把底层 conn 关掉，但调用方手上的
// *Session 仍然非 nil、看着像能用。下一条真正的命令才撞上
// 「use of closed network connection」，而那时错误已经弹到用户脸上了。
//
// 复用池化连接前先探一次活，比「失败后重试」安全：重试可能把一条已经送达
// 服务端、只是响应丢了的 MOVE / EXPUNGE 再执行一遍。NOOP 探活最坏只是白跑
// 一个来回，不会重复任何有副作用的操作。
//
// 它当然也有竞态（连接可能在 NOOP 之后、真命令之前才死），但那个窗口是毫秒级、
// 而不是「空闲三分钟后必然发生」——把一个确定的故障降成一个罕见的故障。
func (s *Session) Noop() error {
	if s.Client == nil {
		return fmt.Errorf("not connected")
	}
	return s.Client.Noop().Wait()
}

func (s *Session) Close() error {
	if s.Client == nil {
		return nil
	}
	err := s.Client.Logout().Wait()
	s.Client.Close()
	s.Client = nil
	return err
}

// SetIDLEHandler registers a callback for unsolicited server updates.
// Must be called before StartIDLE.
func (s *Session) SetIDLEHandler(fn func(IDLEEvent)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idleHandler = fn
}

func (s *Session) dispatchIDLE(ev IDLEEvent) {
	s.mu.Lock()
	fn := s.idleHandler
	s.mu.Unlock()
	if fn != nil {
		fn(ev)
	}
}

func (s *Session) readCapabilities() {
	caps := s.Client.Caps()
	if caps == nil {
		return
	}
	for cap := range caps {
		s.Capabilities = append(s.Capabilities, string(cap))
	}
	s.SupportsIDLE = caps.Has(imapv2.CapIdle) || caps.Has(imapv2.CapIMAP4rev2)
}

func (s *Session) sendIMAPID() {
	caps := s.Client.Caps()
	if caps == nil || !caps.Has(imapv2.CapID) {
		return
	}

	name := s.Config.ClientName
	if name == "" {
		name = "MailDev"
	}
	vendor := s.Config.ClientVendor
	if vendor == "" {
		vendor = name
	}

	s.Client.ID(&imapv2.IDData{
		Name:    name,
		Version: "1.0.0",
		Vendor:  vendor,
	}).Wait()
}

// dial creates a raw TCP connection, optionally through a proxy.
func dial(cfg types.IMAPConfig) (net.Conn, error) {
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))

	if cfg.Proxy != nil && cfg.Proxy.Enabled() {
		return dialProxy(cfg.Proxy, addr)
	}
	return net.DialTimeout("tcp", addr, 15*time.Second)
}

func dialProxy(p *types.ProxyConfig, addr string) (net.Conn, error) {
	switch p.Type {
	case "socks5":
		proxyAddr := fmt.Sprintf("%s:%d", p.Host, p.Port)
		var auth *proxy.Auth
		if p.Username != "" {
			auth = &proxy.Auth{User: p.Username, Password: p.Password}
		}
		dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("socks5 init failed: %w", err)
		}
		return dialer.Dial("tcp", addr)
	default:
		return nil, fmt.Errorf("unsupported proxy type: %s", p.Type)
	}
}

func is163Server(host string) bool {
	h := strings.ToLower(host)
	return strings.Contains(h, "163.com") ||
		strings.Contains(h, "126.com") ||
		strings.Contains(h, "yeah.net") ||
		strings.Contains(h, "yeah.com")
}
