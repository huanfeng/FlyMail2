package account_test

import (
	"errors"
	"testing"

	"flymail/modules/email/account"
)

func TestSetEnabledAndDefault(t *testing.T) {
	svc, _, _ := newSvc(t)

	// 新建账户
	resp, err := svc.Create(account.CreateAccountRequest{
		Name: "Test", Email: "test@example.com", Password: "pw",
		IMAPHost: "imap.example.com", IMAPPort: 993,
		SMTPHost: "smtp.example.com", SMTPPort: 465,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// 新建账户默认 Enabled == true
	got, err := svc.Get(resp.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Enabled {
		t.Errorf("新账户 Enabled 应为 true，got false")
	}

	// SetEnabled(false) 后 Get.Enabled == false
	if err := svc.SetEnabled(resp.ID, false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	got, err = svc.Get(resp.ID)
	if err != nil {
		t.Fatalf("Get after disable: %v", err)
	}
	if got.Enabled {
		t.Errorf("SetEnabled(false) 后 Enabled 应为 false，got true")
	}

	// SetEnabled(true) 后 Get.Enabled == true
	if err := svc.SetEnabled(resp.ID, true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	got, err = svc.Get(resp.ID)
	if err != nil {
		t.Fatalf("Get after enable: %v", err)
	}
	if !got.Enabled {
		t.Errorf("SetEnabled(true) 后 Enabled 应为 true，got false")
	}
}

func TestSetEnabledNotFound(t *testing.T) {
	svc, _, _ := newSvc(t)
	err := svc.SetEnabled(9999, false)
	if !errors.Is(err, account.ErrAccountNotFound) {
		t.Errorf("want ErrAccountNotFound, got %v", err)
	}
}

func TestIsEnabledNotFound(t *testing.T) {
	svc, _, _ := newSvc(t)
	_, err := svc.IsEnabled(9999)
	if !errors.Is(err, account.ErrAccountNotFound) {
		t.Errorf("want ErrAccountNotFound, got %v", err)
	}
}
