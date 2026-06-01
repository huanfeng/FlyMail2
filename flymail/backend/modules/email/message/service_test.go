package message_test

import (
	"path/filepath"
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
	return message.NewService(repo), repo
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
