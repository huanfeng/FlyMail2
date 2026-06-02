package folder_test

import (
	"path/filepath"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/modules/email/folder"

	"gorm.io/gorm"
)

func newRepo(t *testing.T) (*folder.Repository, *gorm.DB) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if sqlDB, e := db.DB(); e == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return folder.NewRepository(db), db
}

func TestUpsertByPathInsertThenUpdate(t *testing.T) {
	repo, _ := newRepo(t)
	f := &folder.Folder{AccountID: 1, Path: "INBOX", DisplayName: "收件箱", Type: "inbox", SortOrder: 1}
	if err := repo.UpsertByPath(f); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if f.ID == 0 {
		t.Fatal("ID should be set after insert")
	}
	f2 := &folder.Folder{AccountID: 1, Path: "INBOX", DisplayName: "Inbox", Type: "inbox", SortOrder: 1}
	if err := repo.UpsertByPath(f2); err != nil {
		t.Fatalf("update: %v", err)
	}
	list, _ := repo.ListByAccount(1)
	if len(list) != 1 {
		t.Fatalf("want 1 folder, got %d", len(list))
	}
	if list[0].DisplayName != "Inbox" {
		t.Errorf("display name not updated: %q", list[0].DisplayName)
	}
}

func TestUpsertPreservesSyncAnchors(t *testing.T) {
	repo, _ := newRepo(t)
	now := time.Now()
	f := &folder.Folder{AccountID: 1, Path: "INBOX", DisplayName: "X", Type: "inbox", UIDValidity: 42, UIDNext: 100, LastSyncedAt: &now}
	if err := repo.UpsertByPath(f); err != nil {
		t.Fatal(err)
	}
	f2 := &folder.Folder{AccountID: 1, Path: "INBOX", DisplayName: "X", Type: "inbox"}
	if err := repo.UpsertByPath(f2); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetByPath(1, "INBOX")
	if got.UIDValidity != 42 || got.UIDNext != 100 {
		t.Errorf("sync anchors not preserved: validity=%d next=%d", got.UIDValidity, got.UIDNext)
	}
}

func TestCountByAccount(t *testing.T) {
	repo, _ := newRepo(t)
	// account 1：两个文件夹
	_ = repo.UpsertByPath(&folder.Folder{AccountID: 1, Path: "INBOX", DisplayName: "Inbox", Type: "inbox"})
	_ = repo.UpsertByPath(&folder.Folder{AccountID: 1, Path: "Sent", DisplayName: "Sent", Type: "sent"})
	// account 2：一个文件夹，不应计入 account 1
	_ = repo.UpsertByPath(&folder.Folder{AccountID: 2, Path: "INBOX", DisplayName: "Inbox", Type: "inbox"})

	n1, err := repo.CountByAccount(1)
	if err != nil {
		t.Fatalf("CountByAccount(1): %v", err)
	}
	if n1 != 2 {
		t.Errorf("account 1: want 2, got %d", n1)
	}

	n2, err := repo.CountByAccount(2)
	if err != nil {
		t.Fatalf("CountByAccount(2): %v", err)
	}
	if n2 != 1 {
		t.Errorf("account 2: want 1, got %d", n2)
	}
}

func TestFindInboxByType(t *testing.T) {
	repo, _ := newRepo(t)
	_ = repo.UpsertByPath(&folder.Folder{AccountID: 1, Path: "Sent", DisplayName: "Sent", Type: "sent"})
	_ = repo.UpsertByPath(&folder.Folder{AccountID: 1, Path: "INBOX", DisplayName: "Inbox", Type: "inbox"})
	inbox, err := repo.FindInbox(1)
	if err != nil {
		t.Fatalf("find inbox: %v", err)
	}
	if inbox == nil || inbox.Path != "INBOX" {
		t.Errorf("inbox not found correctly: %+v", inbox)
	}
}

func TestFindByType(t *testing.T) {
	repo, _ := newRepo(t)
	_ = repo.UpsertByPath(&folder.Folder{AccountID: 1, Path: "Sent", DisplayName: "Sent", Type: "sent"})
	_ = repo.UpsertByPath(&folder.Folder{AccountID: 1, Path: "INBOX", DisplayName: "Inbox", Type: "inbox"})

	// 找已发送
	sent, err := repo.FindByType(1, "sent")
	if err != nil {
		t.Fatalf("FindByType sent: %v", err)
	}
	if sent == nil || sent.Path != "Sent" {
		t.Errorf("sent folder not found: %+v", sent)
	}

	// 不存在的 type 返回 nil, nil
	drafts, err := repo.FindByType(1, "drafts")
	if err != nil {
		t.Fatalf("FindByType drafts: %v", err)
	}
	if drafts != nil {
		t.Errorf("expected nil for missing type, got %+v", drafts)
	}
}
