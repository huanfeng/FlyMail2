package monitoring_test

import (
	"path/filepath"
	"testing"
	"time"

	"flymail/internal/crypto"
	"flymail/internal/database"
	"flymail/internal/sse"
	"flymail/modules/email/account"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	syncmod "flymail/modules/email/sync"
	"flymail/modules/system/monitoring"
)

func newMonitoring(t *testing.T) (*monitoring.Service, *account.Service, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "t.db")
	db, err := database.Open(dbPath)
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
	enc, err := crypto.New("a-test-encryption-key-32bytes!!")
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	accountSvc := account.NewService(account.NewRepository(db), enc)
	folderSvc := folder.NewService(folder.NewRepository(db))
	messageSvc := message.NewService(message.NewRepository(db), message.NewBodyRepository(db))
	syncSvc := syncmod.NewService(accountSvc, folderSvc, messageSvc)
	manager := syncmod.NewManager(accountSvc, folderSvc, messageSvc, sse.NewHub())
	svc := monitoring.NewService(accountSvc, folderSvc, syncSvc, manager, time.Now(), "test-ver", dbPath)
	return svc, accountSvc, dbPath
}

func TestOverview(t *testing.T) {
	svc, accountSvc, _ := newMonitoring(t)
	if _, err := accountSvc.Create(account.CreateAccountRequest{
		Name: "A", Email: "a@x.com", Password: "pw",
		IMAPHost: "imap.x.com", IMAPPort: 993, SMTPHost: "smtp.x.com", SMTPPort: 465,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ov, err := svc.Overview()
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if ov.Accounts != 1 {
		t.Errorf("accounts = %d, want 1", ov.Accounts)
	}
	if ov.Version != "test-ver" {
		t.Errorf("version = %q", ov.Version)
	}
	if ov.PollIntervalSec <= 0 {
		t.Errorf("poll interval 应为正数，得 %d", ov.PollIntervalSec)
	}
	if ov.ActiveWorkers != 0 {
		t.Errorf("未启动 Manager，active workers 应为 0，得 %d", ov.ActiveWorkers)
	}
	if ov.DBSizeBytes <= 0 {
		t.Errorf("db 文件应有大小，得 %d", ov.DBSizeBytes)
	}
}

func TestAccountsHealth(t *testing.T) {
	svc, accountSvc, _ := newMonitoring(t)
	resp, err := accountSvc.Create(account.CreateAccountRequest{
		Name: "Acc", Email: "acc@x.com", Password: "pw",
		IMAPHost: "imap.x.com", IMAPPort: 993, SMTPHost: "smtp.x.com", SMTPPort: 465,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := svc.Accounts()
	if err != nil {
		t.Fatalf("Accounts: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("应有 1 个账户健康项，得 %d", len(list))
	}
	h := list[0]
	if h.ID != resp.ID || h.Email != "acc@x.com" || !h.Enabled {
		t.Errorf("账户健康字段错误: %+v", h)
	}
	if h.HasWorker {
		t.Errorf("未启动 Manager，不应有 worker")
	}
	if h.SyncPhase != "none" {
		t.Errorf("无手动同步时 phase 应为 none，得 %q", h.SyncPhase)
	}
}
