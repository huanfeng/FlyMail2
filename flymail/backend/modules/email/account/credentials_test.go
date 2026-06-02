package account_test

import (
	"testing"
	"time"

	"flymail/modules/email/account"
)

func TestIMAPConfigDecryptsPassword(t *testing.T) {
	svc, _, _ := newSvc(t)

	created, err := svc.Create(account.CreateAccountRequest{
		Name:         "Test User",
		Email:        "test@example.com",
		Password:     "secret-pw",
		IMAPHost:     "imap.example.com",
		IMAPPort:     993,
		IMAPSecurity: "ssl",
		SMTPHost:     "smtp.example.com",
		SMTPPort:     465,
		SMTPSecurity: "ssl",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cfg, err := svc.IMAPConfig(created.ID)
	if err != nil {
		t.Fatalf("IMAPConfig: %v", err)
	}

	if cfg.Password != "secret-pw" {
		t.Errorf("Password = %q, want %q", cfg.Password, "secret-pw")
	}
	// Username 为空时应回退到 Email
	if cfg.Username != "test@example.com" {
		t.Errorf("Username = %q, want %q", cfg.Username, "test@example.com")
	}
	// IMAPSecurity "ssl" → types.SecuritySSL
	const wantSecurity = "ssl"
	if string(cfg.Security) != wantSecurity {
		t.Errorf("Security = %q, want %q", cfg.Security, wantSecurity)
	}
}

func TestSMTPConfigDecrypts(t *testing.T) {
	svc, _, _ := newSvc(t)

	created, err := svc.Create(account.CreateAccountRequest{
		Name:         "SMTP User",
		Email:        "smtp@example.com",
		Password:     "smtp-secret",
		IMAPHost:     "imap.example.com",
		IMAPPort:     993,
		IMAPSecurity: "ssl",
		SMTPHost:     "smtp.example.com",
		SMTPPort:     465,
		SMTPSecurity: "ssl",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cfg, err := svc.SMTPConfig(created.ID)
	if err != nil {
		t.Fatalf("SMTPConfig: %v", err)
	}

	if cfg.Password != "smtp-secret" {
		t.Errorf("Password = %q, want %q", cfg.Password, "smtp-secret")
	}
	if cfg.Host != "smtp.example.com" {
		t.Errorf("Host = %q, want %q", cfg.Host, "smtp.example.com")
	}
	if cfg.Port != 465 {
		t.Errorf("Port = %d, want %d", cfg.Port, 465)
	}
	// Username 为空时应回退到 Email
	if cfg.Username != "smtp@example.com" {
		t.Errorf("Username = %q, want %q", cfg.Username, "smtp@example.com")
	}
	const wantSecurity = "ssl"
	if string(cfg.Security) != wantSecurity {
		t.Errorf("Security = %q, want %q", cfg.Security, wantSecurity)
	}
}

func TestTouchLastSyncSetsTimestamp(t *testing.T) {
	svc, _, _ := newSvc(t)

	created, err := svc.Create(account.CreateAccountRequest{
		Name:     "Sync User",
		Email:    "sync@example.com",
		Password: "pw",
		IMAPHost: "imap.example.com",
		IMAPPort: 993,
		SMTPHost: "smtp.example.com",
		SMTPPort: 465,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	now := time.Now()
	if err := svc.TouchLastSync(created.ID, now); err != nil {
		t.Fatalf("TouchLastSync: %v", err)
	}

	resp, err := svc.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.LastSyncAt == nil {
		t.Fatal("LastSyncAt is nil, want non-nil")
	}
}
