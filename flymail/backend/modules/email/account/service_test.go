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

func TestServiceHealthProviderInjection(t *testing.T) {
	svc, _, _ := newSvc(t)
	created, err := svc.Create(account.CreateAccountRequest{
		Name: "T", Email: "t@x.com", Username: "t", Password: "pw",
		IMAPHost: "127.0.0.1", IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "127.0.0.1", SMTPPort: 587, SMTPSecurity: "starttls",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Health != nil {
		t.Fatal("未注入 provider 时 Health 应缺省")
	}

	svc.SetHealthProvider(func(a *account.Account) *account.AccountHealthInfo {
		if a.ID == created.ID {
			return &account.AccountHealthInfo{
				NeedsReauth: false, BreakerOpen: true, Failures: 5, LastError: "dial tcp: timeout",
			}
		}
		return nil
	})

	got, err := svc.Get(created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Health == nil || !got.Health.BreakerOpen || got.Health.Failures != 5 {
		t.Fatalf("Get 未带出健康快照: %+v", got.Health)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var found *account.AccountHealthInfo
	for i := range list {
		if list[i].ID == created.ID {
			found = list[i].Health
		}
	}
	if found == nil {
		t.Fatal("List 未带出健康快照")
	}

	// provider 返回 nil（健康）→ health 字段缺省
	svc.SetHealthProvider(func(a *account.Account) *account.AccountHealthInfo { return nil })
	got2, _ := svc.Get(created.ID)
	if got2.Health != nil {
		t.Fatalf("健康账户 health 应缺省，got %+v", got2.Health)
	}
}
