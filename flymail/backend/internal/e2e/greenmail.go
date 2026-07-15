package e2e

import (
	"fmt"
	"net/smtp"
	"sync/atomic"
	"testing"
	"time"

	coreimap "flymail-core/imap"
	"flymail-core/types"
)

var mailboxSeq int64

// uniqueMailbox 生成隔离用唯一收件地址。带时间戳：GreenMail -KeepUp 复用时跨 run 也不冲突。
func uniqueMailbox(t *testing.T) string {
	t.Helper()
	n := atomic.AddInt64(&mailboxSeq, 1)
	return fmt.Sprintf("e2e-%d-%d@localhost", time.Now().UnixNano()%1e9, n)
}

// sendSeed 经 GreenMail SMTP 投递一封纯文本邮件到 to 的 INBOX。
func sendSeed(t *testing.T, from, to, subject, body string) {
	t.Helper()
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n",
		from, to, subject, body)
	if err := smtp.SendMail(greenmailSMTPAddr(), nil, from, []string{to}, []byte(msg)); err != nil {
		t.Fatalf("sendSeed to %s: %v", to, err)
	}
}

// imapConnect 直连 GreenMail 做服务器端断言（auth.disabled：任意密码，首次登录自动建邮箱）。
func imapConnect(t *testing.T, mailbox string) *coreimap.Session {
	t.Helper()
	sess, err := coreimap.Dial(types.IMAPConfig{
		Host:     greenmailHost(),
		Port:     greenmailIMAPPort(),
		Username: mailbox,
		Password: "e2e",
		Security: types.SecurityNone,
	})
	if err != nil {
		t.Fatalf("imapConnect %s: %v", mailbox, err)
	}
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

// coreimapFetchHeaders 仅抓头部的 FetchOptions（服务器端断言用）。
func coreimapFetchHeaders() coreimap.FetchOptions {
	return coreimap.FetchOptions{FetchBody: false, FallbackHeaders: true}
}

// eventually 轮询 cond 直到为真或超时（异步链路统一等待原语，禁止裸 sleep 断言）。
func eventually(t *testing.T, timeout, interval time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(interval)
	}
	t.Fatalf("eventually 超时(%s): %s", timeout, desc)
}
