package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestVerbs_Send 发送链路：POST /send（SMTP 经 GreenMail）→ 收件邮箱 IMAP 断言收到。
// 探针结论：GreenMail 无 Sent 文件夹，APPEND 副本为尽力而为（flymail 仅 warn），不断言。
func TestVerbs_Send(t *testing.T) {
	requireE2E(t)
	ta := newTestApp(t, false)
	c := newClient(t, ta)
	sender := uniqueMailbox(t)
	recipient := uniqueMailbox(t)
	acctID := c.createAccount(sender)

	c.send(acctID, []string{recipient}, "verb-send-subject", "<p>hello-from-e2e</p>")

	sess := imapConnect(t, recipient)
	eventually(t, 30*time.Second, 500*time.Millisecond, "收件方 INBOX 收到发送的邮件", func() bool {
		sel, err := sess.SelectFolder("INBOX")
		if err != nil || sel.NumMessages == 0 {
			return false
		}
		emails, err := sess.FetchBySeqRange(1, sel.NumMessages, coreimapFetchHeaders())
		if err != nil {
			return false
		}
		for _, e := range emails {
			if e.Subject == "verb-send-subject" {
				return true
			}
		}
		return false
	})
}

// TestVerbs_Delete 删除链路：GreenMail 无 Trash → flymail 走「无回收站 → EXPUNGE 永久删除」分支，
// 断言邮件从 IMAP INBOX 消失。
func TestVerbs_Delete(t *testing.T) {
	requireE2E(t)
	ta := newTestApp(t, false)
	c := newClient(t, ta)
	mb := uniqueMailbox(t)
	acctID := c.createAccount(mb)
	sendSeed(t, "seeder@localhost", mb, "del-subject", "del-body")
	c.triggerSyncAndWait(acctID, 60*time.Second)

	inbox := findFolder(c.listFolders(acctID), "inbox")
	if inbox == nil {
		t.Fatal("无 inbox")
	}
	msgs := c.listMessages(inbox.ID)
	if len(msgs) != 1 {
		t.Fatalf("邮件数=%d 期望 1", len(msgs))
	}

	c.deleteMessage(msgs[0].ID)

	sess := imapConnect(t, mb)
	eventually(t, 30*time.Second, 500*time.Millisecond, "邮件从 IMAP INBOX 删除(EXPUNGE)", func() bool {
		sel, err := sess.SelectFolder("INBOX")
		return err == nil && sel.NumMessages == 0
	})
	if after := c.listMessages(inbox.ID); len(after) != 0 {
		t.Errorf("API 列表仍有 %d 封,期望 0", len(after))
	}
}

// TestVerbs_Move 移动链路：core/imap 无 CreateFolder，先经 Session.Client(go-imap/v2) CREATE
// 目标文件夹，再同步入库 → POST /move → IMAP 断言邮件到达目标文件夹且 INBOX 清空。
func TestVerbs_Move(t *testing.T) {
	requireE2E(t)
	ta := newTestApp(t, false)
	c := newClient(t, ta)
	mb := uniqueMailbox(t)
	acctID := c.createAccount(mb)

	const target = "E2E-Archive"
	sess := imapConnect(t, mb)
	if err := sess.Client.Create(target, nil).Wait(); err != nil {
		t.Fatalf("CREATE %s: %v", target, err)
	}
	sendSeed(t, "seeder@localhost", mb, "move-subject", "move-body")
	c.triggerSyncAndWait(acctID, 60*time.Second)

	folders := c.listFolders(acctID)
	var dst *folderDTO
	for i := range folders {
		if strings.EqualFold(folders[i].Path, target) {
			dst = &folders[i]
			break
		}
	}
	if dst == nil {
		t.Fatalf("目标文件夹 %s 未同步入库: %+v", target, folders)
	}
	inbox := findFolder(folders, "inbox")
	msgs := c.listMessages(inbox.ID)
	if len(msgs) != 1 {
		t.Fatalf("邮件数=%d 期望 1", len(msgs))
	}

	c.moveMessage(msgs[0].ID, dst.ID)

	eventually(t, 30*time.Second, 500*time.Millisecond, "邮件移动到 "+target, func() bool {
		selDst, err := sess.SelectFolder(target)
		if err != nil || selDst.NumMessages != 1 {
			return false
		}
		selInbox, err := sess.SelectFolder("INBOX")
		return err == nil && selInbox.NumMessages == 0
	})
}
