package e2e

import (
	"os"
	"testing"
)

func TestGreenmailAddrs_Defaults(t *testing.T) {
	os.Unsetenv("GREENMAIL_HOST")
	os.Unsetenv("GREENMAIL_IMAP_PORT")
	os.Unsetenv("GREENMAIL_SMTP_PORT")
	os.Unsetenv("GREENMAIL_API_PORT")
	if got := greenmailIMAPAddr(); got != "localhost:3143" {
		t.Fatalf("IMAP 默认地址错: %q", got)
	}
	if got := greenmailSMTPAddr(); got != "localhost:3025" {
		t.Fatalf("SMTP 默认地址错: %q", got)
	}
	if got := greenmailAPIBase(); got != "http://localhost:8080" {
		t.Fatalf("API 默认地址错: %q", got)
	}
}

func TestGreenmailAddrs_EnvOverride(t *testing.T) {
	t.Setenv("GREENMAIL_HOST", "10.0.0.5")
	t.Setenv("GREENMAIL_IMAP_PORT", "13143")
	if got := greenmailIMAPAddr(); got != "10.0.0.5:13143" {
		t.Fatalf("env 覆盖 IMAP 地址错: %q", got)
	}
	if got := greenmailIMAPPort(); got != 13143 {
		t.Fatalf("env 覆盖 IMAP 端口错: %d", got)
	}
}
