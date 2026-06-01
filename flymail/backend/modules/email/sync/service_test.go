package sync_test

import (
	"path/filepath"
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

type fakeSession struct{}

func (fakeSession) ListFolders() ([]types.FolderInfo, error) {
	return []types.FolderInfo{
		{Name: "Inbox", Path: "INBOX", Attributes: []string{"\\Inbox"}},
		{Name: "Sent", Path: "Sent", Attributes: []string{"\\Sent"}},
	}, nil
}
func (fakeSession) SelectFolder(path string) (*coreimap.SelectedFolder, error) {
	return &coreimap.SelectedFolder{Path: path, NumMessages: 2, UIDValidity: 1, UIDNext: 3}, nil
}
func (fakeSession) FolderStatus(path string, items ...coreimap.StatusItem) (*coreimap.FolderStatusResult, error) {
	v := uint32(1)
	return &coreimap.FolderStatusResult{UIDValidity: &v}, nil
}
func (fakeSession) FetchByUIDRange(from, to imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	return []*types.ParsedEmail{
		{UID: 1, Subject: "a", Date: time.Now()},
		{UID: 2, Subject: "b", Date: time.Now()},
	}, nil
}
func (fakeSession) Close() error { return nil }

type fakeAccounts struct{ touched bool }

func (f *fakeAccounts) IMAPConfig(id uint) (types.IMAPConfig, error) {
	return types.IMAPConfig{Host: "h"}, nil
}
func (f *fakeAccounts) TouchLastSync(id uint, t time.Time) error { f.touched = true; return nil }

func newSyncService(t *testing.T) (*syncmod.Service, *fakeAccounts, *folder.Service) {
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
	accts := &fakeAccounts{}
	svc := syncmod.NewService(accts, fsvc, msvc)
	svc.SetDial(func(cfg types.IMAPConfig) (syncmod.Session, error) { return fakeSession{}, nil })
	return svc, accts, fsvc
}

func TestTriggerRunsToCompletion(t *testing.T) {
	svc, accts, fsvc := newSyncService(t)
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
	svc, _, _ := newSyncService(t)
	block := make(chan struct{})
	svc.SetDial(func(cfg types.IMAPConfig) (syncmod.Session, error) {
		<-block
		return fakeSession{}, nil
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
