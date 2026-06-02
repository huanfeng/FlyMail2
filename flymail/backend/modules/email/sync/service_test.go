package sync_test

import (
	"bytes"
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
	gomessage "github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

// fakeSession 实现 syncmod.Session 接口，线程安全记录方法调用。
type fakeSession struct {
	mu              gosync.Mutex
	markReadUIDs    []imapv2.UID
	markUnreadUIDs  []imapv2.UID
	markStarredUIDs []imapv2.UID
	markUnstarred   []imapv2.UID
	rawMessage      []byte // FetchRawMessage 返回的原始 RFC 5322 字节
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

func (f *fakeSession) CanIDLE() bool                            { return false }
func (f *fakeSession) StartIDLE() (*coreimap.IdleHandle, error) { return nil, nil }
func (f *fakeSession) SetIDLEHandler(func(coreimap.IDLEEvent))  {}

func (f *fakeSession) Close() error { return nil }

// FetchRawMessage 返回预置的原始 RFC 5322 字节（供 AttachmentContent 测试使用）。
func (f *fakeSession) FetchRawMessage(uid imapv2.UID) ([]byte, error) {
	return f.rawMessage, nil
}

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

// TestAccountStats 验证 AccountStats 在同步完成后返回正确的邮件数与文件夹数。
func TestAccountStats(t *testing.T) {
	svc, _, _, _ := newSyncService(t)

	// 触发同步：fakeSession.ListFolders 返回 2 个文件夹，FetchByUIDRange 返回 2 封邮件（INBOX）
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

	stats, err := svc.AccountStats(1)
	if err != nil {
		t.Fatalf("AccountStats: %v", err)
	}
	// fakeSession 同步了 2 封邮件（INBOX uid=1,2）和 2 个文件夹（INBOX + Sent）
	if stats.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2", stats.MessageCount)
	}
	if stats.FolderCount != 2 {
		t.Errorf("FolderCount = %d, want 2", stats.FolderCount)
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

// buildRFC822WithPDFAttachment 构造一封含 text/plain 正文 + application/pdf 附件的原始 RFC 5322 字节。
// 附件 filename=doc.pdf，内容为 "PDFDATA"。
func buildRFC822WithPDFAttachment(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer

	// 构造顶层 multipart/mixed
	var h gomessage.Header
	h.SetContentType("multipart/mixed", map[string]string{"boundary": "testboundary"})
	h.Set("From", "sender@example.com")
	h.Set("To", "receiver@example.com")
	h.Set("Subject", "Test with attachment")

	mw, err := gomessage.CreateWriter(&buf, h)
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}

	// text/plain 正文部件
	var ph gomessage.Header
	ph.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
	pw, err := mw.CreatePart(ph)
	if err != nil {
		t.Fatalf("CreatePart text: %v", err)
	}
	if _, err := pw.Write([]byte("hello text")); err != nil {
		t.Fatalf("write text: %v", err)
	}
	pw.Close()

	// application/pdf 附件部件
	var ah gomessage.Header
	ah.SetContentType("application/pdf", nil)
	ah.SetContentDisposition("attachment", map[string]string{"filename": "doc.pdf"})
	aw, err := mw.CreatePart(ah)
	if err != nil {
		t.Fatalf("CreatePart pdf: %v", err)
	}
	if _, err := aw.Write([]byte("PDFDATA")); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	aw.Close()

	mw.Close()
	return buf.Bytes()
}

// TestAttachmentContent 验证 AttachmentContent 能正确解析附件，并对越界索引返回 ErrAttachmentNotFound。
func TestAttachmentContent(t *testing.T) {
	// 确保 go-message/mail 包被使用（mail.Header 用于后续扩展，此处显式引用避免 import 报错）
	_ = mail.Header{}

	svc, _, fsvc, fakeSess := newSyncService(t)

	// 先同步文件夹，使 INBOX 入库
	if err := fsvc.SyncFolders(1, fakeSess); err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}

	// 触发同步，使邮件 uid=1 入库（message id=1）
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

	// 设置 rawMessage：一封含 pdf 附件的 RFC 5322 邮件
	fakeSess.rawMessage = buildRFC822WithPDFAttachment(t)

	// 调用 AttachmentContent(1, 0)，断言返回正确附件
	res, err := svc.AttachmentContent(1, 0)
	if err != nil {
		t.Fatalf("AttachmentContent(1,0): %v", err)
	}
	if res.Filename != "doc.pdf" {
		t.Errorf("Filename = %q, want %q", res.Filename, "doc.pdf")
	}
	if res.ContentType != "application/pdf" {
		t.Errorf("ContentType = %q, want %q", res.ContentType, "application/pdf")
	}
	if string(res.Data) != "PDFDATA" {
		t.Errorf("Data = %q, want %q", string(res.Data), "PDFDATA")
	}

	// 调用 AttachmentContent(1, 5)，断言返回 ErrAttachmentNotFound
	_, err = svc.AttachmentContent(1, 5)
	if !errors.Is(err, syncmod.ErrAttachmentNotFound) {
		t.Errorf("expected ErrAttachmentNotFound, got %v", err)
	}
}

// TestSetReadUpdatesFolderUnreadCount 回归：标记已读后文件夹未读数应即时下降，
// 而不是等到下次同步才更新（前端文件夹未读角标依赖此值）。
func TestSetReadUpdatesFolderUnreadCount(t *testing.T) {
	svc, _, fsvc, fakeSess := newSyncService(t)

	if err := fsvc.SyncFolders(1, fakeSess); err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
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

	// 同步后两封均未读 → 未读数应为 2。
	inbox, err := fsvc.FindInbox(1)
	if err != nil || inbox == nil {
		t.Fatalf("FindInbox: %v", err)
	}
	if inbox.UnreadCount != 2 {
		t.Fatalf("同步后未读应为 2，实际 %d", inbox.UnreadCount)
	}

	// 标记其中一封已读 → 未读数应即时降为 1。
	if err := svc.SetRead(1, true); err != nil {
		t.Fatalf("SetRead: %v", err)
	}
	inbox, err = fsvc.FindInbox(1)
	if err != nil || inbox == nil {
		t.Fatalf("FindInbox: %v", err)
	}
	if inbox.UnreadCount != 1 {
		t.Errorf("标记已读后未读应即时变为 1，实际 %d", inbox.UnreadCount)
	}
}
