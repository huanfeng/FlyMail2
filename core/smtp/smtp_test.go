package smtp

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"testing"

	"flymail-core/types"
)

// TestTLSPlainAuth_SkipsTLSCheck 验证：连接已加密（隐式 TLS/465）时，即使 ServerInfo.TLS=false
// 也照常发送 PLAIN 凭证（修复 net/smtp 手动包 TLS 后误判"未加密"而拒发的问题）。
func TestTLSPlainAuth_SkipsTLSCheck(t *testing.T) {
	a := &tlsPlainAuth{"", "user@gmail.com", "app-pw", "smtp.gmail.com"}
	proto, resp, err := a.Start(&smtp.ServerInfo{Name: "smtp.gmail.com", TLS: false})
	if err != nil {
		t.Fatalf("已加密连接不应报错，得到: %v", err)
	}
	if proto != "PLAIN" {
		t.Fatalf("proto = %q，期望 PLAIN", proto)
	}
	if want := []byte("\x00user@gmail.com\x00app-pw"); !bytes.Equal(resp, want) {
		t.Fatalf("resp = %q，期望 %q", resp, want)
	}
}

// TestTLSPlainAuth_WrongHost 主机名不匹配仍拒绝（防凭证发往非预期主机）。
func TestTLSPlainAuth_WrongHost(t *testing.T) {
	a := &tlsPlainAuth{"", "u", "p", "smtp.gmail.com"}
	if _, _, err := a.Start(&smtp.ServerInfo{Name: "evil.example.com", TLS: false}); err == nil {
		t.Fatal("主机名不匹配应报错")
	}
}

// TestAuthFor 已加密选 tlsPlainAuth（跳过 TLS 自检），未加密选标准 PlainAuth。
func TestAuthFor(t *testing.T) {
	c := &Client{}
	c.config.Username = "u"
	c.config.Password = "p"
	c.config.Host = "smtp.example.com"

	secure, err := c.authFor(true, false)
	if err != nil {
		t.Fatalf("secured=true 不应报错: %v", err)
	}
	if _, ok := secure.(*tlsPlainAuth); !ok {
		t.Fatal("secured=true 应返回 tlsPlainAuth")
	}
	// 未加密：标准 PlainAuth 对非 localhost 会拒绝发送（保持安全语义）。
	plain, err := c.authFor(false, false)
	if err != nil {
		t.Fatalf("secured=false 不应报错: %v", err)
	}
	if _, _, err := plain.Start(&smtp.ServerInfo{Name: "smtp.example.com", TLS: false}); err == nil {
		t.Fatal("secured=false 且非 TLS 时标准 PlainAuth 应拒绝")
	}
}

// TestAuthFor_XOAuth2 AccessToken 非空时改走 XOAUTH2；未加密链路必须拒绝而非降级，
// 否则 Bearer 令牌会以明文出网（令牌等价于长期凭证）。
func TestAuthFor_XOAuth2(t *testing.T) {
	c := &Client{}
	c.config.Username = "u@gmail.com"
	c.config.AccessToken = "ya29.token"
	c.config.Password = "ignored"
	c.config.Host = "smtp.gmail.com"

	auth, err := c.authFor(true, false)
	if err != nil {
		t.Fatalf("secured=true 不应报错: %v", err)
	}
	mech, ir, err := auth.Start(&smtp.ServerInfo{Name: "smtp.gmail.com", TLS: true})
	if err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	if mech != "XOAUTH2" {
		t.Fatalf("机制应为 XOAUTH2，实际 %q", mech)
	}
	want := []byte("user=u@gmail.com\x01auth=Bearer ya29.token\x01\x01")
	if !bytes.Equal(ir, want) {
		t.Fatalf("初始响应不符\n want %q\n  got %q", want, ir)
	}
	if _, err := c.authFor(false, false); err == nil {
		t.Fatal("未加密链路携带令牌时必须拒绝")
	}
}

// TestXOAuth2_WrongHost 令牌同样不得发往非预期主机。
func TestXOAuth2_WrongHost(t *testing.T) {
	a := &xoauth2Auth{"u", "tok", "smtp.gmail.com"}
	if _, _, err := a.Start(&smtp.ServerInfo{Name: "evil.example.com", TLS: true}); err == nil {
		t.Fatal("主机名不匹配应报错")
	}
}

