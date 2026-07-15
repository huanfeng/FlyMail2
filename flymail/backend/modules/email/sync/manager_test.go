package sync

import (
	"context"
	"encoding/json"
	"path/filepath"
	gosync "sync"
	"testing"
	"time"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	"flymail/internal/database"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"

	imapv2 "github.com/emersion/go-imap/v2"
)

// mgrFakeSession 是 Manager 测试用的 IMAP 会话桩，可配置 SELECT/FETCH 结果。
type mgrFakeSession struct {
	listFolders func() ([]types.FolderInfo, error)
	selectFn    func(path string) (*coreimap.SelectedFolder, error)
	fetchRange  func(from, to imapv2.UID) ([]*types.ParsedEmail, error)
}

func (f *mgrFakeSession) ListFolders() ([]types.FolderInfo, error) {
	if f.listFolders != nil {
		return f.listFolders()
	}
	return nil, nil
}

func (f *mgrFakeSession) SelectFolder(path string) (*coreimap.SelectedFolder, error) {
	if f.selectFn != nil {
		return f.selectFn(path)
	}
	return &coreimap.SelectedFolder{Path: path}, nil
}

func (f *mgrFakeSession) FolderStatus(path string, items ...coreimap.StatusItem) (*coreimap.FolderStatusResult, error) {
	return &coreimap.FolderStatusResult{}, nil
}

func (f *mgrFakeSession) FetchByUIDRange(from, to imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	if f.fetchRange != nil {
		return f.fetchRange(from, to)
	}
	return nil, nil
}

func (f *mgrFakeSession) FetchBySeqRange(from, to uint32, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	return nil, nil
}

func (f *mgrFakeSession) FetchByUIDs(uids []imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	return nil, nil
}

func (f *mgrFakeSession) MarkRead(uids ...imapv2.UID) error             { return nil }
func (f *mgrFakeSession) MarkUnread(uids ...imapv2.UID) error           { return nil }
func (f *mgrFakeSession) MarkStarred(uids ...imapv2.UID) error          { return nil }
func (f *mgrFakeSession) MarkUnstarred(uids ...imapv2.UID) error        { return nil }
func (f *mgrFakeSession) Delete(uids ...imapv2.UID) error               { return nil }
func (f *mgrFakeSession) Move(mailbox string, uids ...imapv2.UID) error { return nil }

func (f *mgrFakeSession) CanIDLE() bool                            { return false }
func (f *mgrFakeSession) StartIDLE() (*coreimap.IdleHandle, error) { return nil, nil }
func (f *mgrFakeSession) SetIDLEHandler(func(coreimap.IDLEEvent))  {}

func (f *mgrFakeSession) Close() error { return nil }

// FetchRawMessage 桩实现，manager 测试不涉及附件下载，直接返回空。
func (f *mgrFakeSession) FetchRawMessage(uid imapv2.UID) ([]byte, error) { return nil, nil }

// fakePublisher 线程安全记录发布的事件载荷。
type fakePublisher struct {
	mu       gosync.Mutex
	payloads [][]byte
}

func (p *fakePublisher) Publish(payload []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]byte, len(payload))
	copy(cp, payload)
	p.payloads = append(p.payloads, cp)
}

func (p *fakePublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.payloads)
}

func (p *fakePublisher) at(i int) []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.payloads[i]
}

// fakeAccountLister 实现 AccountLister，ListEnabledIDs 返回值可变。
type fakeAccountLister struct {
	mu       gosync.Mutex
	ids      []uint
	touched  bool
	imapErr  error
	cfgCalls int
}

func (a *fakeAccountLister) setIDs(ids []uint) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ids = ids
}

func (a *fakeAccountLister) ListEnabledIDs() ([]uint, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]uint, len(a.ids))
	copy(out, a.ids)
	return out, nil
}

func (a *fakeAccountLister) IMAPConfig(id uint) (types.IMAPConfig, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfgCalls++
	return types.IMAPConfig{Host: "h"}, a.imapErr
}

func (a *fakeAccountLister) TouchLastSync(id uint, t time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.touched = true
	return nil
}

// newTestDB 建立内存 sqlite 并迁移。
func newTestServices(t *testing.T) (*folder.Service, *message.Service, *folder.Repository, *message.Repository) {
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
	fsvc := folder.NewService(frepo)
	msvc := message.NewService(mrepo, message.NewBodyRepository(db))
	return fsvc, msvc, frepo, mrepo
}

