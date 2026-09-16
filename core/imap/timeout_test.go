package imap

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flymail-core/types"
)

// 连接静默时必须有个头。
//
// ── 缘起 ─────────────────────────────────────────────────────────────────────
//
// IMAP 连接被 NAT 网关、负载均衡、运营商静默掐断时，TCP 那侧既没有 FIN 也没有
// RST：本地看连接还在，读永远不返回。go-imap 给「响应读到一半」装了 30 秒上限，
// 却把「等下一条响应的第一个字节」留成不限——因为 IDLE 和空闲持有都需要它不限。
// 于是「命令发出去、服务端一个字节都不回」这个最常见的形状完全没有兜底。
//
// ── 这些用例钉的是「会返回」，以及「不误杀」 ─────────────────────────────────
//
// 两个方向都会出事，所以两个方向都得钉：
//
//	没有上限  → 账户永久停摆，而且既不报错也不断连，外面看不出异常
//	上限误伤  → IDLE 连接每隔几分钟被自己的超时打断，反复重连，比原缺陷更显眼
//
// 只钉前者是危险的：一个「给所有读都装上 5 秒超时」的实现能让前者全过，
// 而它会把每条 IDLE 连接都打死。
//
// 每条用例都把超时改成毫秒级再跑，并且带独立的整体时限——缺陷复发时
// 测试要**失败**，不能是挂住（挂住在 CI 上只表现为跑不完，没人知道是哪条）。

// withTimeouts 临时改小各个超时，返回恢复函数。
func withTimeouts(t *testing.T, handshake, connIdle, idleHold, probe time.Duration) {
	t.Helper()
	oh, oc, oi, op, ol := handshakeTimeout, connIdleTimeout, idleHoldTimeout, probeTimeout, logoutTimeout
	handshakeTimeout, connIdleTimeout, idleHoldTimeout, probeTimeout = handshake, connIdle, idleHold, probe
	// 收尾的 LOGOUT 也要跟着缩短，否则每条用例结束时都要对着装死的服务端白等
	logoutTimeout = 300 * time.Millisecond
	t.Cleanup(func() {
		handshakeTimeout, connIdleTimeout, idleHoldTimeout, probeTimeout, logoutTimeout = oh, oc, oi, op, ol
	})
}

// mustFinishWithin 在 d 内跑完 fn，否则判定为挂死。
func mustFinishWithin(t *testing.T, d time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s 没有在 %s 内返回——这正是线上「账户永久停摆」的形状", what, d)
	}
}

// ── 假服务端 ────────────────────────────────────────────────────────────────

// silentServer 接受 TCP 连接之后一个字节都不发，模拟被静默掐断的连接。
func silentServer(t *testing.T) (host string, port int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// 收下连接，什么都不回，也不关闭——这就是「静默」
			t.Cleanup(func() { c.Close() })
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

// fakeIMAP 是一个最小 IMAP 服务端：能完成问候与登录，之后按 goSilent 决定
// 是继续应答还是装死。
type fakeIMAP struct {
	ln       net.Listener
	goSilent atomic.Bool // 置位后，对收到的命令一律不回应
	idled    atomic.Bool // 客户端是否正处于 IDLE
	idleTag  string      // 当前 IDLE 命令的 tag，DONE 时要用它应答
}

func startFakeIMAP(t *testing.T) *fakeIMAP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeIMAP{ln: ln}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeIMAP) addr() (string, int) {
	a := f.ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", a.Port
}

func (f *fakeIMAP) serve(c net.Conn) {
	defer c.Close()
	// CAPABILITY 直接放在问候里，省掉一轮往返
	fmt.Fprintf(c, "* OK [CAPABILITY IMAP4rev1 IDLE] fake ready\r\n")

	br := bufio.NewReader(c)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		// ⚠ DONE 必须在切分 tag 之前处理：它是一个没有空格的裸词，
		// 走下面的 len(fields) < 2 会被直接丢掉，Stop 就会一直等下去。
		if strings.EqualFold(line, "DONE") {
			f.idled.Store(false)
			fmt.Fprintf(c, "%s OK IDLE terminated\r\n", f.idleTag)
			continue
		}

		fields := strings.SplitN(line, " ", 3)
		if len(fields) < 2 {
			continue
		}
		tag, cmd := fields[0], strings.ToUpper(fields[1])

		if f.goSilent.Load() {
			continue // 收下命令，不回应答
		}

		switch cmd {
		case "LOGIN":
			fmt.Fprintf(c, "%s OK [CAPABILITY IMAP4rev1 IDLE] LOGIN completed\r\n", tag)
		case "CAPABILITY":
			fmt.Fprintf(c, "* CAPABILITY IMAP4rev1 IDLE\r\n%s OK CAPABILITY completed\r\n", tag)
		case "NOOP":
			fmt.Fprintf(c, "%s OK NOOP completed\r\n", tag)
		case "IDLE":
			f.idleTag = tag
			f.idled.Store(true)
			fmt.Fprintf(c, "+ idling\r\n")
		case "LOGOUT":
			fmt.Fprintf(c, "* BYE\r\n%s OK LOGOUT completed\r\n", tag)
			return
		case "SELECT":
			fmt.Fprintf(c, "* 0 EXISTS\r\n* OK [UIDVALIDITY 1]\r\n* OK [UIDNEXT 1]\r\n%s OK [READ-WRITE] SELECT completed\r\n", tag)
		default:
			fmt.Fprintf(c, "%s OK completed\r\n", tag)
		}
	}
}

