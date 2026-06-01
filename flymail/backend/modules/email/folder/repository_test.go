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
