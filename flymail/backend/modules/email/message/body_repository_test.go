package message_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/modules/email/message"
)

func newBodyRepo(t *testing.T) (*message.Repository, *message.BodyRepository) {
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
	return message.NewRepository(db), message.NewBodyRepository(db)
}

func TestBodyUpsertIdempotent(t *testing.T) {
	repo, bodyRepo := newBodyRepo(t)

	// 先插入一条 Message 作为外键
	m := &message.Message{AccountID: 1, FolderID: 1, UID: 1, Date: time.Now()}
	if err := repo.Upsert(m); err != nil {
		t.Fatalf("upsert message: %v", err)
	}
	got, _ := repo.ListByFolder(1, 0, 10)
	msgID := got[0].ID

	b1 := &message.MessageBody{MessageID: msgID, HTMLBody: "v1"}
	if err := bodyRepo.Upsert(b1); err != nil {
		t.Fatalf("upsert body1: %v", err)
	}

	b2 := &message.MessageBody{MessageID: msgID, HTMLBody: "v2"}
	if err := bodyRepo.Upsert(b2); err != nil {
		t.Fatalf("upsert body2: %v", err)
	}

	result, err := bodyRepo.GetByMessageID(msgID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if result == nil {
		t.Fatal("expected body, got nil")
	}
	if result.HTMLBody != "v2" {
		t.Errorf("want HTMLBody=v2, got %q", result.HTMLBody)
	}

	// 再次 Get 确认幂等
	result2, _ := bodyRepo.GetByMessageID(msgID)
	if result2 == nil || result2.HTMLBody != "v2" {
		t.Errorf("second get wrong: %+v", result2)
	}
}

func TestReplaceAttachments(t *testing.T) {
	repo, bodyRepo := newBodyRepo(t)

	m := &message.Message{AccountID: 1, FolderID: 1, UID: 2, Date: time.Now()}
	if err := repo.Upsert(m); err != nil {
		t.Fatalf("upsert message: %v", err)
	}
	got, _ := repo.ListByFolder(1, 0, 10)
	msgID := got[0].ID

	// Replace 2 个附件
	atts2 := []message.Attachment{
		{MessageID: msgID, Filename: "a.pdf", ContentType: "application/pdf", Size: 100},
		{MessageID: msgID, Filename: "b.png", ContentType: "image/png", Size: 200},
	}
	if err := bodyRepo.ReplaceAttachments(msgID, atts2); err != nil {
		t.Fatalf("replace 2: %v", err)
	}
	list, _ := bodyRepo.ListAttachments(msgID)
	if len(list) != 2 {
		t.Fatalf("want 2 attachments, got %d", len(list))
	}

	// Replace 1 个附件
	atts1 := []message.Attachment{
		{MessageID: msgID, Filename: "c.zip", ContentType: "application/zip", Size: 300},
	}
	if err := bodyRepo.ReplaceAttachments(msgID, atts1); err != nil {
		t.Fatalf("replace 1: %v", err)
	}
	list, _ = bodyRepo.ListAttachments(msgID)
	if len(list) != 1 {
		t.Fatalf("want 1 attachment, got %d", len(list))
	}
	if list[0].Filename != "c.zip" {
		t.Errorf("want c.zip, got %q", list[0].Filename)
	}

	// Replace 空切片得 0 个
	if err := bodyRepo.ReplaceAttachments(msgID, nil); err != nil {
		t.Fatalf("replace empty: %v", err)
	}
	list, _ = bodyRepo.ListAttachments(msgID)
	if len(list) != 0 {
		t.Fatalf("want 0 attachments, got %d", len(list))
	}
}

func TestGetByIDAndMarks(t *testing.T) {
	repo, _ := newBodyRepo(t)

	m := &message.Message{AccountID: 2, FolderID: 3, UID: 99, Date: time.Now()}
	if err := repo.Upsert(m); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	list, _ := repo.ListByFolder(3, 0, 10)
	id := list[0].ID

	// GetByID
	got, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != id {
		t.Errorf("id mismatch: %d != %d", got.ID, id)
	}

	// SetSeen
	if err := repo.SetSeen(id, true); err != nil {
		t.Fatalf("SetSeen: %v", err)
	}
	got, _ = repo.GetByID(id)
	if !got.Seen {
		t.Error("want Seen=true")
	}

	// SetFlagged
	if err := repo.SetFlagged(id, true); err != nil {
		t.Fatalf("SetFlagged: %v", err)
	}
	got, _ = repo.GetByID(id)
	if !got.Flagged {
		t.Error("want Flagged=true")
	}

	// MarkBodySynced
	if err := repo.MarkBodySynced(id, "摘要", true); err != nil {
		t.Fatalf("MarkBodySynced: %v", err)
	}
	got, _ = repo.GetByID(id)
	if !got.BodySynced {
		t.Error("want BodySynced=true")
	}
	if got.Snippet != "摘要" {
		t.Errorf("want Snippet=摘要, got %q", got.Snippet)
	}
	if !got.HasAttachment {
		t.Error("want HasAttachment=true")
	}
}

func TestGetByIDNotFound(t *testing.T) {
	repo, _ := newBodyRepo(t)

	_, err := repo.GetByID(9999)
	if !errors.Is(err, message.ErrMessageNotFound) {
		t.Errorf("want ErrMessageNotFound, got %v", err)
	}
}