func (f *fakeIMAP) dial(t *testing.T) *Session {
	t.Helper()
	host, port := f.addr()
	sess, err := Dial(types.IMAPConfig{
		Host: host, Port: port,
		Username: "u", Password: "p",
		Security: types.SecurityNone,
	})
	if err != nil {
		t.Fatalf("Dial 失败：%v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

// ── 用例 ────────────────────────────────────────────────────────────────────

// 服务端收了 TCP 却不发问候，建连必须失败而不是挂住。
//
// TLS 那条路径是重灾区：握手在 imapclient 接管**之前**发生，原先完全没有上限，
// tls.Handshake() 会一直等下去。而 Dial 是在账户的 runner 那条 goroutine 上调的,
// 它挂住 = 那个账户从此不再同步。
func TestDialFailsWhenServerNeverSpeaks(t *testing.T) {
	for _, sec := range []types.SecurityMode{types.SecurityNone, types.SecuritySSL} {
		t.Run(string(sec), func(t *testing.T) {
			withTimeouts(t, 300*time.Millisecond, time.Minute, time.Minute, time.Minute)
			host, port := silentServer(t)

			var err error
			mustFinishWithin(t, 5*time.Second, "Dial", func() {
				var sess *Session
				sess, err = Dial(types.IMAPConfig{
					Host: host, Port: port,
					Username: "u", Password: "p",
					Security: sec,
				})
				if sess != nil {
					_ = sess.Close()
				}
			})
			if err == nil {
				t.Fatal("服务端一个字节都没发，Dial 却成功了")
			}
		})
	}
}

// 登录之后服务端装死：命令必须在空闲上限内失败。
//
// 这是线上那个形状的直接复现——连接还在、TCP 层健康、命令发得出去，
// 就是永远等不到应答。
func TestCommandFailsWhenServerGoesSilent(t *testing.T) {
	withTimeouts(t, 5*time.Second, 300*time.Millisecond, time.Minute, 5*time.Second)
	f := startFakeIMAP(t)
	sess := f.dial(t)

	// ⚠ 先跑一条**成功**的命令，这一步不能省。
	//
	// 建连收尾时会装一次读超时，只靠它的话，紧接着装死就能被那一次拦住——
	// 用例会通过，但什么都没证明。真正要验的是 go-imap 每读完一条响应就把读超时
	// 清成「不限」之后，我们有没有把它换回上限。
	//
	// ⚠ 前置命令必须是**普通命令**。用 NOOP 不行：它自带硬时限，收尾时同样会
	// 装一次读超时，于是又把拦截逻辑遮住了——拆掉拦截，用例照样绿。
	if _, err := sess.SelectFolder("INBOX"); err != nil {
		t.Fatalf("前置的正常 SELECT 就失败了：%v", err)
	}

	f.goSilent.Store(true)

	var err error
	mustFinishWithin(t, 5*time.Second, "SelectFolder", func() {
		_, err = sess.SelectFolder("INBOX")
	})
	if err == nil {
		t.Fatal("服务端不应答，命令却成功返回了")
	}
}

// NOOP 探活有自己更短的上限：它的用途就是「立刻回答连接还活着吗」。
//
// 探活如果也按 5 分钟等，那五分钟里这个账户整个停着，
// 和它要防的故障没有区别。
func TestNoopProbeHasShorterDeadline(t *testing.T) {
	// 空闲上限故意设得很大，好让「探活没有自己的上限」这种实现暴露出来
	withTimeouts(t, 5*time.Second, 30*time.Second, time.Minute, 300*time.Millisecond)
	f := startFakeIMAP(t)
	sess := f.dial(t)

	if _, err := sess.SelectFolder("INBOX"); err != nil {
		t.Fatalf("前置的正常 SELECT 就失败了：%v", err)
	}
	f.goSilent.Store(true)

	start := time.Now()
	var err error
	mustFinishWithin(t, 5*time.Second, "Noop", func() { err = sess.Noop() })
	if err == nil {
		t.Fatal("服务端不应答，探活却报告连接正常")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("探活等了 %s，说明它用的是空闲上限而不是自己的短上限", elapsed)
	}
}

// ⚠ IDLE 不能被命令期的上限打断。
//
// IDLE 的本意就是长时间不说话。给所有读一律装上短超时的实现能让上面几条全过，
// 却会让每条 IDLE 连接在超时整点被打断、反复重连——比它要修的缺陷更显眼。
func TestIDLESurvivesSilenceLongerThanCommandTimeout(t *testing.T) {
	withTimeouts(t, 5*time.Second, 200*time.Millisecond, 10*time.Second, 5*time.Second)
	f := startFakeIMAP(t)
	sess := f.dial(t)

	if _, err := sess.SelectFolder("INBOX"); err != nil {
		t.Fatalf("SELECT 失败：%v", err)
	}
	h, err := sess.StartIDLE()
	if err != nil {
		t.Fatalf("StartIDLE 失败：%v", err)
	}

	// 静默远超命令期上限（200ms），IDLE 必须还活着
	time.Sleep(1500 * time.Millisecond)
	select {
	case err := <-h.Done():
		t.Fatalf("IDLE 被命令期的读超时打断了：%v", err)
	default:
	}

	if err := h.Stop("test"); err != nil {
		t.Fatalf("Stop 失败：%v", err)
	}
}

// 退出 IDLE 之后上限要收回来，否则静默掐断又变成不可发现的了。
//
// 「进 IDLE 时放宽、出来忘了收」是这个改动最容易留下的尾巴：
// 功能上完全正常，只是保护悄悄失效了，没有任何现象。
func TestCommandTimeoutRestoredAfterIDLE(t *testing.T) {
	withTimeouts(t, 5*time.Second, 300*time.Millisecond, 30*time.Second, 5*time.Second)
	f := startFakeIMAP(t)
	sess := f.dial(t)

	if _, err := sess.SelectFolder("INBOX"); err != nil {
		t.Fatalf("SELECT 失败：%v", err)
	}
	h, err := sess.StartIDLE()
	if err != nil {
		t.Fatalf("StartIDLE 失败：%v", err)
	}
	if err := h.Stop("test"); err != nil {
		t.Fatalf("Stop 失败：%v", err)
	}

	// 出了 IDLE 就该按命令期的 300ms 判死，而不是 IDLE 的 30s
	f.goSilent.Store(true)
	start := time.Now()
	mustFinishWithin(t, 5*time.Second, "IDLE 之后的命令", func() {
		_, _ = sess.SelectFolder("INBOX")
	})
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("IDLE 退出后仍在按 IDLE 的长上限等（%s），保护已经失效", elapsed)
	}
}

// 正常连接不受影响——没有这条，上面所有「会超时」都可以靠「连都连不上」作弊通过。
func TestHealthyConnectionUnaffected(t *testing.T) {
	withTimeouts(t, 5*time.Second, 2*time.Second, 30*time.Second, 2*time.Second)
	f := startFakeIMAP(t)
	sess := f.dial(t)

	if err := sess.Noop(); err != nil {
		t.Fatalf("健康连接上的 NOOP 失败了：%v", err)
	}
	if _, err := sess.SelectFolder("INBOX"); err != nil {
		t.Fatalf("健康连接上的 SELECT 失败了：%v", err)
	}
}

// 空闲上限必须明显大于调用方持有空闲连接的时长。
//
// 这是这套超时唯一的耦合点：go-imap 的读 goroutine 在连接空着的时候一直阻塞在
// 「等下一条响应的第一个字节」上，而我们给这个等待装了上限。上限比调用方
// 持有空闲连接的时间还短的话，健康连接会被定期误杀。
// flymail 那侧的对应断言在 sync 包里（runner 的 idleClose 是 60 秒）。
func TestIdleTimeoutsOrdering(t *testing.T) {
	if connIdleTimeout <= probeTimeout {
		t.Fatalf("空闲上限（%s）不该比探活上限（%s）还短", connIdleTimeout, probeTimeout)
	}
	if idleHoldTimeout <= connIdleTimeout {
		t.Fatalf("IDLE 上限（%s）必须大于命令期上限（%s），否则 IDLE 会被打断",
			idleHoldTimeout, connIdleTimeout)
	}
	if handshakeTimeout >= connIdleTimeout {
		t.Fatalf("握手时限（%s）不该比空闲上限（%s）还长", handshakeTimeout, connIdleTimeout)
	}
	if ConnIdleTimeout() != connIdleTimeout || IDLEHoldTimeout() != idleHoldTimeout {
		t.Fatal("对外暴露的取值与内部不一致，跨模块的断言会失效")
	}
}
