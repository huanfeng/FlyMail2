package sync

// 覆盖正文预取的三档模式与范围限制。

import (
	"path/filepath"
	"testing"
	"time"

	coreimap "flymail-core/imap"
	"flymail-core/types"
	imapv2 "github.com/emersion/go-imap/v2"

	"flymail/internal/database"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
)

// bodyFakeSession 按 UID 返回带正文的邮件，并记录每次 FETCH 请求的 UID。
type bodyFakeSession struct {
	mgrFakeSession
	selected  []string
	fetched   []imapv2.UID
	failFetch bool
}

func (s *bodyFakeSession) SelectFolder(path string) (*coreimap.SelectedFolder, error) {
	s.selected = append(s.selected, path)
	return &coreimap.SelectedFolder{Path: path}, nil
}

func (s *bodyFakeSession) FetchByUIDs(uids []imapv2.UID, _ coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	if s.failFetch {
		return nil, errFakeFetch
	}
	s.fetched = append(s.fetched, uids...)
	out := make([]*types.ParsedEmail, 0, len(uids))
	for _, u := range uids {
		out = append(out, &types.ParsedEmail{UID: uint32(u), TextBody: "body of " + string(rune('0'+u%10))})
	}
	return out, nil
}

var errFakeFetch = &fetchErr{}

type fetchErr struct{}

func (*fetchErr) Error() string { return "fetch failed" }

// newBodyManager 组装一个可直接调用预取方法的 Manager（不起 runner）。
func newBodyManager(t *testing.T) (*Manager, *folder.Repository, *message.Repository) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	frepo := folder.NewRepository(db)
	mrepo := message.NewRepository(db)
	m := NewManager(
		&fakeAccountLister{},
		folder.NewService(frepo),
		message.NewService(mrepo, message.NewBodyRepository(db)),
		nil,
	)
	return m, frepo, mrepo
}

func bSeedFolder(t *testing.T, frepo *folder.Repository, path, ftype string) *folder.Folder {
	t.Helper()
	f := &folder.Folder{AccountID: 1, Path: path, DisplayName: path, Type: ftype, Selectable: true}
	if err := frepo.UpsertByPath(f); err != nil {
		t.Fatalf("seed folder: %v", err)
	}
	return f
}

func bSeedMsg(t *testing.T, mrepo *message.Repository, folderID uint, uid uint32, date time.Time) uint {
	t.Helper()
	m := &message.Message{AccountID: 1, FolderID: folderID, UID: uid, Date: date}
	if err := mrepo.Upsert(m); err != nil {
		t.Fatalf("seed msg: %v", err)
	}
	return m.ID
}

func TestBodySyncModeDefaults(t *testing.T) {
	m, _, _ := newBodyManager(t)
	// 未注入配置时的兜底
	if got := m.bodySyncMode(); got != bodyModeNew {
		t.Errorf("默认模式 = %q, want new", got)
	}
	if got := m.bodySyncRecentDays(); got != 30 {
		t.Errorf("默认天数 = %d, want 30", got)
	}
	// 非法取值回落到 new，避免设置被写坏时行为不可预期
	m.SetBodySyncProviders(func() string { return "garbage" }, func() int { return 0 })
	if got := m.bodySyncMode(); got != bodyModeNew {
		t.Errorf("非法模式 = %q, want new", got)
	}
	if got := m.bodySyncRecentDays(); got != 30 {
		t.Errorf("非正天数 = %d, want 30", got)
	}
}

func TestPrefetchNewBodiesOnlyTouchesThisRound(t *testing.T) {
	m, frepo, mrepo := newBodyManager(t)
	inbox := bSeedFolder(t, frepo, "INBOX", "inbox")
	old := bSeedMsg(t, mrepo, inbox.ID, 1, time.Now().Add(-72*time.Hour))
	fresh := bSeedMsg(t, mrepo, inbox.ID, 2, time.Now())

	sess := &bodyFakeSession{}
	// AfterID = old：只有 id > old 的才算这一轮新增
	m.prefetchNewBodies(1, inbox, &message.NewMail{Count: 1, AfterID: old}, sess)

	if len(sess.fetched) != 1 || sess.fetched[0] != 2 {
		t.Fatalf("应只抓新邮件 uid=2，实际 %v", sess.fetched)
	}
	if got, _ := mrepo.GetByID(fresh); !got.BodySynced {
		t.Error("新邮件应已标记 body_synced")
	}
	if got, _ := mrepo.GetByID(old); got.BodySynced {
		t.Error("历史邮件不应被 new 模式回补")
	}
}

func TestPrefetchNewBodiesSkipsBaselineImport(t *testing.T) {
	m, frepo, mrepo := newBodyManager(t)
	inbox := bSeedFolder(t, frepo, "INBOX", "inbox")
	bSeedMsg(t, mrepo, inbox.ID, 1, time.Now())

	sess := &bodyFakeSession{}
	// 基线导入：新账户第一次把历史邮件拉进来，不该立刻触发全量正文下载
	m.prefetchNewBodies(1, inbox, &message.NewMail{Count: 500, Baseline: true, AfterID: 0}, sess)

	if len(sess.fetched) != 0 {
		t.Errorf("基线导入不应预取正文，实际抓了 %v", sess.fetched)
	}
}

