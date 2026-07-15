// Package e2e 提供基于 GreenMail 的端到端测试。
// 通过 E2E_GREENMAIL=1 门控；Windows 本机用仓库根 e2e.ps1 运行，Linux/CI 用 e2e.sh。
package e2e

import (
	"os"
	"strconv"
	"testing"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func greenmailHost() string { return envOr("GREENMAIL_HOST", "localhost") }

func greenmailIMAPPort() int {
	if n, err := strconv.Atoi(os.Getenv("GREENMAIL_IMAP_PORT")); err == nil && n > 0 {
		return n
	}
	return 3143
}

func greenmailIMAPAddr() string {
	return greenmailHost() + ":" + strconv.Itoa(greenmailIMAPPort())
}

func greenmailSMTPPort() int {
	if n, err := strconv.Atoi(os.Getenv("GREENMAIL_SMTP_PORT")); err == nil && n > 0 {
		return n
	}
	return 3025
}

func greenmailSMTPAddr() string {
	return greenmailHost() + ":" + strconv.Itoa(greenmailSMTPPort())
}

func greenmailAPIBase() string {
	return "http://" + greenmailHost() + ":" + envOr("GREENMAIL_API_PORT", "8080")
}

// requireE2E 在未启用 E2E（无 GreenMail）时跳过测试。
func requireE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("E2E_GREENMAIL") == "" {
		t.Skip("set E2E_GREENMAIL=1 to run (requires GreenMail; use ./e2e.ps1 or ./e2e.sh)")
	}
}
