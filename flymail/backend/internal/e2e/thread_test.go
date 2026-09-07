package e2e

import (
	"fmt"
	"net/http"
	"net/smtp"
	"net/url"
	"testing"
	"time"

	imapv2 "github.com/emersion/go-imap/v2"
)

// sendSeedWithHeaders 经 GreenMail SMTP 投递一封带自定义头（Message-ID / In-Reply-To / References）的纯文本邮件。
func sendSeedWithHeaders(t *testing.T, from, to, subject string, headers map[string]string, body string) {
	t.Helper()
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n", from, to, subject)
	for k, v := range headers {
		msg += k + ": " + v + "\r\n"
	}
	msg += "MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + body + "\r\n"
	if err := smtp.SendMail(greenmailSMTPAddr(), nil, from, []string{to}, []byte(msg)); err != nil {
		t.Fatalf("sendSeedWithHeaders to %s: %v", to, err)
	}
}

type threadItem struct {
	ThreadID     string       `json:"thread_id"`
	Count        int          `json:"count"`
	Unread       int          `json:"unread"`
	Subject      string       `json:"subject"`
	LatestID     uint         `json:"latest_id"`
	Participants []addressDTO `json:"participants"`
}

func (c *apiClient) listThreads(folderID uint) (threads []threadItem, total int) {
	c.t.Helper()
	var page struct {
		Threads []threadItem `json:"threads"`
		Total   int          `json:"total"`
	}
	c.mustJSON(http.MethodGet, "/api/v1/folders/"+utoa(folderID)+"/threads", nil, http.StatusOK, &page)
	return page.Threads, page.Total
}

// TestThread_InboundChain 线程链路（GreenMail）：原信 + 两封回复 + 一封无关邮件 → 同步 → 打开正文 →
// 会话列表折叠成 2 行 → 成员按时间正序 → 会话级已读回写到服务器。
//
// ⚠ GreenMail 2.1.8 对 BODY.PEEK[HEADER.FIELDS (...)] 一律返回空内容，ENVELOPE 里也不带 In-Reply-To
// （实测，真实服务器都支持）。因此这里同步刚结束时三封是三条线程；线程头要等正文落库时由 parser 补上
// （StoreParsedBody 的兜底），本用例正是覆盖这条兜底路径。HEADER.FIELDS 路径由 core 单测 + 真实账户验证。
func TestThread_InboundChain(t *testing.T) {
	requireE2E(t)
	ta := newTestApp(t, false)
	c := newClient(t, ta)
	mb := uniqueMailbox(t)
	acctID := c.createAccount(mb)

	root := fmt.Sprintf("<root-%d@e2e.local>", time.Now().UnixNano())
	r1 := fmt.Sprintf("<r1-%d@e2e.local>", time.Now().UnixNano())
	sendSeedWithHeaders(t, "alice@localhost", mb, "thread-root", map[string]string{"Message-ID": root}, "root body")
	sendSeedWithHeaders(t, "bob@localhost", mb, "Re: thread-root", map[string]string{"Message-ID": r1, "In-Reply-To": root, "References": root}, "reply 1")
	sendSeedWithHeaders(t, "alice@localhost", mb, "Re: thread-root", map[string]string{"In-Reply-To": r1, "References": root + " " + r1}, "reply 2")
	sendSeed(t, "carol@localhost", mb, "unrelated", "solo")

	c.triggerSyncAndWait(acctID, 60*time.Second)
	inbox := findFolder(c.listFolders(acctID), "inbox")
	if inbox == nil {
		t.Fatal("no inbox")
	}
	// 同步后每封都必须有线程 id（列表能分组），GreenMail 上此时尚未归并
	threads, total := c.listThreads(inbox.ID)
	if total != 4 || len(threads) != 4 {
		t.Fatalf("after sync: total=%d n=%d %+v", total, len(threads), threads)
	}

	// 打开两封回复的正文（首访按需抓整封）：parser 补线程头 → 归并
	msgs := c.listMessages(inbox.ID)
	for _, m := range msgs {
		if m.Subject == "Re: thread-root" {
			if d := c.messageDetail(m.ID); !d.BodySynced {
				t.Fatalf("body not synced for %d", m.ID)
			}
		}
	}
	threads, total = c.listThreads(inbox.ID)
	if total != 2 || len(threads) != 2 {
		t.Fatalf("after bodies: total=%d n=%d %+v", total, len(threads), threads)
	}
	var conv *threadItem
	for i := range threads {
		if threads[i].Count == 3 {
			conv = &threads[i]
		}
	}
	if conv == nil {
		t.Fatalf("expected a 3-message thread: %+v", threads)
	}
	// 取详情不改已读（标已读是前端动作）；参与者 alice / bob 去重；
	// 三封 INTERNALDATE 同秒，「最新一封」按 (date, id) 取到最后那封回复
	if conv.Unread != 3 || len(conv.Participants) != 2 || conv.Subject != "Re: thread-root" {
		t.Errorf("thread summary: %+v", conv)
	}

	var members struct {
		Messages []messageItem `json:"messages"`
	}
	c.mustJSON(http.MethodGet, "/api/v1/threads/messages?thread_id="+url.QueryEscape(conv.ThreadID), nil, http.StatusOK, &members)
	if len(members.Messages) != 3 || members.Messages[0].Subject != "thread-root" || members.Messages[2].ID != conv.LatestID {
		t.Errorf("members: %+v", members.Messages)
	}

	// 会话级已读：本地立即生效，服务器端经回写队列 eventually 变成 \Seen；无关邮件不受影响
	c.mustJSON(http.MethodPost, "/api/v1/threads/batch/read", map[string]any{"thread_ids": []string{conv.ThreadID}, "read": true}, http.StatusOK, nil)
	threads, _ = c.listThreads(inbox.ID)
	for _, th := range threads {
		if th.ThreadID == conv.ThreadID && th.Unread != 0 {
			t.Errorf("thread should be read locally: %+v", th)
		}
		if th.ThreadID != conv.ThreadID && th.Unread != 1 {
			t.Errorf("unrelated thread must stay unread: %+v", th)
		}
	}
	sess := imapConnect(t, mb)
	if _, err := sess.SelectFolder("INBOX"); err != nil {
		t.Fatal(err)
	}
	eventually(t, 20*time.Second, 300*time.Millisecond, "thread members \\Seen on server", func() bool {
		emails, err := sess.FetchByUIDRange(imapv2.UID(1), 0, coreimapFetchHeaders())
		if err != nil {
			return false
		}
		seen, unrelatedSeen := 0, false
		for _, e := range emails {
			if e.Subject == "unrelated" {
				unrelatedSeen = e.IsRead
			} else if e.IsRead {
				seen++
			}
		}
		return seen == 3 && !unrelatedSeen
	})
}
