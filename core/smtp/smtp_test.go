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

	if _, ok := c.authFor(true).(*tlsPlainAuth); !ok {
		t.Fatal("secured=true 应返回 tlsPlainAuth")
	}
	// 未加密：标准 PlainAuth 对非 localhost 会拒绝发送（保持安全语义）。
	if _, _, err := c.authFor(false).Start(&smtp.ServerInfo{Name: "smtp.example.com", TLS: false}); err == nil {
		t.Fatal("secured=false 且非 TLS 时标准 PlainAuth 应拒绝")
	}
}
