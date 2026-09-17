package sync

import (
	"testing"
	"time"

	"flymail/modules/email/message"
)

// ⚠ 摘要必须从库里重新读，不能用同步时那份快照。
//
// nm.Unseen 里的 Message 是**抓元数据时**的副本，那时正文还没下载，Snippet 恒为空。
// 正文预取在通知之前跑，跑完摘要才落库——直接用快照的话，通知里永远没有摘要，
// 而且这种「永远是空」在单元测试里很容易被写成「本来就允许为空」而漏掉。
func TestSnippetOfReadsStoredValue(t *testing.T) {
	m, frepo, mrepo := newBodyManager(t)
	f := bSeedFolder(t, frepo, "INBOX", "inbox")

	msg := &message.Message{
		AccountID: 1, FolderID: f.ID, UID: 1,
		Subject: "会议变更", FromAddr: "a@x.com", Date: time.Now().UTC(),
	}
	if err := mrepo.Upsert(msg); err != nil {
		t.Fatalf("seed message: %v", err)
	}
	// 同步阶段的快照：此刻库里也还没有摘要
	if got := m.snippetOf(msg.ID); got != "" {
		t.Fatalf("前提不成立，正文还没落库就有摘要了：%q", got)
	}

	// 正文预取落库（MarkBodySynced 就是预取路径写摘要的地方）
	if err := mrepo.MarkBodySynced(msg.ID, "下周三上午十点，会议室 A", false); err != nil {
		t.Fatalf("MarkBodySynced: %v", err)
	}

	if got := m.snippetOf(msg.ID); got != "下周三上午十点，会议室 A" {
		t.Errorf("没有读到落库后的摘要，拿到 %q", got)
	}
}

// 取不到摘要不能让通知发不出去。
func TestSnippetOfIsForgiving(t *testing.T) {
	m, _, _ := newBodyManager(t)
	if got := m.snippetOf(0); got != "" {
		t.Errorf("messageID 为 0 时应当返回空串，拿到 %q", got)
	}
	if got := m.snippetOf(999999); got != "" {
		t.Errorf("邮件不存在时应当返回空串，拿到 %q", got)
	}
}
