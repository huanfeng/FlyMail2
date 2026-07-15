package smoke

import (
	"os"
	"strconv"
	"testing"
	"time"

	"flymail/internal/crypto"
	"flymail/internal/database"
	"flymail/modules/email/account"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	syncmod "flymail/modules/email/sync"
)

// TestM3SmokeRealAccount 仅在设置了 FLYMAIL_SMOKE_* 环境变量时运行真实 IMAP 冒烟。
func TestM3SmokeRealAccount(t *testing.T) {
	email := os.Getenv("FLYMAIL_SMOKE_EMAIL")
	if email == "" {
		t.Skip("set FLYMAIL_SMOKE_EMAIL etc. to run real IMAP smoke")
	}
	db, err := database.Open(t.TempDir() + "/smoke.db")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	enc, _ := crypto.New("smoke-key-smoke-key-smoke-key-32")
	acctSvc := account.NewService(account.NewRepository(db), enc)
	host := os.Getenv("FLYMAIL_SMOKE_IMAP_HOST")
	created, err := acctSvc.Create(account.CreateAccountRequest{
		Name: "smoke", Email: email, Password: os.Getenv("FLYMAIL_SMOKE_PW"),
		IMAPHost: host, IMAPPort: atoiEnv("FLYMAIL_SMOKE_IMAP_PORT", 993), IMAPSecurity: "ssl",
		SMTPHost: host, SMTPPort: 465, SMTPSecurity: "ssl",
	})
	if err != nil {
		t.Fatal(err)
	}
	fsvc := folder.NewService(folder.NewRepository(db))
	msvc := message.NewService(message.NewRepository(db), message.NewBodyRepository(db))
	syncSvc := syncmod.NewService(acctSvc, fsvc, msvc)
	if err := syncSvc.Trigger(created.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := syncSvc.StatusOf(created.ID)
		if st.Phase == syncmod.PhaseDone {
			break
		}
		if st.Phase == syncmod.PhaseError {
			t.Fatalf("sync error: %s", st.Error)
		}
		time.Sleep(500 * time.Millisecond)
	}
	folders, _ := fsvc.List(created.ID)
	t.Logf("synced %d folders", len(folders))
	if len(folders) == 0 {
		t.Fatal("no folders synced")
	}
	inbox, _ := fsvc.FindInbox(created.ID)
	if inbox == nil {
		t.Fatal("inbox not found")
	}
	items, _ := msvc.List(inbox.ID, 0, 20)
	t.Logf("inbox first page: %d messages", len(items))
	for _, m := range items {
		t.Logf("  uid=%d seen=%v from=%q subject=%q", m.UID, m.Seen, m.FromName, m.Subject)
	}
}

func atoiEnv(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return def
}