// TestManagerReconcileStartsAndStopsWorkers 验证 reconcile 按启用账户增减 worker。
func TestManagerReconcileStartsAndStopsWorkers(t *testing.T) {
	fsvc, msvc, _, _ := newTestServices(t)
	lister := &fakeAccountLister{ids: []uint{1, 2}}
	m := NewManager(lister, fsvc, msvc, &fakePublisher{})
	// worker 内 CanIDLE=false，runSession 会立即 pollAll（无文件夹，快速返回），
	// 随后阻塞在大 poll 间隔的 select；为防 runSession 快速返回后空转，注入返回错误的 dial
	// 会触发 backoff，从而 worker 阻塞在 backoff timer——也可控。这里让 dial 成功并阻塞在 poll。
	m.SetPollIntervalProvider(func() int { return 3600 })
	m.SetDial(func(cfg types.IMAPConfig) (Session, error) {
		return &mgrFakeSession{}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	waitWorkerCount(t, m, 2)

	lister.setIDs([]uint{1})
	m.reconcile()
	waitWorkerCount(t, m, 1)

	lister.setIDs([]uint{})
	m.reconcile()
	waitWorkerCount(t, m, 0)

	done := make(chan struct{})
	go func() {
		m.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() 卡死")
	}
}

func waitWorkerCount(t *testing.T, m *Manager, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.runnerCount() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("workerCount = %d, want %d", m.runnerCount(), want)
}

// TestManagerPollAllSyncsAndPublishes 验证 pollAll 增量同步并发布 new_mail 事件。
func TestManagerPollAllSyncsAndPublishes(t *testing.T) {
	fsvc, msvc, frepo, mrepo := newTestServices(t)

	// 预置 INBOX：Type=inbox, Selectable=true, TotalCount=10, UIDNext=11。
	if err := frepo.UpsertByPath(&folder.Folder{
		AccountID:  1,
		Path:       "INBOX",
		Type:       "inbox",
		Selectable: true,
		UIDNext:    11,
		TotalCount: 10,
	}); err != nil {
		t.Fatalf("预置 INBOX: %v", err)
	}
	inbox, err := fsvc.FindInbox(1)
	if err != nil || inbox == nil {
		t.Fatalf("FindInbox: %v inbox=%v", err, inbox)
	}

	sess := &mgrFakeSession{
		listFolders: func() ([]types.FolderInfo, error) {
			return []types.FolderInfo{{Name: "INBOX", Path: "INBOX", Attributes: []string{"\\Inbox"}}}, nil
		},
		selectFn: func(path string) (*coreimap.SelectedFolder, error) {
			return &coreimap.SelectedFolder{Path: path, NumMessages: 15, UIDValidity: 0, UIDNext: 16}, nil
		},
		fetchRange: func(from, to imapv2.UID) ([]*types.ParsedEmail, error) {
			// 期望抓 [11,15] 共 5 封。
			out := []*types.ParsedEmail{}
			for u := from; u <= to; u++ {
				out = append(out, &types.ParsedEmail{UID: uint32(u), Subject: "s", Date: time.Now()})
			}
			return out, nil
		},
	}

	lister := &fakeAccountLister{ids: []uint{1}}
	pub := &fakePublisher{}
	m := NewManager(lister, fsvc, msvc, pub)

	_ = m.FullSync(1, sess, nil)

	// DB 新增 5 行：CountByFolder == 15（预置时无邮件，实际 0+5=5）。
	// 注意：预置 TotalCount=10 只是 folder 表字段，message 表此前为空。
	n, _ := mrepo.CountByFolder(inbox.ID)
	if n != 5 {
		t.Errorf("CountByFolder = %d, want 5", n)
	}

	got, _ := fsvc.GetByID(inbox.ID)
	if got.UIDNext != 16 {
		t.Errorf("folder UIDNext = %d, want 16", got.UIDNext)
	}

	if pub.count() != 1 {
		t.Fatalf("publisher count = %d, want 1", pub.count())
	}
	var ev Event
	if err := json.Unmarshal(pub.at(0), &ev); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if ev.Type != "new_mail" {
		t.Errorf("event.Type = %q, want new_mail", ev.Type)
	}
	if ev.NewCount != 5 {
		t.Errorf("event.NewCount = %d, want 5", ev.NewCount)
	}
	if ev.FolderID != inbox.ID {
		t.Errorf("event.FolderID = %d, want %d", ev.FolderID, inbox.ID)
	}
	if !lister.touched {
		t.Errorf("TouchLastSync not called")
	}
}

// TestManagerPollAllNoNewMail 验证无新邮件时不发布事件。
func TestManagerPollAllNoNewMail(t *testing.T) {
	fsvc, msvc, frepo, _ := newTestServices(t)

	if err := frepo.UpsertByPath(&folder.Folder{
		AccountID:  1,
		Path:       "INBOX",
		Type:       "inbox",
		Selectable: true,
		UIDNext:    11,
		TotalCount: 0,
	}); err != nil {
		t.Fatalf("预置 INBOX: %v", err)
	}

	sess := &mgrFakeSession{
		listFolders: func() ([]types.FolderInfo, error) {
			return []types.FolderInfo{{Name: "INBOX", Path: "INBOX", Attributes: []string{"\\Inbox"}}}, nil
		},
		selectFn: func(path string) (*coreimap.SelectedFolder, error) {
			// UIDNext 等于已存值 11，无新邮件。
			return &coreimap.SelectedFolder{Path: path, NumMessages: 0, UIDValidity: 0, UIDNext: 11}, nil
		},
		fetchRange: func(from, to imapv2.UID) ([]*types.ParsedEmail, error) {
			return nil, nil
		},
	}

	lister := &fakeAccountLister{ids: []uint{1}}
	pub := &fakePublisher{}
	m := NewManager(lister, fsvc, msvc, pub)

	_ = m.FullSync(1, sess, nil)

	if pub.count() != 0 {
		t.Errorf("publisher count = %d, want 0", pub.count())
	}
}
