package imap

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"

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

	// conn 是裹在 TLS 之下的超时包装层，用来补上 go-imap 留白的那段读超时
	// （见 timeout.go）。只有 Dial 会构造 Session，所以它一定非 nil；
	// 判空只是为了让手工构造的零值 Session 不至于 panic。
	conn *timeoutConn

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

	// ⚠ 必须裹在 TLS **之下**：TLS 握手发生在 imapclient 接管之前，那一段
	// 今天没有任何上限——服务端接了 TCP 然后一个字节都不回，Handshake()
	// 就永远不返回，整个账户的 runner 从此停摆。
	conn := newTimeoutConn(rawConn)
	s.conn = conn
	// 握手期（TLS / 问候 / LOGIN / ID / CAPABILITY）走绝对时限，
	// 登录成功后再交回按次续期的空闲上限。
	conn.setHardDeadline(handshakeTimeout)
	defer conn.clearHardDeadline()

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
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return nil, fmt.Errorf("TLS handshake failed: %w", err)
		}
		s.Client = imapclient.New(tlsConn, opts)

	case types.SecurityStartTLS:
		optsWithTLS := *opts
		optsWithTLS.TLSConfig = tlsConfig
		c, err := imapclient.NewStartTLS(conn, &optsWithTLS)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("STARTTLS failed: %w", err)
		}
		s.Client = c
		s.SecurityMode = "starttls"

	default:
		s.Client = imapclient.New(conn, opts)
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
	// 探活拖不得：它存在的意义就是「立刻回答这条连接还活着吗」。
	// 落到默认的 5 分钟空闲上限的话，那五分钟里这个账户整个停着，
	// 和它要防的故障没有区别。
	if s.conn != nil {
		return s.conn.withHardDeadline(probeTimeout, func() error {
			return s.Client.Noop().Wait()
		})
	}
	return s.Client.Noop().Wait()
}

func (s *Session) Close() error {
	if s.Client == nil {
		return nil
	}
	// 收尾路径永远不该把调用方挂住——走到 Close 往往正是因为这条连接已经
	// 出问题了，而 LOGOUT 要等服务端回话。
	var err error
	if s.conn != nil {
		err = s.conn.withHardDeadline(logoutTimeout, func() error {
			return s.Client.Logout().Wait()
		})
	} else {
		err = s.Client.Logout().Wait()
	}
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

// newDialer 返回建立 TCP 连接用的拨号器。
//
// KeepAliveConfig 是这里的重点：它让内核在连接静默时主动探测对端，是唯一能在
// **应用层正阻塞在读**的时候仍然发现对端已经消失的机制（IDLE 就是那个场景）。
// 见 timeout.go 里 keepAliveConfig 的说明。
func newDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:         connectTimeout,
		KeepAliveConfig: keepAliveConfig,
	}
}

// dial creates a raw TCP connection, optionally through a proxy.
func dial(cfg types.IMAPConfig) (net.Conn, error) {
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))

	if cfg.Proxy != nil && cfg.Proxy.Enabled() {
		return dialProxy(cfg.Proxy, addr)
	}
	return newDialer().Dial("tcp", addr)
}

func dialProxy(p *types.ProxyConfig, addr string) (net.Conn, error) {
	switch p.Type {
	case "socks5":
		proxyAddr := fmt.Sprintf("%s:%d", p.Host, p.Port)
		var auth *proxy.Auth
		if p.Username != "" {
			auth = &proxy.Auth{User: p.Username, Password: p.Password}
		}
		// ⚠ forward 必须是带超时的拨号器。原先传的是 proxy.Direct——它是
		// 不带任何超时的 net.Dial，代理地址不通时会一直挂着。
		dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, newDialer())
		if err != nil {
			return nil, fmt.Errorf("socks5 init failed: %w", err)
		}
		// SOCKS5 握手本身（认证、CONNECT）也要有上限，否则代理接了连接却
		// 不回应答，同样是永久挂起。x/net 的 socks 拨号器实现了 DialContext。
		if cd, ok := dialer.(proxy.ContextDialer); ok {
			ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
			defer cancel()
			return cd.DialContext(ctx, "tcp", addr)
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
