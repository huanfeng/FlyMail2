package main

import (
	"os"
	"strconv"
	"testing"

	coreimap "flymail-core/imap"
	"flymail-core/types"
)

// TestDiagInbox 诊断：连真实账户，打印 INBOX 的 SELECT / STATUS / FETCH 真实返回值。
// 仅在设置 FLYMAIL_SMOKE_* 时运行：
//
//	$env:FLYMAIL_SMOKE_EMAIL='you@163.com'; $env:FLYMAIL_SMOKE_PW='授权码';
//	$env:FLYMAIL_SMOKE_IMAP_HOST='imap.163.com'; $env:FLYMAIL_SMOKE_IMAP_PORT='993';
//	go test -run TestDiagInbox -v
func TestDiagInbox(t *testing.T) {
	email := os.Getenv("FLYMAIL_SMOKE_EMAIL")
	if email == "" {
		t.Skip("set FLYMAIL_SMOKE_EMAIL / FLYMAIL_SMOKE_PW / FLYMAIL_SMOKE_IMAP_HOST[/_PORT] to run")
	}
	port := 993
	if p, err := strconv.Atoi(os.Getenv("FLYMAIL_SMOKE_IMAP_PORT")); err == nil && p > 0 {
		port = p
	}
	cfg := types.IMAPConfig{
		Host:         os.Getenv("FLYMAIL_SMOKE_IMAP_HOST"),
		Port:         port,
		Username:     email,
		Password:     os.Getenv("FLYMAIL_SMOKE_PW"),
		Security:     types.SecuritySSL,
		ClientName:   "FlyMail",
		ClientVendor: "FlyMail",
	}
	sess, err := coreimap.Dial(cfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer sess.Close()
	t.Logf("已连接；SupportsIDLE=%v 安全模式=%s", sess.SupportsIDLE, sess.SecurityMode)

	// 1) SELECT INBOX
	sel, err := sess.SelectFolder("INBOX")
	if err != nil {
		t.Fatalf("SelectFolder(INBOX): %v", err)
	}
	t.Logf("SELECT INBOX -> NumMessages=%d UIDValidity=%d UIDNext=%d", sel.NumMessages, sel.UIDValidity, sel.UIDNext)

	// 2) STATUS INBOX（看 STATUS 是否给出 UIDNext/NumMessages）
	st, err := sess.FolderStatus("INBOX", coreimap.StatusNumMessages, coreimap.StatusUIDNext, coreimap.StatusUIDValidity, coreimap.StatusUnseen)
	if err != nil {
		t.Logf("STATUS INBOX 失败: %v", err)
	} else {
		dump := func(name string, p *uint32) string {
			if p == nil {
				return name + "=<nil>"
			}
			return name + "=" + strconv.FormatUint(uint64(*p), 10)
		}
		t.Logf("STATUS INBOX -> %s %s %s %s",
			dump("NumMessages", st.NumMessages), dump("UIDNext", st.UIDNext),
			dump("UIDValidity", st.UIDValidity), dump("Unseen", st.Unseen))
	}

	// 3) 按序号抓最近 5 封（验证修复路径：163 无 UIDNEXT 时用 seq 抓取，且响应仍带真实 UID）
	if sel.NumMessages > 0 {
		from := sel.NumMessages - 4
		if sel.NumMessages < 5 {
			from = 1
		}
		emails, err := sess.FetchBySeqRange(from, sel.NumMessages, coreimap.FetchOptions{FetchBody: false, FallbackHeaders: true})
		if err != nil {
			t.Logf("FetchBySeqRange(%d,%d) 失败: %v", from, sel.NumMessages, err)
		} else {
			t.Logf("FetchBySeqRange(%d,%d) 取到 %d 封（应带真实 UID）：", from, sel.NumMessages, len(emails))
			for _, e := range emails {
				addr := ""
				if len(e.From) > 0 {
					addr = e.From[0].Email
				}
				t.Logf("  uid=%d seen=%v from=%q subject=%q", e.UID, e.IsRead, addr, e.Subject)
			}
		}
	}
}
