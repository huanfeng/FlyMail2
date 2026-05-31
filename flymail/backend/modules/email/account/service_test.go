package account_test

import (
	"errors"
	"testing"

	"flymail/modules/email/account"
)

func TestServiceCreateEncryptsAndHidesPassword(t *testing.T) {
	svc, repo, enc := newSvc(t)
	resp, err := svc.Create(account.CreateAccountRequest{
		Name: "Work", Email: "u@example.com", Password: "p@ss",
		IMAPHost: "imap.example.com", IMAPPort: 993, IMAPSecurity: "ssl",
		SMTPHost: "smtp.example.com", SMTPPort: 465, SMTPSecurity: "ssl",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resp.ID == 0 {
		t.Fatal("应返回新 ID")
	}
	raw, err := repo.GetByID(resp.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if raw.PasswordEnc == "" || raw.PasswordEnc == "p@ss" {
		t.Errorf("密码未加密存储: %q", raw.PasswordEnc)
	}
	dec, _ := enc.Decrypt(raw.PasswordEnc)
	if dec != "p@ss" {
		t.Errorf("解密结果 = %q, want p@ss", dec)
	}
}

func TestServiceUpdateKeepsPasswordWhenEmpty(t *testing.T) {
	svc, repo, _ := newSvc(t)
	created, _ := svc.Create(account.CreateAccountRequest{
		Name: "W", Email: "k@example.com", Password: "orig",
		IMAPHost: "h", IMAPPort: 993, SMTPHost: "h", SMTPPort: 465,
	})
	before, _ := repo.GetByID(created.ID)

	if _, err := svc.Update(created.ID, account.UpdateAccountRequest{
		Name: "W2", Email: "k@example.com", Password: "",
		IMAPHost: "h", IMAPPort: 993, SMTPHost: "h", SMTPPort: 465,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	after, _ := repo.GetByID(created.ID)
	if after.PasswordEnc != before.PasswordEnc {
		t.Error("空密码更新不应改变已存密码")
	}
	if after.Name != "W2" {
		t.Errorf("Name 未更新: %q", after.Name)
	}
}

func TestServiceGetNotFound(t *testing.T) {
	svc, _, _ := newSvc(t)
	if _, err := svc.Get(999); !errors.Is(err, account.ErrAccountNotFound) {
		t.Errorf("want ErrAccountNotFound, got %v", err)
	}
}
