package send_test

import (
	"errors"
	"testing"
	"time"

	"flymail-core/types"

	"flymail/modules/email/account"
	"flymail/modules/email/folder"
	"flymail/modules/email/send"
)

// --- fakes ---

type fakeAccounts struct {
	acct    *account.AccountResponse
	smtpCfg types.SMTPConfig
	imapCfg types.IMAPConfig
	getErr  error
	smtpErr error
	imapErr error
}

func (f *fakeAccounts) Get(id uint) (*account.AccountResponse, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.acct, nil
}

func (f *fakeAccounts) SMTPConfig(id uint) (types.SMTPConfig, error) {
	return f.smtpCfg, f.smtpErr
}

func (f *fakeAccounts) IMAPConfig(id uint) (types.IMAPConfig, error) {
	return f.imapCfg, f.imapErr
}

type fakeFolders struct {
	folder *folder.Folder
	err    error
}

func (f *fakeFolders) FindByType(accountID uint, folderType string) (*folder.Folder, error) {
	return f.folder, f.err
}

// --- helpers ---

func newTestService(accounts *fakeAccounts, folders *fakeFolders) *send.Service {
	svc := send.NewService(accounts, folders)
	svc.SetNow(func() time.Time {
		return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	})
	return svc
}

func defaultAccounts() *fakeAccounts {
	return &fakeAccounts{
		acct: &account.AccountResponse{
			ID:    1,
			Email: "sender@example.com",
		},
		smtpCfg: types.SMTPConfig{Host: "smtp.example.com", Port: 465},
		imapCfg: types.IMAPConfig{Host: "imap.example.com", Port: 993},
	}
}

func sentFolder() *folder.Folder {
	return &folder.Folder{
		ID:        10,
		AccountID: 1,
		Path:      "Sent",
		Type:      "sent",
	}
}

// --- tests ---

// TestSendCallsSMTPAndAppend：有"已发送"文件夹时，sendFn 和 appendFn 都被调用。
func TestSendCallsSMTPAndAppend(t *testing.T) {
	accounts := defaultAccounts()
	folders := &fakeFolders{folder: sentFolder()}

	var capturedRecipients []string
	var capturedMailbox string
	var sendCalled, appendCalled bool

	svc := newTestService(accounts, folders)
	svc.SetSenders(
		func(cfg types.SMTPConfig, from string, recipients []string, raw []byte) error {
			sendCalled = true
			capturedRecipients = recipients
			return nil
		},
		func(cfg types.IMAPConfig, mailbox string, raw []byte) error {
			appendCalled = true
			capturedMailbox = mailbox
			return nil
		},
	)

	req := send.SendRequest{
		AccountID: 1,
		To:        []string{"alice@example.com"},
		Cc:        []string{"bob@example.com"},
		Bcc:       []string{"hidden@example.com"},
		Subject:   "Test",
		BodyHTML:  "<p>hello</p>",
	}

	if err := svc.Send(req); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if !sendCalled {
		t.Error("sendFn was not called")
	}

	// recipients 必须包含 To + Cc + Bcc
	wantRecipients := map[string]bool{
		"alice@example.com":  true,
		"bob@example.com":    true,
		"hidden@example.com": true,
	}
	for _, r := range capturedRecipients {
		if !wantRecipients[r] {
			t.Errorf("unexpected recipient: %s", r)
		}
		delete(wantRecipients, r)
	}
	for r := range wantRecipients {
		t.Errorf("missing recipient: %s", r)
	}

	if !appendCalled {
		t.Error("appendFn was not called")
	}
	if capturedMailbox != "Sent" {
		t.Errorf("appendFn mailbox = %q, want Sent", capturedMailbox)
	}
}

// TestSendNoSentFolderSkipsAppend：FindByType 返回 nil 时不调用 appendFn，但 Send 成功。
func TestSendNoSentFolderSkipsAppend(t *testing.T) {
	accounts := defaultAccounts()
	folders := &fakeFolders{folder: nil}

	var appendCalled bool

	svc := newTestService(accounts, folders)
	svc.SetSenders(
		func(cfg types.SMTPConfig, from string, recipients []string, raw []byte) error {
			return nil
		},
		func(cfg types.IMAPConfig, mailbox string, raw []byte) error {
			appendCalled = true
			return nil
		},
	)

	req := send.SendRequest{
		AccountID: 1,
		To:        []string{"alice@example.com"},
		Subject:   "No Sent Folder",
		BodyHTML:  "<p>test</p>",
	}

	if err := svc.Send(req); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if appendCalled {
		t.Error("appendFn should NOT be called when sent folder is nil")
	}
}

// TestSendNoRecipient：To 为空时返回 ErrNoRecipient。
func TestSendNoRecipient(t *testing.T) {
	accounts := defaultAccounts()
	folders := &fakeFolders{folder: nil}

	svc := newTestService(accounts, folders)
	svc.SetSenders(
		func(cfg types.SMTPConfig, from string, recipients []string, raw []byte) error {
			return nil
		},
		func(cfg types.IMAPConfig, mailbox string, raw []byte) error {
			return nil
		},
	)

	req := send.SendRequest{
		AccountID: 1,
		To:        []string{}, // 空
		Subject:   "No To",
		BodyHTML:  "<p>test</p>",
	}

	err := svc.Send(req)
	if !errors.Is(err, send.ErrNoRecipient) {
		t.Errorf("expected ErrNoRecipient, got: %v", err)
	}
}

// TestSendSMTPFailureDoesNotAppend：SMTP 发送失败时，appendFn 不应被调用，且 Send 返回错误。
func TestSendSMTPFailureDoesNotAppend(t *testing.T) {
	accounts := defaultAccounts()
	folders := &fakeFolders{folder: sentFolder()}

	var appendCalled bool
	smtpErr := errors.New("connection refused")

	svc := newTestService(accounts, folders)
	svc.SetSenders(
		func(cfg types.SMTPConfig, from string, recipients []string, raw []byte) error {
			return smtpErr
		},
		func(cfg types.IMAPConfig, mailbox string, raw []byte) error {
			appendCalled = true
			return nil
		},
	)

	req := send.SendRequest{
		AccountID: 1,
		To:        []string{"alice@example.com"},
		Subject:   "SMTP Fail",
		BodyHTML:  "<p>test</p>",
	}

	err := svc.Send(req)
	if err == nil {
		t.Fatal("expected error when SMTP fails, got nil")
	}
	if appendCalled {
		t.Error("appendFn should NOT be called when SMTP fails")
	}
}
