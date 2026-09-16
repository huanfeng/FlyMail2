package imap

import (
	"errors"
	"net"
	"os"
	"sync"
	"time"
)

// 连接超时：补上 go-imap 故意留白的那一段。
//
// ── go-imap 已经管住了什么 ─────────────────────────────────────────────────
//
// imapclient 自己设读写超时，分工是：
//
//	响应读到一半     30s   （readResponse 入口处装上）
//	literal（正文）  5min
//	写命令 / 写正文  30s / 5min
//	**等下一条响应的第一个字节    不限**   ← readResponse 退出时清掉
//
// 前几项都够用，问题在最后一行。它必须不限：IDLE 期间服务器可以几十分钟不说话，
// 连接空着等下一条命令时也一样。但这同时意味着——**命令发出去之后，如果服务端
// 一个字节都不回，就会永远卡在那里**。IMAP 连接被 NAT 网关、负载均衡、运营商
// 静默掐断时正是这个形状：TCP 那侧没有 FIN 也没有 RST，本地看连接还在，
// 读永远不返回。没有任何上层逻辑能发现它：既没报错，也没断连。
//
// ── 这里做两件事 ───────────────────────────────────────────────────────────
//
//  1. 把「不限」换成一个上限。go-imap 用 SetReadDeadline(零值) 表达「不限」，
//     这个包装层拦下零值、换成自己的上限，其余取值原样放行——于是它与
//     go-imap 的 30s 响应超时是叠加关系，不是替换关系。
//  2. 握手期给一个绝对时限。TLS 握手、SOCKS5 握手都在 imapclient 接管之前发生，
//     那段今天完全没有上限：服务端接了 TCP 然后不吭声，Dial 就再也不返回。
//
// ── 为什么不是「每条命令自己算超时」 ───────────────────────────────────────
//
// 那需要在 24 个 Session 方法上各绕一层，而且 go-imap 的读取是一条后台
// goroutine 统一收包，"当前是哪条命令"在连接这一层根本看不出来。拦一个
// SetReadDeadline 的零值是同一件事的唯一支点。
var (
	// connectTimeout 建立 TCP 连接的上限（含走代理时连代理的那一段）。
	connectTimeout = 15 * time.Second

	// handshakeTimeout 从拿到 TCP 连接到登录完成的总时限，覆盖 SOCKS5 握手、
	// TLS 握手、问候语、LOGIN、ID、CAPABILITY。
	// 这几步都是固定几个来回，正常在一秒内完成；给到 45 秒是为了容忍慢网络，
	// 而不是为了容忍无响应的服务端。
	handshakeTimeout = 45 * time.Second

	// connIdleTimeout 「等下一条响应的第一个字节」的上限，也就是本文件开头
	// 说的那段留白。
	//
	// ⚠ 它同时是**空闲连接被允许静默持有的时长**：连接闲着的时候，go-imap 的
	// 读 goroutine 一直阻塞在这里。取值必须明显大于调用方持有空闲连接的时间
	// （flymail 的 runner 是 60 秒就关掉），否则会把健康的连接误杀。
	// 反过来取得太大，静默掐断的连接就要挂那么久才被发现。5 分钟是个折中。
	connIdleTimeout = 5 * time.Minute

	// idleHoldTimeout IDLE 期间的同一个上限。IDLE 的本意就是长时间不说话，
	// 必须大于调用方重进 IDLE 的周期（flymail 的 runner 是 29 分钟）。
	// 这条路径上真正的保命手段是 TCP keepalive：它在内核里发探测包，
	// 不依赖应用层有没有在读。
	idleHoldTimeout = 35 * time.Minute

	// probeTimeout NOOP 探活的上限。它的用途就是「立刻回答这条连接还活着吗」，
	// 拖 5 分钟毫无意义——那段时间该账户的同步整个停着。
	probeTimeout = 20 * time.Second

	// logoutTimeout Close 里 LOGOUT 的上限。收尾路径永远不该把调用方挂住：
	// 走到 Close 往往正是因为这条连接已经出问题了。
	logoutTimeout = 10 * time.Second
)

// keepAliveConfig 让内核在连接静默时主动探测对端。
//
// 这是唯一能在**应用层正阻塞读**的时候仍然发现对端已经消失的机制，所以它是
// IDLE 那条路径（应用层上限 35 分钟）的主要保护。Go 的默认值是「开启、空闲
// 15 秒」，但探测间隔与次数取自系统 sysctl，Linux 默认 75 秒 × 9 次，
// 加起来要 11 分钟才判定断开。这里显式收紧到约 90 秒。
var keepAliveConfig = net.KeepAliveConfig{
	Enable:   true,
	Idle:     30 * time.Second,
	Interval: 15 * time.Second,
	Count:    4,
}