// TestXOAuth2_Next 服务端返回错误质询（一段 JSON）时回空响应，把交互推进到明确失败，
// 而不是让连接挂在质询状态。
func TestXOAuth2_Next(t *testing.T) {
	a := &xoauth2Auth{"u", "tok", "h"}
	resp, err := a.Next([]byte(`{"status":"401"}`), true)
	if err != nil {
		t.Fatalf("Next 不应报错: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("质询应回空响应，实际 %q", resp)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// authenticate：AUTH 是扩展，先看 EHLO 的回应再决定用不用
// ─────────────────────────────────────────────────────────────────────────────

// fakeSMTP 起一个最小的假 SMTP 服务端，只回应握手所需的几条命令。
// advertiseAuth 控制 EHLO 的回应里有没有 AUTH 扩展——这正是被测行为的分水岭。
// 返回地址与一个「是否收到过 AUTH 命令」的标记（读取前先等 done）。
func fakeSMTP(t *testing.T, advertiseAuth bool) (addr string, sawAuth *bool, done chan struct{}) {
	t.Helper()
	// 只监听回环：不这么写会在开发机上弹防火墙授权框
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	seen := false
	fin := make(chan struct{})
	go func() {
		defer close(fin)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		fmt.Fprintf(conn, "220 fake ESMTP\r\n")
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				fmt.Fprintf(conn, "250-fake\r\n")
				if advertiseAuth {
					fmt.Fprintf(conn, "250-AUTH PLAIN LOGIN\r\n")
				}
				fmt.Fprintf(conn, "250 8BITMIME\r\n")
			case strings.HasPrefix(cmd, "AUTH"):
				seen = true
				fmt.Fprintf(conn, "235 ok\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				fmt.Fprintf(conn, "221 bye\r\n")
				return
			default:
				fmt.Fprintf(conn, "250 ok\r\n")
			}
		}
	}()
	return ln.Addr().String(), &seen, fin
}

// dialFake 连上假服务端并完成 EHLO，返回可直接交给 authenticate 的客户端。
func dialFake(t *testing.T, addr string) *smtp.Client {
	t.Helper()
	conn, err := smtp.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.Hello("localhost"); err != nil {
		t.Fatalf("ehlo: %v", err)
	}
	return conn
}

func TestAuthenticate_SkipsWhenServerDoesNotAdvertiseAuth(t *testing.T) {
	// 不要求认证的服务器（内网中继、本地 MTA、GreenMail）不广告 AUTH。
	// 从前这里无条件发 AUTH，明文下 PlainAuth 拒绝交凭证，
	// 结果是「既没要求认证也没法认证」的服务器一封信都发不出去。
	addr, sawAuth, done := fakeSMTP(t, false)
	c := &Client{config: types.SMTPConfig{Host: "localhost", Username: "u", Password: "p"}}
	conn := dialFake(t, addr)

	if err := c.authenticate(conn, connState{}); err != nil {
		t.Fatalf("服务器没广告 AUTH 时不该报错，得到：%v", err)
	}
	_ = conn.Quit()
	<-done
	if *sawAuth {
		t.Error("服务器没广告 AUTH，却仍然发了 AUTH 命令")
	}
}

func TestAuthenticate_StillRefusesPlainCredentialsOverCleartext(t *testing.T) {
	// 服务器广告了 AUTH 但链路是明文：PlainAuth 拒绝交出凭证是**正确**的，
	// 这条保护不能被上一条的放宽顺手抹掉。
	addr, _, _ := fakeSMTP(t, true)
	c := &Client{config: types.SMTPConfig{Host: "example.com", Username: "u", Password: "p"}}
	conn := dialFake(t, addr)

	err := c.authenticate(conn, connState{})
	if err == nil {
		t.Fatal("明文连接上不该交出 PLAIN 凭证")
	}
	if !strings.Contains(err.Error(), "SMTP auth failed") {
		t.Errorf("错误信息应点明是认证失败，得到：%v", err)
	}
}

func TestAuthenticate_SkipsWhenNoCredentials(t *testing.T) {
	// 没配用户名也没有令牌：没有凭证可谈，哪怕服务器广告了 AUTH 也不必认证
	addr, sawAuth, done := fakeSMTP(t, true)
	c := &Client{config: types.SMTPConfig{Host: "localhost"}}
	conn := dialFake(t, addr)

	if err := c.authenticate(conn, connState{}); err != nil {
		t.Fatalf("无凭证时不该报错，得到：%v", err)
	}
	_ = conn.Quit()
	<-done
	if *sawAuth {
		t.Error("没有凭证，却仍然发了 AUTH 命令")
	}
}

func TestAuthFor_ExplicitCleartextIsHonored(t *testing.T) {
	// 用户在界面上把安全模式选成 none（内网中继 / 自建 MTA / 测试服务器）：
	// 按他选的来。否则那个选项就是个选了也发不出信的死胡同——
	// net/smtp 的 PlainAuth 会在非加密链路上一律拒绝交凭证。
	c := &Client{config: types.SMTPConfig{
		Host: "relay.internal", Port: 25, Username: "u", Password: "p",
		Security: types.SecurityNone,
	}}
	auth, err := c.authFor(false, false)
	if err != nil {
		t.Fatalf("显式明文不该报错：%v", err)
	}
	if _, ok := auth.(*tlsPlainAuth); !ok {
		t.Fatalf("显式明文应跳过 net/smtp 的 TLS 自检，得到 %T", auth)
	}
	// 真的能交出凭证，而不是在 Start 阶段被拦下
	if _, _, err := auth.Start(&smtp.ServerInfo{Name: "relay.internal", TLS: false}); err != nil {
		t.Errorf("显式明文下 Start 不该失败：%v", err)
	}
}

func TestAuthFor_DowngradedCleartextStillRefused(t *testing.T) {
	// 意图是 STARTTLS 却没能加密（协商失败等降级）：仍然拒绝交凭证。
	// 「说好了加密却没有」与「用户明确选择明文」是两回事，
	// 上一条的放宽不能顺手把这条也放掉。
	c := &Client{config: types.SMTPConfig{
		Host: "mail.example.com", Port: 587, Username: "u", Password: "p",
		Security: types.SecurityStartTLS,
	}}
	auth, err := c.authFor(false, false)
	if err != nil {
		t.Fatalf("authFor 本身不该报错：%v", err)
	}
	if _, ok := auth.(*tlsPlainAuth); ok {
		t.Fatal("降级到明文时不该跳过 TLS 自检")
	}
	if _, _, err := auth.Start(&smtp.ServerInfo{Name: "mail.example.com", TLS: false}); err == nil {
		t.Error("降级到明文时应拒绝交出凭证")
	}
}

func TestAuthFor_NoneButServerOffersTLS_Refused(t *testing.T) {
	// 本包里 SecurityNone 走的是**机会式 STARTTLS**（connect 的 default 分支），
	// 所以 none 的语义是「自动」而不是「我要明文」。服务器广告了 STARTTLS 却没升级成功时
	// secured 同样是 false——那不是用户选择了明文，而是「说好了加密却没有」，
	// 正是主动 MITM 打断 TLS 握手就能把凭证降级出来的那种情形。
	c := &Client{config: types.SMTPConfig{
		Host: "relay.example.com", Port: 25, Username: "u", Password: "p",
		Security: types.SecurityNone,
	}}
	auth, err := c.authFor(false, true) // 服务器提供了 STARTTLS
	if err != nil {
		t.Fatalf("authFor 本身不该报错：%v", err)
	}
	if _, ok := auth.(*tlsPlainAuth); ok {
		t.Fatal("服务器提供了 STARTTLS 却没升级成功时，不该跳过 TLS 自检")
	}
	if _, _, err := auth.Start(&smtp.ServerInfo{Name: "relay.example.com", TLS: false}); err == nil {
		t.Error("降级链路上不该交出凭证")
	}
}

// TestAdvertisesAuth_OldStyleEquals 老式 `AUTH=LOGIN PLAIN` 广告也要认出来。
//
// net/smtp 按空格切 EHLO 每行取 key，于是这种广告存进去的 key 是整串 `AUTH=LOGIN`，
// 精确查 "AUTH" 查不到。漏判会静默跳过认证，最终错在 MAIL FROM 的 530 上，
// 与真正的原因隔了一层。部分 Exchange 与国内服务商仍在发这种形式。
func TestAdvertisesAuth_OldStyleEquals(t *testing.T) {
	for _, tc := range []struct {
		name      string
		advertise string
		want      bool
	}{
		{"标准形式", "250-AUTH PLAIN LOGIN\r\n", true},
		{"老式等号形式", "250-AUTH=LOGIN PLAIN\r\n", true},
		{"完全不广告", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			defer ln.Close()
			go func() {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				br := bufio.NewReader(conn)
				fmt.Fprintf(conn, "220 fake ESMTP\r\n")
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					cmd := strings.ToUpper(strings.TrimSpace(line))
					switch {
					case strings.HasPrefix(cmd, "EHLO"):
						fmt.Fprintf(conn, "250-fake\r\n")
						if tc.advertise != "" {
							fmt.Fprint(conn, tc.advertise)
						}
						fmt.Fprintf(conn, "250 8BITMIME\r\n")
					case strings.HasPrefix(cmd, "QUIT"):
						fmt.Fprintf(conn, "221 bye\r\n")
						return
					default:
						fmt.Fprintf(conn, "250 ok\r\n")
					}
				}
			}()

			conn, err := smtp.Dial(ln.Addr().String())
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer conn.Close()
			if err := conn.Hello("localhost"); err != nil {
				t.Fatalf("ehlo: %v", err)
			}
			if got := advertisesAuth(conn); got != tc.want {
				t.Errorf("advertisesAuth = %v，期望 %v（广告行：%q）", got, tc.want, tc.advertise)
			}
		})
	}
}
