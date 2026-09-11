package smtp

import (
	"bytes"
	"net/smtp"
	"testing"
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

	secure, err := c.authFor(true)
	if err != nil {
		t.Fatalf("secured=true 不应报错: %v", err)
	}
	if _, ok := secure.(*tlsPlainAuth); !ok {
		t.Fatal("secured=true 应返回 tlsPlainAuth")
	}
	// 未加密：标准 PlainAuth 对非 localhost 会拒绝发送（保持安全语义）。
	plain, err := c.authFor(false)
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

	auth, err := c.authFor(true)
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
	if _, err := c.authFor(false); err == nil {
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
