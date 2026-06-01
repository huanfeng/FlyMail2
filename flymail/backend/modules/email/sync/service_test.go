package sync_test

import (
	"errors"
	"path/filepath"
	gosync "sync"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	syncmod "flymail/modules/email/sync"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	imapv2 "github.com/emersion/go-imap/v2"
)

// fakeSession 实现 syncmod.Session 接口，线程安全记录方法调用。
type fakeSession struct {
	mu              gosync.Mutex
	markReadUIDs    []imapv2.UID
	markUnreadUIDs  []imapv2.UID
	markStarredUIDs []imapv2.UID
	markUnstarred   []imapv2.UID
}

func (f *fakeSession) ListFolders() ([]types.FolderInfo, error) {
	return []types.FolderInfo{
		{Name: "Inbox", Path: "INBOX", Attributes: []string{"\\Inbox"}},
		{Name: "Sent", Path: "Sent", Attributes: []string{"\\Sent"}},
	}, nil
}

func (f *fakeSession) SelectFolder(path string) (*coreimap.SelectedFolder, error) {
	return &coreimap.SelectedFolder{Path: path, NumMessages: 2, UIDValidity: 1, UIDNext: 3}, nil
}

func (f *fakeSession) FolderStatus(path string, items ...coreimap.StatusItem) (*coreimap.FolderStatusResult, error) {
	v := uint32(1)
	return &coreimap.FolderStatusResult{UIDValidity: &v}, nil
}

func (f *fakeSession) FetchByUIDRange(from, to imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	return []*types.ParsedEmail{
		{UID: 1, Subject: "a", Date: time.Now()},
		{UID: 2, Subject: "b", Date: time.Now()},
	}, nil
}

func (f *fakeSession) FetchBySeqRange(from, to uint32, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	return []*types.ParsedEmail{
		{UID: 1, Subject: "a", Date: time.Now()},
		{UID: 2, Subject: "b", Date: time.Now()},
	}, nil
}

// FetchByUIDs 返回一封带正文的 ParsedEmail（供 MessageDetail 测试使用）。
func (f *fakeSession) FetchByUIDs(uids []imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	emails := make([]*types.ParsedEmail, 0, len(uids))
	for _, uid := range uids {
		emails = append(emails, &types.ParsedEmail{
			UID:      uint32(uid),
			Subject:  "test subject",
			Date:     time.Now(),
			TextBody: "hello text",
			HTMLBody: "<p>hello html</p>",
		})
	}
	return emails, nil
}

func (f *fakeSession) MarkRead(uids ...imapv2.UID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markReadUIDs = append(f.markReadUIDs, uids...)
	return nil
}

func (f *fakeSession) MarkUnread(uids ...imapv2.UID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markUnreadUIDs = append(f.markUnreadUIDs, uids...)
	return nil
}

func (f *fakeSession) MarkStarred(uids ...imapv2.UID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markStarredUIDs = append(f.markStarredUIDs, uids...)
	return nil
}

func (f *fakeSession) MarkUnstarred(uids ...imapv2.UID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markUnstarred = append(f.markUnstarred, uids...)
	return nil
}

func (f *fakeSession) Close() error { return nil }

// hasMarkRead 线程安全地检查 MarkRead 是否被调用过（包含指定 uid）。
func (f *fakeSession) hasMarkRead(uid imapv2.UID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.markReadUIDs {
		if u == uid {
			return true
		}
	}
	return false
}

type fakeAccounts struct {
	touched bool
	enabled bool // 默认零值 false，需在 newSyncService 中显式设为 true
}

func (f *fakeAccounts) IMAPConfig(id uint) (types.IMAPConfig, error) {
	return types.IMAPConfig{Host: "h"}, nil
}
func (f *fakeAccounts) TouchLastSync(id uint, t time.Time) error { f.touched = true; return nil }
func (f *fakeAccounts) IsEnabled(id uint) (bool, error)          { return f.enabled, nil }

// newSyncService 构建带临时 SQLite 数据库的测试用 Service，dial 注入共享 fakeSession。
func newSyncService(t *testing.T) (*syncmod.Service, *fakeAccounts, *folder.Service, *fakeSession) {
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
	fsvc := folder.NewService(folder.NewRepository(db))
	msvc := message.NewService(message.NewRepository(db), message.NewBodyRepository(db))
	accts := &fakeAccounts{enabled: true}
	sess := &fakeSession{}
	svc := syncmod.NewService(accts, fsvc, msvc)
	svc.SetDial(func(cfg types.IMAPConfig) (syncmod.Session, error) { return sess, nil })
	return svc, accts, fsvc, sess
}

