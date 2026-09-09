package e2e

import (
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"testing"
	"time"
)

// sendSeedHTML 经 GreenMail SMTP 投递一封 HTML 邮件。
func sendSeedHTML(t *testing.T, from, to, subject, html string) {
	t.Helper()
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n",
		from, to, subject, html)
	if err := smtp.SendMail(greenmailSMTPAddr(), nil, from, []string{to}, []byte(msg)); err != nil {
		t.Fatalf("sendSeedHTML to %s: %v", to, err)
	}
}

type privacyDetail struct {
	messageDetail
	RemoteCount   int  `json:"remote_count"`
	RemoteAllowed bool `json:"remote_allowed"`
}

// TestPrivacy_SanitizedDetail：含脚本与追踪像素的 HTML 邮件，详情默认剥脚本、换占位符；
// ?remote=1 保留远程引用；发件人加入信任名单后默认也保留。
func TestPrivacy_SanitizedDetail(t *testing.T) {
	requireE2E(t)
	ta := newTestApp(t, false)
	c := newClient(t, ta)
	mb := uniqueMailbox(t)
	acctID := c.createAccount(mb)
	sendSeedHTML(t, "news@localhost", mb, "tracked",
		`<html><body onload="alert(1)"><script>alert(2)</script><p style="color:red">hello</p>`+
			`<img src="https://track.example/pixel.gif" width="1" height="1"><a href="javascript:alert(3)">x</a></body></html>`)
	c.triggerSyncAndWait(acctID, 60*time.Second)
	inbox := findFolder(c.listFolders(acctID), "inbox")
	msgs := c.listMessages(inbox.ID)
	if len(msgs) != 1 {
		t.Fatalf("messages: %+v", msgs)
	}
	id := msgs[0].ID

	var d privacyDetail
	c.mustJSON(http.MethodGet, "/api/v1/messages/"+utoa(id), nil, http.StatusOK, &d)
	low := strings.ToLower(d.HTMLBody)
	if strings.Contains(low, "<script") || strings.Contains(low, "onload") || strings.Contains(low, "javascript:") || strings.Contains(low, "track.example") {
		t.Errorf("default detail must be sanitized and blocked: %q", d.HTMLBody)
	}
	if d.RemoteCount != 1 || d.RemoteAllowed || !strings.Contains(d.HTMLBody, "hello") || !strings.Contains(d.HTMLBody, "color:red") {
		t.Errorf("default detail: count=%d allowed=%v body=%q", d.RemoteCount, d.RemoteAllowed, d.HTMLBody)
	}

	c.mustJSON(http.MethodGet, "/api/v1/messages/"+utoa(id)+"?remote=1", nil, http.StatusOK, &d)
	if !d.RemoteAllowed || !strings.Contains(d.HTMLBody, "https://track.example/pixel.gif") || strings.Contains(strings.ToLower(d.HTMLBody), "<script") {
		t.Errorf("remote=1 should keep the image but not scripts: allowed=%v body=%q", d.RemoteAllowed, d.HTMLBody)
	}

	// 信任发件人后默认就放行
	c.mustJSON(http.MethodPost, "/api/v1/privacy/trusted-senders", map[string]string{"address": "NEWS@localhost"}, http.StatusCreated, nil)
	c.mustJSON(http.MethodPost, "/api/v1/privacy/trusted-senders", map[string]string{"address": "news@localhost"}, http.StatusConflict, nil)
	c.mustJSON(http.MethodGet, "/api/v1/messages/"+utoa(id), nil, http.StatusOK, &d)
	if !d.RemoteAllowed || !strings.Contains(d.HTMLBody, "https://track.example/pixel.gif") {
		t.Errorf("trusted sender should be allowed by default: %+v", d)
	}
	var list struct {
		Senders []struct {
			ID      uint   `json:"id"`
			Address string `json:"address"`
		} `json:"senders"`
	}
	c.mustJSON(http.MethodGet, "/api/v1/privacy/trusted-senders", nil, http.StatusOK, &list)
	if len(list.Senders) != 1 || list.Senders[0].Address != "news@localhost" {
		t.Fatalf("senders: %+v", list.Senders)
	}
	c.mustJSON(http.MethodDelete, "/api/v1/privacy/trusted-senders/"+utoa(list.Senders[0].ID), nil, http.StatusOK, nil)
	c.mustJSON(http.MethodGet, "/api/v1/messages/"+utoa(id), nil, http.StatusOK, &d)
	if d.RemoteAllowed {
		t.Errorf("after removing trust, remote must be blocked again")
	}
}

// TestPrivacy_LoginRateLimit：连续 10 次错误密码后第 11 次返回 429（哪怕密码正确），带 Retry-After。
func TestPrivacy_LoginRateLimit(t *testing.T) {
	requireE2E(t)
	ta := newTestApp(t, false)
	c := newClient(t, ta)
	for i := 1; i <= 10; i++ {
		resp, _ := c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": adminUser, "password": "wrong-" + utoa(uint(i))})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d, want 401", i, resp.StatusCode)
		}
	}
	resp, body := c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{"username": adminUser, "password": adminPass})
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" || !strings.Contains(string(body), "retry_after") {
		t.Fatalf("11th attempt: status %d retry-after=%q body=%s", resp.StatusCode, resp.Header.Get("Retry-After"), body)
	}
}
