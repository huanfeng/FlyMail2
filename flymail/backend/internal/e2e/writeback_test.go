package e2e

import (
	"testing"
	"time"

	imapv2 "github.com/emersion/go-imap/v2"
)

// TestWriteback_SeenAndFlagged 回写链路：API 标已读/星标（本地乐观写库 + 异步队列回写 IMAP）
// → core/imap 直连 GreenMail 轮询断言 \Seen / \Flagged 已生效。
func TestWriteback_SeenAndFlagged(t *testing.T) {
	requireE2E(t)
	ta := newTestApp(t, false)
	c := newClient(t, ta)
	mb := uniqueMailbox(t)
	acctID := c.createAccount(mb)
	sendSeed(t, "seeder@localhost", mb, "wb-subject", "wb-body")
	c.triggerSyncAndWait(acctID, 60*time.Second)

	inbox := findFolder(c.listFolders(acctID), "inbox")
	if inbox == nil {
		t.Fatal("无 inbox")
	}
	msgs := c.listMessages(inbox.ID)
	if len(msgs) != 1 {
		t.Fatalf("邮件数=%d 期望 1", len(msgs))
	}
	m := msgs[0]

	sess := imapConnect(t, mb)
	fetchFlags := func() (isRead, isStarred bool, ok bool) {
		if _, err := sess.SelectFolder("INBOX"); err != nil {
			return false, false, false
		}
		emails, err := sess.FetchByUIDs([]imapv2.UID{imapv2.UID(m.UID)}, coreimapFetchHeaders())
		if err != nil || len(emails) != 1 {
			return false, false, false
		}
		return emails[0].IsRead, emails[0].IsStarred, true
	}

	c.markRead(m.ID, true)
	eventually(t, 30*time.Second, 500*time.Millisecond, `\Seen 回写到 IMAP`, func() bool {
		isRead, _, ok := fetchFlags()
		return ok && isRead
	})

	c.markFlagged(m.ID, true)
	eventually(t, 30*time.Second, 500*time.Millisecond, `\Flagged 回写到 IMAP`, func() bool {
		_, isStarred, ok := fetchFlags()
		return ok && isStarred
	})

	// API 侧本地状态同样生效
	after := c.listMessages(inbox.ID)
	if len(after) != 1 || !after[0].Seen || !after[0].Flagged {
		t.Errorf("API 列表状态未更新: %+v", after)
	}
}