func TestTriggerRunsToCompletion(t *testing.T) {
	svc, accts, fsvc, _ := newSyncService(t)
	if err := svc.Trigger(1); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := svc.StatusOf(1)
		if st.Phase == syncmod.PhaseDone || st.Phase == syncmod.PhaseError {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	st, ok := svc.StatusOf(1)
	if !ok || st.Phase != syncmod.PhaseDone {
		t.Fatalf("sync not done: %+v", st)
	}
	if !accts.touched {
		t.Errorf("TouchLastSync not called")
	}
	folders, _ := fsvc.List(1)
	if len(folders) != 2 {
		t.Errorf("folders not synced: %d", len(folders))
	}
}

func TestTriggerRejectsConcurrent(t *testing.T) {
	svc, _, _, _ := newSyncService(t)
	block := make(chan struct{})
	svc.SetDial(func(cfg types.IMAPConfig) (syncmod.Session, error) {
		<-block
		return &fakeSession{}, nil
	})
	if err := svc.Trigger(1); err != nil {
		t.Fatalf("first trigger: %v", err)
	}
	err := svc.Trigger(1)
	if err == nil {
		t.Errorf("second concurrent trigger should be rejected")
	}
	close(block)
	// 等后台 goroutine 跑完，避免 Cleanup 关库时它还在访问 DB
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st, ok := svc.StatusOf(1)
		if ok && (st.Phase == syncmod.PhaseDone || st.Phase == syncmod.PhaseError) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestMessageDetailFetchesBodyWhenNotSynced 验证 MessageDetail 在 BodySynced=false 时
// 调用 FetchByUIDs 抓取正文并落库，返回非空 TextBody/HTMLBody。
func TestMessageDetailFetchesBodyWhenNotSynced(t *testing.T) {
	svc, _, fsvc, _ := newSyncService(t)

	// 先同步文件夹，使 INBOX 入库（fakeSession.ListFolders 返回 INBOX）
	sess := &fakeSession{}
	if err := fsvc.SyncFolders(1, sess); err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}

	// 找到 INBOX 的 ID
	inbox, err := fsvc.FindInbox(1)
	if err != nil || inbox == nil {
		t.Fatalf("FindInbox: %v, inbox=%v", err, inbox)
	}

	// 直接 Trigger 让 svc 同步 INBOX 消息（FetchByUIDRange 返回 uid=1,2；BodySynced 默认 false）
	if err := svc.Trigger(1); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := svc.StatusOf(1)
		if st.Phase == syncmod.PhaseDone || st.Phase == syncmod.PhaseError {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if st, _ := svc.StatusOf(1); st.Phase != syncmod.PhaseDone {
		t.Fatalf("sync not done: %+v", st)
	}

	// 此时邮件已入库但 BodySynced=false，调用 MessageDetail 应触发按需抓取
	detail, err := svc.MessageDetail(1)
	if err != nil {
		t.Fatalf("MessageDetail: %v", err)
	}
	if detail.TextBody == "" && detail.HTMLBody == "" {
		t.Errorf("expected non-empty body, got TextBody=%q HTMLBody=%q", detail.TextBody, detail.HTMLBody)
	}
	// 再查一次，BodySynced 应为 true
	detail2, err := svc.MessageDetail(1)
	if err != nil {
		t.Fatalf("MessageDetail second call: %v", err)
	}
	if !detail2.BodySynced {
		t.Errorf("expected BodySynced=true after fetch, got false")
	}
}

// TestTriggerRejectsDisabled 验证停用账户触发同步时返回 ErrAccountDisabled。
func TestTriggerRejectsDisabled(t *testing.T) {
	svc, accts, _, _ := newSyncService(t)
	accts.enabled = false
	err := svc.Trigger(1)
	if err == nil {
		t.Fatal("expected ErrAccountDisabled, got nil")
	}
	if !errors.Is(err, syncmod.ErrAccountDisabled) {
		t.Fatalf("expected ErrAccountDisabled, got %v", err)
	}
}

// TestSetReadLocalThenWriteback 验证 SetRead 立即更新本地，并在 2s 内异步回写 IMAP。
func TestSetReadLocalThenWriteback(t *testing.T) {
	svc, _, fsvc, fakeSess := newSyncService(t)

	// 先同步文件夹
	if err := fsvc.SyncFolders(1, fakeSess); err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	inbox, err := fsvc.FindInbox(1)
	if err != nil || inbox == nil {
		t.Fatalf("FindInbox: %v", err)
	}

	// 同步消息（uid=1,2 入库，Seen=false）
	if err := svc.Trigger(1); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := svc.StatusOf(1)
		if st.Phase == syncmod.PhaseDone || st.Phase == syncmod.PhaseError {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 调用 SetRead，立即断言本地已更新
	if err := svc.SetRead(1, true); err != nil {
		t.Fatalf("SetRead: %v", err)
	}

	// 轮询等待异步回写（最多 2s）
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fakeSess.hasMarkRead(imapv2.UID(1)) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !fakeSess.hasMarkRead(imapv2.UID(1)) {
		t.Errorf("expected MarkRead to be called for uid=1, but it was not within 2s")
	}
}
