package sync_test

import (
	"path/filepath"
	"testing"
	"time"

	"flymail-core/types"

	"flymail/internal/database"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	syncmod "flymail/modules/email/sync"
)

// newMailops 自建一套服务 + repo（同一 db），并注入记录调用的 fakeSession。
func newMailops(t *testing.T) (*syncmod.Service, *folder.Repository, *message.Repository, *fakeSession) {
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
	sess := &fakeSession{}
	svc := syncmod.NewService(&fakeAccounts{enabled: true}, fsvc, msvc)
	svc.SetDial(func(types.IMAPConfig) (syncmod.Session, error) { return sess, nil })
	return svc, frepo, mrepo, sess
}

// seedFolder 建一个文件夹并返回其 ID。
func seedFolder(t *testing.T, frepo *folder.Repository, accountID uint, path, ftype string) uint {
	t.Helper()
	f := &folder.Folder{AccountID: accountID, Path: path, DisplayName: path, Type: ftype, Selectable: true}
	if err := frepo.UpsertByPath(f); err != nil {
		t.Fatalf("seed folder %s: %v", path, err)
	}
	return f.ID
}

func seedMsg(t *testing.T, mrepo *message.Repository, accountID, folderID uint, uid uint32) uint {
	t.Helper()
	m := &message.Message{AccountID: accountID, FolderID: folderID, UID: uid, Date: time.Now()}
	if err := mrepo.Upsert(m); err != nil {
		t.Fatalf("seed msg: %v", err)
	}
	return m.ID
}

func TestDeleteMessageMovesToTrash(t *testing.T) {
	svc, frepo, mrepo, sess := newMailops(t)
	inboxID := seedFolder(t, frepo, 1, "INBOX", "inbox")
	seedFolder(t, frepo, 1, "Trash", "trash")
	msgID := seedMsg(t, mrepo, 1, inboxID, 100)

	if err := svc.DeleteMessage(msgID); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	// 非回收站的邮件应被移动到回收站，而非永久删除。
	if sess.movedTo != "Trash" {
		t.Errorf("应移动到 Trash，实际 movedTo=%q", sess.movedTo)
	}
	if len(sess.deletedUIDs) != 0 {
		t.Errorf("不应永久删除，deletedUIDs=%v", sess.deletedUIDs)
	}
	// 本地源行应被清除。
	if _, err := mrepo.GetByID(msgID); err == nil {
		t.Errorf("本地行应已删除")
	}
}

func TestDeleteMessageInTrashIsPermanent(t *testing.T) {
	svc, frepo, mrepo, sess := newMailops(t)
	trashID := seedFolder(t, frepo, 1, "Trash", "trash")
	msgID := seedMsg(t, mrepo, 1, trashID, 200)

	if err := svc.DeleteMessage(msgID); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	// 回收站内的邮件应被永久删除（EXPUNGE），不再移动。
	if len(sess.deletedUIDs) != 1 || sess.deletedUIDs[0] != 200 {
		t.Errorf("应永久删除 uid=200，实际 deletedUIDs=%v", sess.deletedUIDs)
	}
	if sess.movedTo != "" {
		t.Errorf("不应移动，movedTo=%q", sess.movedTo)
	}
	if _, err := mrepo.GetByID(msgID); err == nil {
		t.Errorf("本地行应已删除")
	}
}

func TestMoveMessage(t *testing.T) {
	svc, frepo, mrepo, sess := newMailops(t)
	inboxID := seedFolder(t, frepo, 1, "INBOX", "inbox")
	archiveID := seedFolder(t, frepo, 1, "Archive", "archive")
	msgID := seedMsg(t, mrepo, 1, inboxID, 300)

	if err := svc.MoveMessage(msgID, archiveID); err != nil {
		t.Fatalf("MoveMessage: %v", err)
	}
	if sess.movedTo != "Archive" {
		t.Errorf("应移动到 Archive，实际 movedTo=%q", sess.movedTo)
	}
	if _, err := mrepo.GetByID(msgID); err == nil {
		t.Errorf("本地源行应已删除")
	}
}

func TestMoveMessageCrossAccountRejected(t *testing.T) {
	svc, frepo, mrepo, _ := newMailops(t)
	inboxID := seedFolder(t, frepo, 1, "INBOX", "inbox")
	// 账户 2 的文件夹
	otherID := seedFolder(t, frepo, 2, "INBOX", "inbox")
	msgID := seedMsg(t, mrepo, 1, inboxID, 400)

	if err := svc.MoveMessage(msgID, otherID); err == nil {
		t.Errorf("跨账户移动应被拒绝")
	}
	// 失败后本地行应仍在。
	if _, err := mrepo.GetByID(msgID); err != nil {
		t.Errorf("移动失败后本地行应保留: %v", err)
	}
}