func TestPrefetchNewBodiesSkipsExcludedFolders(t *testing.T) {
	m, frepo, mrepo := newBodyManager(t)
	archive := bSeedFolder(t, frepo, "[Gmail]/All Mail", "archive")
	bSeedMsg(t, mrepo, archive.ID, 1, time.Now())

	sess := &bodyFakeSession{}
	m.prefetchNewBodies(1, archive, &message.NewMail{Count: 1, AfterID: 0}, sess)

	if len(sess.fetched) != 0 {
		t.Errorf("archive 不参与预取，实际抓了 %v", sess.fetched)
	}
}

func TestPrefetchHistoryModes(t *testing.T) {
	m, frepo, mrepo := newBodyManager(t)
	inbox := bSeedFolder(t, frepo, "INBOX", "inbox")
	label := bSeedFolder(t, frepo, "Work", "custom")
	junk := bSeedFolder(t, frepo, "Junk", "junk")

	recent := bSeedMsg(t, mrepo, inbox.ID, 1, time.Now().Add(-2*24*time.Hour))
	oldOne := bSeedMsg(t, mrepo, inbox.ID, 2, time.Now().Add(-100*24*time.Hour))
	labelOne := bSeedMsg(t, mrepo, label.ID, 3, time.Now().Add(-1*24*time.Hour))
	junkOne := bSeedMsg(t, mrepo, junk.ID, 4, time.Now())

	t.Run("new 模式不回补历史", func(t *testing.T) {
		m.SetBodySyncProviders(func() string { return bodyModeNew }, func() int { return 30 })
		sess := &bodyFakeSession{}
		m.prefetchHistoryBodies(1, sess, nil)
		if len(sess.fetched) != 0 {
			t.Errorf("new 模式不应回补，实际 %v", sess.fetched)
		}
	})

	t.Run("recent 模式只补窗口内且跳过垃圾箱", func(t *testing.T) {
		m.SetBodySyncProviders(func() string { return bodyModeRecent }, func() int { return 30 })
		sess := &bodyFakeSession{}
		m.prefetchHistoryBodies(1, sess, nil)

		got := map[imapv2.UID]bool{}
		for _, u := range sess.fetched {
			got[u] = true
		}
		if !got[1] || !got[3] {
			t.Errorf("30 天内的收件箱/自定义邮件应被回补，实际 %v", sess.fetched)
		}
		if got[2] {
			t.Error("100 天前的邮件不该进 30 天窗口")
		}
		if got[4] {
			t.Error("垃圾箱不参与预取")
		}
		if msg, _ := mrepo.GetByID(recent); !msg.BodySynced {
			t.Error("窗口内邮件应已落正文")
		}
		if msg, _ := mrepo.GetByID(labelOne); !msg.BodySynced {
			t.Error("自定义文件夹邮件应已落正文")
		}
		if msg, _ := mrepo.GetByID(junkOne); msg.BodySynced {
			t.Error("垃圾箱邮件不该落正文")
		}
	})

	t.Run("all 模式补上更早的历史", func(t *testing.T) {
		m.SetBodySyncProviders(func() string { return bodyModeAll }, nil)
		sess := &bodyFakeSession{}
		m.prefetchHistoryBodies(1, sess, nil)

		if msg, _ := mrepo.GetByID(oldOne); !msg.BodySynced {
			t.Error("all 模式应把 100 天前的邮件也补上")
		}
		// 已补过的不重复抓：上一档已落库的邮件不该再出现在请求里
		for _, u := range sess.fetched {
			if u == 1 || u == 3 {
				t.Errorf("已有正文的邮件不该重复抓取: uid=%d", u)
			}
		}
	})
}

func TestFetchBodiesBatchesAndSurvivesFailure(t *testing.T) {
	m, frepo, mrepo := newBodyManager(t)
	inbox := bSeedFolder(t, frepo, "INBOX", "inbox")

	// 超过一个批次，验证分批发起
	msgs := make([]message.Message, 0, bodyFetchBatch+5)
	for i := 1; i <= bodyFetchBatch+5; i++ {
		id := bSeedMsg(t, mrepo, inbox.ID, uint32(i), time.Now())
		msgs = append(msgs, message.Message{ID: id, UID: uint32(i)})
	}

	sess := &bodyFakeSession{}
	if got := m.fetchBodies("INBOX", msgs, sess); got != len(msgs) {
		t.Errorf("落库 %d 封，期望 %d 封", got, len(msgs))
	}
	if len(sess.fetched) != len(msgs) {
		t.Errorf("请求 %d 个 UID，期望 %d 个", len(sess.fetched), len(msgs))
	}

	// FETCH 失败只是少补几封，不返回错误、不该让调用方判定连接故障
	failing := &bodyFakeSession{failFetch: true}
	if got := m.fetchBodies("INBOX", msgs, failing); got != 0 {
		t.Errorf("失败时应落库 0 封，实际 %d", got)
	}
}
