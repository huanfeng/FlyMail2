package message_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/modules/email/message"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	imapv2 "github.com/emersion/go-imap/v2"
)

type fakeFetcher struct {
	uidValidity        uint32
	uidNext            uint32
	numMessages        uint32
	emails             map[uint32]*types.ParsedEmail
	statusValidity     uint32
	selectValidityZero bool
}

func (f *fakeFetcher) SelectFolder(path string) (*coreimap.SelectedFolder, error) {
	v := f.uidValidity
	if f.selectValidityZero {
		v = 0
	}
	return &coreimap.SelectedFolder{Path: path, NumMessages: f.numMessages, UIDValidity: v, UIDNext: f.uidNext}, nil
}

func (f *fakeFetcher) FolderStatus(path string, items ...coreimap.StatusItem) (*coreimap.FolderStatusResult, error) {
	v := f.statusValidity
	return &coreimap.FolderStatusResult{UIDValidity: &v}, nil
}

func (f *fakeFetcher) FetchByUIDRange(from, to imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	var out []*types.ParsedEmail
	for uid, e := range f.emails {
		if imapv2.UID(uid) >= from && (to == 0 || imapv2.UID(uid) <= to) {
			out = append(out, e)
		}
	}
	return out, nil
}

// FetchBySeqRange：fake 中把序号当作 uid 处理（emails 以 uid 1..N 连续填充时等价）。
func (f *fakeFetcher) FetchBySeqRange(from, to uint32, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	var out []*types.ParsedEmail
	for uid, e := range f.emails {
		if uid >= from && (to == 0 || uid <= to) {
			out = append(out, e)
		}
	}
	return out, nil
}

func newMsgService(t *testing.T) (*message.Service, *message.Repository) {
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
	repo := message.NewRepository(db)
	bodyRepo := message.NewBodyRepository(db)
	return message.NewService(repo, bodyRepo), repo
}

func TestSyncFolderMessagesStoresMetadata(t *testing.T) {
	svc, repo := newMsgService(t)
	emails := map[uint32]*types.ParsedEmail{}
	for uid := uint32(1); uid <= 5; uid++ {
		emails[uid] = &types.ParsedEmail{
			UID: uid, Subject: "Mail", MessageID: "mid", Date: time.Now(),
			From:   []types.Address{{Name: "张三", Email: "z@e.com"}},
			To:     []types.Address{{Name: "Me", Email: "me@e.com"}},
			IsRead: uid%2 == 0, Size: 100,
		}
	}
	f := &fakeFetcher{uidValidity: 42, uidNext: 6, numMessages: 5, emails: emails}
	state, rebuilt, err := svc.SyncFolderMessages(1, 1, "INBOX", 0, f)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if rebuilt {
		t.Errorf("first sync should not rebuild")
	}
	if state.UIDValidity != 42 || state.Total != 5 {
		t.Errorf("state wrong: %+v", state)
	}
	list, _ := repo.ListByFolder(1, 0, 50)
	if len(list) != 5 {
		t.Fatalf("want 5 stored, got %d", len(list))
	}
	if list[0].FromName != "张三" || list[0].FromAddr != "z@e.com" {
		t.Errorf("from not split: %+v", list[0])
	}
}

func TestSyncRebuildsOnUIDValidityChange(t *testing.T) {
	svc, repo := newMsgService(t)
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 999, Date: time.Now()})
	f := &fakeFetcher{uidValidity: 100, uidNext: 2, numMessages: 1, emails: map[uint32]*types.ParsedEmail{
		1: {UID: 1, Subject: "new", Date: time.Now()},
	}}
	_, rebuilt, err := svc.SyncFolderMessages(1, 1, "INBOX", 42, f)
	if err != nil {
		t.Fatal(err)
	}
	if !rebuilt {
		t.Errorf("should rebuild on uidvalidity change")
	}
	list, _ := repo.ListByFolder(1, 0, 50)
	if len(list) != 1 || list[0].UID != 1 {
		t.Errorf("old uid 999 should be gone: %+v", list)
	}
}

func TestSyncFolderMessagesMultiBatch(t *testing.T) {
	svc, repo := newMsgService(t)
	emails := map[uint32]*types.ParsedEmail{}
	for uid := uint32(1); uid <= 450; uid++ {
		emails[uid] = &types.ParsedEmail{UID: uid, Subject: "Mail", Date: time.Now()}
	}
	// uidNext=451, numMessages=450 -> from=1, end=450 -> 3 批（200+200+50）
	f := &fakeFetcher{uidValidity: 1, uidNext: 451, numMessages: 450, emails: emails}
	state, _, err := svc.SyncFolderMessages(1, 1, "INBOX", 0, f)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if state.Total != 450 {
		t.Errorf("state.Total = %d, want 450", state.Total)
	}
	page, _ := repo.ListByFolder(1, 0, 200)
	if len(page) != 200 {
		t.Errorf("first page = %d, want 200", len(page))
	}
}

