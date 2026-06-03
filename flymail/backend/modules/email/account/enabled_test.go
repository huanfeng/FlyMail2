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

func TestSetEnabledEmitsAccountStatus(t *testing.T) {
	svc, _, _ := newSvc(t)
	resp, err := svc.Create(account.CreateAccountRequest{
		Name: "Acc", Email: "a@example.com", Password: "pw",
		IMAPHost: "imap.example.com", IMAPPort: 993,
		SMTPHost: "smtp.example.com", SMTPPort: 465,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var calls int
	var gotType string
	var gotAccount uint
	var gotBody string
	svc.SetEmitter(func(eventType string, accountID uint, title, body string) {
		calls++
		gotType = eventType
		gotAccount = accountID
		gotBody = body
	})

	if err := svc.SetEnabled(resp.ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if calls != 1 || gotType != "account_status" || gotAccount != resp.ID {
		t.Fatalf("应触发 account_status 通知: calls=%d type=%s account=%d", calls, gotType, gotAccount)
	}
	if gotBody == "" {
		t.Errorf("通知正文不应为空")
	}
}

func TestSetEnabledNoEmitOnError(t *testing.T) {
	svc, _, _ := newSvc(t)
	var calls int
	svc.SetEmitter(func(string, uint, string, string) { calls++ })
	// 账户不存在 → SetEnabled 失败，不应触发通知
	_ = svc.SetEnabled(9999, false)
	if calls != 0 {
		t.Errorf("失败时不应触发通知，calls=%d", calls)
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
