package message_test

import (
	"path/filepath"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/modules/email/message"
)

func newRepo(t *testing.T) *message.Repository {
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
	return message.NewRepository(db)
}

func TestUpsertIsIdempotentByFolderUID(t *testing.T) {
	repo := newRepo(t)
	m := &message.Message{AccountID: 1, FolderID: 1, UID: 10, Subject: "A", Seen: false, Date: time.Now()}
	if err := repo.Upsert(m); err != nil {
		t.Fatalf("upsert1: %v", err)
	}
	m2 := &message.Message{AccountID: 1, FolderID: 1, UID: 10, Subject: "A-updated", Seen: true, Date: time.Now()}
	if err := repo.Upsert(m2); err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	n, _ := repo.CountByFolder(1)
	if n != 1 {
		t.Fatalf("want 1 row, got %d", n)
	}
	list, _ := repo.ListByFolder(1, 0, 50)
	if list[0].Subject != "A-updated" || !list[0].Seen {
		t.Errorf("upsert did not update: %+v", list[0])
	}
}

func TestListByFolderUIDCursorDesc(t *testing.T) {
	repo := newRepo(t)
	for _, uid := range []uint32{1, 2, 3, 4, 5} {
		_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: uid, Date: time.Now()})
	}
	page1, _ := repo.ListByFolder(1, 0, 2)
	if len(page1) != 2 || page1[0].UID != 5 || page1[1].UID != 4 {
		t.Fatalf("page1 wrong: %+v", page1)
	}
	page2, _ := repo.ListByFolder(1, 4, 2)
	if len(page2) != 2 || page2[0].UID != 3 || page2[1].UID != 2 {
		t.Fatalf("page2 wrong: %+v", page2)
	}
}

func TestDeleteByFolder(t *testing.T) {
	repo := newRepo(t)
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 1, Date: time.Now()})
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 2, UID: 1, Date: time.Now()})
	if err := repo.DeleteByFolder(1); err != nil {
		t.Fatal(err)
	}
	n1, _ := repo.CountByFolder(1)
	n2, _ := repo.CountByFolder(2)
	if n1 != 0 || n2 != 1 {
		t.Errorf("delete scope wrong: f1=%d f2=%d", n1, n2)
	}
}

func TestUnreadCountByFolder(t *testing.T) {
	repo := newRepo(t)
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 1, Seen: false, Date: time.Now()})
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 2, Seen: true, Date: time.Now()})
	unread, _ := repo.UnreadCountByFolder(1)
	if unread != 1 {
		t.Errorf("unread = %d, want 1", unread)
	}
}
