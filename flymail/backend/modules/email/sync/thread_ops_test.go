package sync_test

import (
	"testing"
	"time"

	"flymail/modules/email/message"
)

// seedThreadMsg 种一封带线程 id 的邮件（会话级操作只看 thread_id，不需要真实头）。
func seedThreadMsg(t *testing.T, mrepo *message.Repository, accountID, folderID uint, uid uint32, tid string, seen bool) uint {
	t.Helper()
	m := &message.Message{AccountID: accountID, FolderID: folderID, UID: uid, MessageID: "m" + tid, ThreadID: tid, Seen: seen, Date: time.Now()}
	if err := mrepo.Upsert(m); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return m.ID
}

// TestThreadOpsScope 验证会话级操作的作用范围：
// 已读作用于全部成员；删除在文件夹视图只动该文件夹，在聚合视图排除已发送。
func TestThreadOpsScope(t *testing.T) {
	svc, frepo, mrepo, sess := newMailops(t)
	inboxID := seedFolder(t, frepo, 1, "INBOX", "inbox")
	sentID := seedFolder(t, frepo, 1, "Sent", "sent")
	seedFolder(t, frepo, 1, "Trash", "trash")
	// 线程 T：收件箱 2 封（1 未读）+ 已发送 1 封；线程 U：收件箱 1 封（未读）
	a := seedThreadMsg(t, mrepo, 1, inboxID, 1, "1:t", false)
	b := seedThreadMsg(t, mrepo, 1, inboxID, 2, "1:t", true)
	s := seedThreadMsg(t, mrepo, 1, sentID, 1, "1:t", true)
	u := seedThreadMsg(t, mrepo, 1, inboxID, 3, "1:u", false)

	// 已读：T 全部成员（含 Sent）
	if err := svc.ThreadSetRead([]string{"1:t"}, true); err != nil {
		t.Fatalf("ThreadSetRead: %v", err)
	}
	for _, id := range []uint{a, b, s} {
		if m, _ := mrepo.GetByID(id); !m.Seen {
			t.Errorf("message %d should be seen", id)
		}
	}
	if m, _ := mrepo.GetByID(u); m.Seen {
		t.Errorf("thread U untouched")
	}

	// 星标：同样全员
	if err := svc.ThreadSetFlagged([]string{"1:t", "1:u"}, true); err != nil {
		t.Fatalf("ThreadSetFlagged: %v", err)
	}
	for _, id := range []uint{a, b, s, u} {
		if m, _ := mrepo.GetByID(id); !m.Flagged {
			t.Errorf("message %d should be flagged", id)
		}
	}

	// 聚合视图删除（不带 in_folder_id）：动收件箱两封，Sent 那封留下
	if err := svc.ThreadDelete([]string{"1:t"}, 0); err != nil {
		t.Fatalf("ThreadDelete: %v", err)
	}
	if sess.movedTo != "Trash" || len(sess.movedUIDs) != 2 {
		t.Errorf("aggregate delete should move 2 inbox messages to Trash: movedTo=%q uids=%v", sess.movedTo, sess.movedUIDs)
	}
	if _, err := mrepo.GetByID(s); err != nil {
		t.Errorf("sent copy must survive aggregate delete: %v", err)
	}
	if n, _ := mrepo.CountByFolder(inboxID, message.Filter{}); n != 1 {
		t.Errorf("inbox should keep only thread U: %d", n)
	}

	// 文件夹视图删除（in_folder_id = Sent）：只删 Sent 里的那封
	sess.movedUIDs = nil
	if err := svc.ThreadDelete([]string{"1:t"}, sentID); err != nil {
		t.Fatalf("ThreadDelete in folder: %v", err)
	}
	if len(sess.movedUIDs) != 1 || sess.movedUIDs[0] != 1 {
		t.Errorf("folder-scoped delete should touch exactly the Sent member: %v", sess.movedUIDs)
	}
	if _, err := mrepo.GetByID(s); err == nil {
		t.Errorf("sent member should be gone after folder-scoped delete")
	}

	// 未知 thread_id：无成员，静默无操作
	if err := svc.ThreadDelete([]string{"1:nope"}, 0); err != nil {
		t.Errorf("unknown thread should be a no-op: %v", err)
	}
}

// TestThreadMoveScope 验证会话级移动：文件夹视图只挪该文件夹成员；跨账户拒绝。
func TestThreadMoveScope(t *testing.T) {
	svc, frepo, mrepo, sess := newMailops(t)
	inboxID := seedFolder(t, frepo, 1, "INBOX", "inbox")
	sentID := seedFolder(t, frepo, 1, "Sent", "sent")
	archiveID := seedFolder(t, frepo, 1, "Archive", "archive")
	seedThreadMsg(t, mrepo, 1, inboxID, 1, "1:t", false)
	seedThreadMsg(t, mrepo, 1, sentID, 1, "1:t", true)

	if err := svc.ThreadMove([]string{"1:t"}, archiveID, inboxID); err != nil {
		t.Fatalf("ThreadMove: %v", err)
	}
	if sess.movedTo != "Archive" || len(sess.movedUIDs) != 1 {
		t.Errorf("should move only the inbox member: movedTo=%q uids=%v", sess.movedTo, sess.movedUIDs)
	}
	if n, _ := mrepo.CountByFolder(sentID, message.Filter{}); n != 1 {
		t.Errorf("sent member must stay: %d", n)
	}

	otherInbox := seedFolder(t, frepo, 2, "INBOX", "inbox")
	seedThreadMsg(t, mrepo, 2, otherInbox, 1, "2:x", false)
	if err := svc.ThreadMove([]string{"2:x"}, archiveID, 0); err == nil {
		t.Errorf("cross-account thread move should be rejected")
	}
}
