package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestSync_InboundChain 收信链路：SMTP 种子 → 触发同步 → 文件夹/列表/详情/未读数。
// 不起后台 Manager，用显式 POST /sync 保证确定性。
func TestSync_InboundChain(t *testing.T) {
	requireE2E(t)
	ta := newTestApp(t, false)
	c := newClient(t, ta)
	mb := uniqueMailbox(t)
	acctID := c.createAccount(mb)

	subjects := []string{"seed-1", "seed-2", "seed-3"}
	for i, subj := range subjects {
		sendSeed(t, "seeder@localhost", mb, subj, fmt.Sprintf("seed-body-%d", i+1))
	}

	c.triggerSyncAndWait(acctID, 60*time.Second)

	folders := c.listFolders(acctID)
	inbox := findFolder(folders, "inbox")
	if inbox == nil {
		t.Fatalf("同步后无 inbox 文件夹: %+v", folders)
	}
	if inbox.UnreadCount != 3 {
		t.Errorf("inbox 未读数=%d 期望 3", inbox.UnreadCount)
	}

	msgs := c.listMessages(inbox.ID)
	if len(msgs) != 3 {
		t.Fatalf("邮件数=%d 期望 3: %+v", len(msgs), msgs)
	}
	got := map[string]messageItem{}
	for _, m := range msgs {
		got[m.Subject] = m
		if m.Seen {
			t.Errorf("新邮件 %q 不应已读", m.Subject)
		}
		if m.FromAddr != "seeder@localhost" {
			t.Errorf("发件人=%q 期望 seeder@localhost", m.FromAddr)
		}
	}
	for _, subj := range subjects {
		if _, ok := got[subj]; !ok {
			t.Errorf("缺少主题 %q", subj)
		}
	}

	// 详情：首访按需连 IMAP 抓正文
	d := c.messageDetail(got["seed-2"].ID)
	if !strings.Contains(d.TextBody, "seed-body-2") {
		t.Errorf("详情正文不含种子内容: %q", d.TextBody)
	}
}
