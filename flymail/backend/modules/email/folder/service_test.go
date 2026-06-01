package folder_test

import (
	"path/filepath"
	"testing"

	"flymail/internal/database"
	"flymail/modules/email/folder"

	"flymail-core/types"
)

type fakeLister struct{ infos []types.FolderInfo }

func (f fakeLister) ListFolders() ([]types.FolderInfo, error) { return f.infos, nil }

func newService(t *testing.T) *folder.Service {
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
	return folder.NewService(folder.NewRepository(db))
}

func TestSyncFoldersClassifiesAndStores(t *testing.T) {
	svc := newService(t)
	lister := fakeLister{infos: []types.FolderInfo{
		{Name: "收件箱", Path: "INBOX", Delimiter: "/", Attributes: []string{"\\Inbox"}},
		{Name: "已发送", Path: "Sent", Delimiter: "/", Attributes: []string{"\\Sent"}},
		{Name: "Containers", Path: "Containers", Attributes: []string{"\\Noselect"}},
	}}
	if err := svc.SyncFolders(1, lister); err != nil {
		t.Fatalf("sync: %v", err)
	}
	list, _ := svc.List(1)
	if len(list) != 3 {
		t.Fatalf("want 3 folders, got %d", len(list))
	}
	byPath := map[string]folder.Folder{}
	for _, f := range list {
		byPath[f.Path] = f
	}
	if byPath["INBOX"].Type != "inbox" {
		t.Errorf("INBOX type = %q, want inbox", byPath["INBOX"].Type)
	}
	if byPath["INBOX"].SortOrder != 1 {
		t.Errorf("INBOX sort = %d, want 1", byPath["INBOX"].SortOrder)
	}
	if byPath["Containers"].Selectable {
		t.Errorf("\\Noselect folder should not be selectable")
	}
	if byPath["INBOX"].DisplayName != "收件箱" {
		t.Errorf("display name = %q, want 收件箱", byPath["INBOX"].DisplayName)
	}
}