// timeoutConn 包在 net.Conn 外面，只改写 SetReadDeadline 的语义。
//
// 它坐在 TLS 之下：Dial 里的顺序是 raw → timeoutConn → tls.Conn → imapclient。
// tls.Conn 的 SetReadDeadline 是直接转发给下层的，所以 go-imap 设的每一次
// 读超时都会经过这里，不管走的是 SSL 还是 STARTTLS。
type timeoutConn struct {
	net.Conn

	mu sync.Mutex
	// idle 是「不限」被替换成的值，随 IDLE 进出切换。
	idle time.Duration
	// hard 非零时是一个绝对时限，压过一切更晚的取值（握手期、探活、登出）。
	hard time.Time
}

func newTimeoutConn(c net.Conn) *timeoutConn {
	return &timeoutConn{Conn: c, idle: connIdleTimeout}
}

// Read / Write：超时即判死，直接关掉连接。
//
// ⚠ 光让这次读返回错误是不够的，这是实现这套超时时最容易踩空的一脚：
// go-imap 的读循环拿到一个非 net.ErrClosed 的错误并不会退出，而是接着调
// readResponse——后者**入口处又把读截止时间推后 30 秒**（它自己的响应超时）。
// 于是一个 5 分钟的上限实际变成 5 分钟零 30 秒，还要指望解码器恰好不再尝试读。
// 行为取决于对方库的内部实现，非常脆。
//
// 关掉连接就没有这个问题：后续读写立刻返回 net.ErrClosed，go-imap 的读循环
// 显式检查这个错误并退出，随后 closeWithError 会把所有待完成的命令一次失败掉；
// 之后新发的命令写不出去，同样立即失败。
//
// 这么做在语义上也是对的：IMAP 是有状态的流式协议，读到一半超时意味着流的位置
// 已经无法确定，这条连接无论如何都不能再用了。
func (c *timeoutConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.killIfTimedOut(err)
	return n, err
}

func (c *timeoutConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.killIfTimedOut(err)
	return n, err
}

func (c *timeoutConn) killIfTimedOut(err error) {
	if err != nil && errors.Is(err, os.ErrDeadlineExceeded) {
		_ = c.Conn.Close()
	}
}

// SetReadDeadline 拦截 go-imap 的读超时设置。
//
// 零值在 go-imap 里表示「不限」（idleReadTimeout = 0），也就是本文件开头说的
// 那段留白，换成 idle 上限；非零值是它自己的 30s / 5min，原样放行。
// hard 非零时取两者里更早的那个——握手期的总时限不能被单步的超时放宽。
func (c *timeoutConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	if t.IsZero() && c.idle > 0 {
		t = time.Now().Add(c.idle)
	}
	if !c.hard.IsZero() && (t.IsZero() || c.hard.Before(t)) {
		t = c.hard
	}
	c.mu.Unlock()
	return c.Conn.SetReadDeadline(t)
}

// setIdleTimeout 改变「不限」被替换成的值，并**立刻重新武装**当前的读超时。
//
// ⚠ 后面那半句是必须的：改变量本身不会影响已经阻塞在 Read 里的那次调用，
// 而 go-imap 的读 goroutine 几乎总是正阻塞着。只有真的调一次 SetReadDeadline
// 才能让内核重新计算那次读的到期时间。
func (c *timeoutConn) setIdleTimeout(d time.Duration) {
	c.mu.Lock()
	c.idle = d
	hard := c.hard
	c.mu.Unlock()

	t := time.Now().Add(d)
	if !hard.IsZero() && hard.Before(t) {
		t = hard
	}
	_ = c.Conn.SetReadDeadline(t)
}

// withHardDeadline 在 d 之内执行 fn，超时则连接上的读写一律失败。
// 用于握手、探活、登出这类「必须有个头」的片段。
func (c *timeoutConn) withHardDeadline(d time.Duration, fn func() error) error {
	c.setHardDeadline(d)
	defer c.clearHardDeadline()
	return fn()
}

func (c *timeoutConn) setHardDeadline(d time.Duration) {
	deadline := time.Now().Add(d)
	c.mu.Lock()
	c.hard = deadline
	c.mu.Unlock()
	_ = c.Conn.SetDeadline(deadline)
}

func (c *timeoutConn) clearHardDeadline() {
	c.mu.Lock()
	c.hard = time.Time{}
	idle := c.idle
	c.mu.Unlock()
	_ = c.Conn.SetWriteDeadline(time.Time{})
	// 读那侧不能真的清成「不限」——那正是要修的东西。
	_ = c.Conn.SetReadDeadline(time.Now().Add(idle))
}

// ConnIdleTimeout 返回连接静默多久之后被判定为已断开。
// 调用方持有空闲连接的时长必须低于它，否则健康连接会被误杀。
func ConnIdleTimeout() time.Duration { return connIdleTimeout }

// IDLEHoldTimeout 返回 IDLE 期间允许的静默时长。
// 调用方重进 IDLE 的周期必须低于它。
func IDLEHoldTimeout() time.Duration { return idleHoldTimeout }
