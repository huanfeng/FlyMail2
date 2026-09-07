package sync

import (
	"testing"
	"time"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	imapv2 "github.com/emersion/go-imap/v2"

	"flymail/modules/email/folder"
	"flymail/modules/email/message"
)

// fakeRules 记录 Manager 传进来的批次，并可选地删掉一封模拟「规则把它移走了」。
type fakeRules struct {
	calls   [][]message.Message
	folders []string
	remove  *message.Repository
}

func (f *fakeRules) Handled(accountID uint, msgs []message.Message) map[uint]bool {
	out := map[uint]bool{}
	for _, m := range msgs {
		if m.Subject == "handled" {
			out[m.ID] = true
		}
	}
	return out
}

func (f *fakeRules) Apply(accountID uint, fl *folder.Folder, msgs []message.Message) (bool, error) {
	f.calls = append(f.calls, msgs)
	f.folders = append(f.folders, fl.Type)
	if f.remove != nil && len(msgs) > 0 {
		_ = f.remove.DeleteByIDs([]uint{msgs[0].ID})
		_ = f.remove.SetSeenByIDs([]uint{msgs[len(msgs)-1].ID}, true)
		return true, nil
	}
	return false, nil
}

// TestManagerRunsRulesAfterIncrementalSync 验证规则引擎的接入点：
// 基线导入不跑；之后的增量同步只把本轮新邮件（且只有收件箱）交给引擎；
// 引擎改动过邮件后文件夹计数与通知用的未读集合按执行后状态重算。
func TestManagerRunsRulesAfterIncrementalSync(t *testing.T) {
	fsvc, msvc, frepo, mrepo := newTestServices(t)
	for _, f := range []folder.Folder{
		{AccountID: 1, Path: "INBOX", Type: "inbox", Selectable: true},
		{AccountID: 1, Path: "Archive", Type: "custom", Selectable: true},
	} {
		if err := frepo.UpsertByPath(&f); err != nil {
			t.Fatal(err)
		}
	}
	inbox, _ := fsvc.FindInbox(1)

	uidNext := uint32(4)
	sess := &mgrFakeSession{
		listFolders: func() ([]types.FolderInfo, error) {
			return []types.FolderInfo{
				{Name: "INBOX", Path: "INBOX", Attributes: []string{"\\Inbox"}},
				{Name: "Archive", Path: "Archive"},
			}, nil
		},
		selectFn: func(path string) (*coreimap.SelectedFolder, error) {
			return &coreimap.SelectedFolder{Path: path, NumMessages: uidNext - 1, UIDValidity: 1, UIDNext: uidNext}, nil
		},
		fetchRange: func(from, to imapv2.UID) ([]*types.ParsedEmail, error) {
			out := []*types.ParsedEmail{}
			for u := from; u <= to && uint32(u) < uidNext; u++ {
				out = append(out, &types.ParsedEmail{UID: uint32(u), Subject: "s", Date: time.Now()})
			}
			return out, nil
		},
	}
	rules := &fakeRules{remove: mrepo}
	m := NewManager(&fakeAccountLister{ids: []uint{1}}, fsvc, msvc, &fakePublisher{})
	m.SetRuleRunner(rules)
	var emitted []string
	m.SetEmitter(func(eventType string, accountID uint, messageID uint, title, body string) {
		emitted = append(emitted, title+"|"+body)
	})

	// 第一轮：基线导入 3 封，规则不跑
	if err := m.FullSync(1, sess, nil); err != nil {
		t.Fatal(err)
	}
	if len(rules.calls) != 0 {
		t.Fatalf("baseline import must not run rules: %d calls", len(rules.calls))
	}

	// 第二轮：服务器又来 3 封（uid 4..6），两个文件夹都同步，但只有收件箱那批进引擎
	uidNext = 7
	if err := m.FullSync(1, sess, nil); err != nil {
		t.Fatal(err)
	}
	if len(rules.calls) != 1 || rules.folders[0] != "inbox" || len(rules.calls[0]) != 3 || rules.calls[0][0].UID != 4 {
		t.Fatalf("rules should get exactly the inbox batch of new mail: folders=%v calls=%d", rules.folders, len(rules.calls))
	}
	// 引擎删了 1 封、标读 1 封 → 文件夹计数与通知按执行后状态：5 封、未读 4，通知说「4 封」
	f, _ := fsvc.GetByID(inbox.ID)
	if f.TotalCount != 5 || f.UnreadCount != 4 {
		t.Errorf("folder counts after rules: total=%d unread=%d", f.TotalCount, f.UnreadCount)
	}
	// 通知闸门看到的是重算后的未读数（本轮新增 3 封 → 删 1 标读 1 → 1 封未读，单封提醒带主题）
	if len(emitted) != 1 || emitted[0] != "新邮件 · |s" {
		t.Errorf("notification should reflect post-rule unseen set: %v", emitted)
	}
}

// TestManagerSuppressesHandledInCustomFolder：Gmail 标签文件夹里同一封「已被规则处理过」的邮件不再提醒。
func TestManagerSuppressesHandledInCustomFolder(t *testing.T) {
	fsvc, msvc, frepo, _ := newTestServices(t)
	if err := frepo.UpsertByPath(&folder.Folder{AccountID: 1, Path: "Work", Type: "custom", Selectable: true, UIDNext: 1}); err != nil {
		t.Fatal(err)
	}
	subjects := map[uint32]string{1: "handled", 2: "fresh"}
	sess := &mgrFakeSession{
		listFolders: func() ([]types.FolderInfo, error) {
			return []types.FolderInfo{{Name: "Work", Path: "Work"}}, nil
		},
		selectFn: func(path string) (*coreimap.SelectedFolder, error) {
			return &coreimap.SelectedFolder{Path: path, NumMessages: 2, UIDValidity: 1, UIDNext: 3}, nil
		},
		fetchRange: func(from, to imapv2.UID) ([]*types.ParsedEmail, error) {
			out := []*types.ParsedEmail{}
			for u := from; u <= to && u <= 2; u++ {
				out = append(out, &types.ParsedEmail{UID: uint32(u), Subject: subjects[uint32(u)], MessageID: subjects[uint32(u)] + "@x", Date: time.Now()})
			}
			return out, nil
		},
	}
	m := NewManager(&fakeAccountLister{ids: []uint{1}}, fsvc, msvc, &fakePublisher{})
	m.SetRuleRunner(&fakeRules{})
	var emitted []string
	m.SetEmitter(func(eventType string, accountID uint, messageID uint, title, body string) {
		emitted = append(emitted, title+"|"+body)
	})
	if err := m.FullSync(1, sess, nil); err != nil {
		t.Fatal(err)
	}
	// 两封新未读，其中 "handled" 已被规则处理过 → 只提醒 "fresh" 那一封（单封文案带主题）
	if len(emitted) != 1 || emitted[0] != "新邮件 · |fresh" {
		t.Errorf("handled message must not be announced again: %v", emitted)
	}
}
