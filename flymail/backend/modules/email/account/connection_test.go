package account_test

import (
	"testing"

	"flymail/modules/email/account"
)

func TestTestConnectionUnreachable(t *testing.T) {
	svc, _, _ := newSvc(t)
	res := svc.TestConnection(account.TestConnectionRequest{
		Email:        "u@example.com",
		Password:     "x",
		IMAPHost:     "127.0.0.1",
		IMAPPort:     1,
		IMAPSecurity: "ssl",
		SMTPHost:     "127.0.0.1",
		SMTPPort:     1,
		SMTPSecurity: "ssl",
	})
	if res.IMAP {
		t.Error("不可达 IMAP 不应成功")
	}
	if res.IMAPError == "" {
		t.Error("应记录 IMAP 错误信息")
	}
	if res.SMTP {
		t.Error("不可达 SMTP 不应成功")
	}
	if res.SMTPError == "" {
		t.Error("应记录 SMTP 错误信息")
	}
}