func TestSyncUIDValidityFallbackToStatus(t *testing.T) {
	svc, _ := newMsgService(t)
	f := &fakeFetcher{uidNext: 2, numMessages: 1, selectValidityZero: true, statusValidity: 77,
		emails: map[uint32]*types.ParsedEmail{1: {UID: 1, Date: time.Now()}}}
	state, _, err := svc.SyncFolderMessages(1, 1, "INBOX", 0, f)
	if err != nil {
		t.Fatal(err)
	}
	if state.UIDValidity != 77 {
		t.Errorf("should fall back to STATUS uidvalidity, got %d", state.UIDValidity)
	}
}

// TestSyncViaSeqWhenNoUIDNext 模拟 163：SELECT/STATUS 都不报 UIDNEXT（uidNext=0、STATUS 也无），
// 但 NumMessages>0；应改用按序号抓取，把邮件落库，并用 maxUID+1 作为 UIDNext 锚点。
func TestSyncViaSeqWhenNoUIDNext(t *testing.T) {
	svc, repo := newMsgService(t)
	emails := map[uint32]*types.ParsedEmail{}
	for uid := uint32(1); uid <= 5; uid++ {
		emails[uid] = &types.ParsedEmail{UID: uid, Subject: "m", Date: time.Now()}
	}
	// uidNext=0（SELECT 不报）；fakeFetcher.FolderStatus 也不返回 UIDNext（仅 UIDValidity）。
	f := &fakeFetcher{uidValidity: 1, uidNext: 0, numMessages: 5, emails: emails}
	state, _, err := svc.SyncFolderMessages(1, 1, "INBOX", 0, f)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	list, _ := repo.ListByFolder(1, 0, 50)
	if len(list) != 5 {
		t.Fatalf("want 5 stored via seq fetch, got %d", len(list))
	}
	if state.Total != 5 {
		t.Errorf("state.Total = %d, want 5", state.Total)
	}
	if state.UIDNext != 6 { // maxUID(5)+1
		t.Errorf("state.UIDNext = %d, want 6 (maxUID+1 锚点)", state.UIDNext)
	}
}

func TestStoreParsedBodyAndDetail(t *testing.T) {
	svc, repo := newMsgService(t)

	// 先插入一封邮件元数据
	m := &message.Message{
		AccountID: 1, FolderID: 1, UID: 1,
		Subject: "测试邮件", Date: time.Now(),
	}
	if err := repo.Upsert(m); err != nil {
		t.Fatalf("upsert message: %v", err)
	}
	// 取回以获得自增 ID
	list, err := repo.ListByFolder(1, 0, 10)
	if err != nil || len(list) == 0 {
		t.Fatalf("list: %v, len=%d", err, len(list))
	}
	msgID := list[0].ID

	// 构造 ParsedEmail 含正文与附件
	longText := strings.Repeat("这是一段很长的正文内容。", 20) // >150 字
	e := &types.ParsedEmail{
		TextBody: longText,
		HTMLBody: "<p>hi</p>",
		Attachments: []types.Attachment{
			{Filename: "a.pdf", ContentType: "application/pdf", Size: 100},
		},
	}

	// 存正文
	if err := svc.StoreParsedBody(msgID, e); err != nil {
		t.Fatalf("StoreParsedBody: %v", err)
	}

	// 验证元数据已回填
	stored, err := repo.GetByID(msgID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !stored.HasAttachment {
		t.Errorf("HasAttachment should be true")
	}
	if !stored.BodySynced {
		t.Errorf("BodySynced should be true")
	}
	if stored.Snippet == "" {
		t.Errorf("Snippet should not be empty")
	}

	// 验证 Detail 组装
	detail, err := svc.Detail(msgID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if detail.TextBody != longText {
		t.Errorf("TextBody mismatch: got %q", detail.TextBody[:20])
	}
	if detail.HTMLBody != "<p>hi</p>" {
		t.Errorf("HTMLBody mismatch: got %q", detail.HTMLBody)
	}
	if len(detail.Attachments) != 1 {
		t.Errorf("want 1 attachment, got %d", len(detail.Attachments))
	} else if detail.Attachments[0].Filename != "a.pdf" {
		t.Errorf("attachment filename mismatch: %q", detail.Attachments[0].Filename)
	}
	if !detail.BodySynced {
		t.Errorf("detail.BodySynced should be true")
	}
	if detail.Snippet == "" {
		t.Errorf("detail.Snippet should not be empty")
	}
}
