package draft_test

import (
	"errors"
	"testing"

	"flymail/modules/email/draft"
	"flymail/modules/email/send"
)

// fakeSender 记录 Send 调用并可注入错误。
type fakeSender struct {
	called  bool
	lastReq send.SendRequest
	err     error
}

func (f *fakeSender) Send(req send.SendRequest) error {
	f.called = true
	f.lastReq = req
	return f.err
}

func newServiceWithDB(t *testing.T) *draft.Service {
	t.Helper()
	db := setupTestDB(t)
	return draft.NewService(draft.NewRepository(db))
}

func TestService_CreateAndGet(t *testing.T) {
	svc := newServiceWithDB(t)

	req := draft.DraftRequest{
		AccountID: 1,
		To:        []string{"alice@example.com", "bob@example.com"},
		Cc:        []string{"carol@example.com"},
		Subject:   "Test",
		BodyHTML:  "<p>body</p>",
	}
	resp, err := svc.Create(req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resp.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if len(resp.To) != 2 {
		t.Errorf("expected 2 To addresses, got %d", len(resp.To))
	}
	if resp.To[0] != "alice@example.com" {
		t.Errorf("unexpected To[0]: %q", resp.To[0])
	}
	if len(resp.Cc) != 1 || resp.Cc[0] != "carol@example.com" {
		t.Errorf("Cc mismatch: %v", resp.Cc)
	}

	// Get 往返验证
	got, err := svc.Get(resp.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Subject != req.Subject {
		t.Errorf("Subject mismatch: got %q want %q", got.Subject, req.Subject)
	}
	if len(got.To) != 2 {
		t.Errorf("Get: expected 2 To, got %d", len(got.To))
	}
}

func TestService_Update(t *testing.T) {
	svc := newServiceWithDB(t)

	resp, _ := svc.Create(draft.DraftRequest{AccountID: 1, Subject: "Old", To: []string{"a@b.com"}})

	updated, err := svc.Update(resp.ID, draft.DraftRequest{
		AccountID: 1,
		Subject:   "New",
		To:        []string{"x@y.com", "z@y.com"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Subject != "New" {
		t.Errorf("expected New, got %q", updated.Subject)
	}
	if len(updated.To) != 2 {
		t.Errorf("expected 2 To after update, got %d", len(updated.To))
	}
}

func TestService_Update_NotFound(t *testing.T) {
	svc := newServiceWithDB(t)
	_, err := svc.Update(999, draft.DraftRequest{AccountID: 1})
	if !errors.Is(err, draft.ErrDraftNotFound) {
		t.Errorf("expected ErrDraftNotFound, got %v", err)
	}
}

func TestService_Delete(t *testing.T) {
	svc := newServiceWithDB(t)
	resp, _ := svc.Create(draft.DraftRequest{AccountID: 1, Subject: "Del"})

	if err := svc.Delete(resp.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := svc.Get(resp.ID)
	if !errors.Is(err, draft.ErrDraftNotFound) {
		t.Errorf("expected ErrDraftNotFound after Delete, got %v", err)
	}
}

func TestService_SendDraft_Success(t *testing.T) {
	svc := newServiceWithDB(t)
	sender := &fakeSender{}

	resp, _ := svc.Create(draft.DraftRequest{
		AccountID: 2,
		To:        []string{"recv@example.com"},
		Subject:   "SendMe",
		BodyHTML:  "<p>hi</p>",
	})

	if err := svc.SendDraft(resp.ID, sender); err != nil {
		t.Fatalf("SendDraft: %v", err)
	}

	// sender.Send 应被调用，且参数正确
	if !sender.called {
		t.Fatal("expected sender.Send to be called")
	}
	if sender.lastReq.AccountID != 2 {
		t.Errorf("AccountID mismatch: got %d", sender.lastReq.AccountID)
	}
	if len(sender.lastReq.To) != 1 || sender.lastReq.To[0] != "recv@example.com" {
		t.Errorf("To mismatch: %v", sender.lastReq.To)
	}

	// 草稿应被删除
	_, err := svc.Get(resp.ID)
	if !errors.Is(err, draft.ErrDraftNotFound) {
		t.Errorf("expected draft deleted after SendDraft, got %v", err)
	}
}

func TestService_SendDraft_SenderError_DraftKept(t *testing.T) {
	svc := newServiceWithDB(t)
	sender := &fakeSender{err: errors.New("smtp failure")}

	resp, _ := svc.Create(draft.DraftRequest{
		AccountID: 1,
		To:        []string{"x@example.com"},
		Subject:   "FailSend",
	})

	err := svc.SendDraft(resp.ID, sender)
	if err == nil {
		t.Fatal("expected error from SendDraft when sender fails")
	}

	// 发送失败时草稿不应被删除
	_, getErr := svc.Get(resp.ID)
	if getErr != nil {
		t.Errorf("draft should remain after send failure, got err: %v", getErr)
	}
}

func TestService_SendDraft_NotFound(t *testing.T) {
	svc := newServiceWithDB(t)
	sender := &fakeSender{}

	err := svc.SendDraft(999, sender)
	if !errors.Is(err, draft.ErrDraftNotFound) {
		t.Errorf("expected ErrDraftNotFound, got %v", err)
	}
	if sender.called {
		t.Error("sender should not be called for non-existent draft")
	}
}
