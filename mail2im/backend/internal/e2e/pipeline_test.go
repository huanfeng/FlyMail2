package e2e

import (
	"mail2im/internal/core"
	"mail2im/internal/dispatcher"
	"mail2im/internal/models"
	"mail2im/internal/testutil"
	"strings"
	"testing"
	"time"
)

// TestPipeline_EmailEventToMockChannel verifies the full pipeline:
// Event → Strategy → MailType routing → Channel delivery
func TestPipeline_EmailEventToMockChannel(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// Create a MailType "primary" with action=notify
	db.Create(&models.MailType{
		Key:      "primary",
		Name:     "Primary",
		Priority: 20,
		IsSystem: true,
		Action:   "notify",
	})

	// Set up EventBus
	core.InitEventBus()

	// Set up Dispatcher with a MockChannel
	mock := testutil.NewMockChannel("test-channel", core.PriorityLow)

	d := dispatcher.NewTestDispatcher()
	dispatcher.Instance = d
	d.Register(mock)

	// Subscribe dispatcher to events
	core.Bus.Subscribe(core.EventEmailReceived, d.HandleEventForTest)

	// Publish an email event
	core.Bus.Publish(core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityHigh,
		Source:   "account:1",
		Payload: map[string]any{
			"email_id":   "test-email-1",
			"subject":    "Invoice #1234",
			"from":       "billing@example.com",
			"mailbox":    "INBOX",
			"mail_type":  "primary",
			"account_id": uint(1),
		},
	})

	// Wait for delivery
	if !mock.WaitForEvent(5 * time.Second) {
		t.Fatal("expected mock channel to receive an event within 5s")
	}

	events := mock.Events()
	if len(events) == 0 {
		t.Fatal("expected at least one event delivered to mock channel")
	}

	payload, ok := events[0].Payload.(map[string]any)
	if !ok {
		t.Fatal("expected payload to be map[string]any")
	}
	if payload["subject"] != "Invoice #1234" {
		t.Errorf("expected subject 'Invoice #1234', got %v", payload["subject"])
	}
}

// TestPipeline_IgnoredMailTypeNotDelivered verifies that mail types with
// action="ignore" are not delivered to any channel.
func TestPipeline_IgnoredMailTypeNotDelivered(t *testing.T) {
	db := testutil.SetupTestDB(t)

	db.Create(&models.MailType{
		Key:      "spam",
		Name:     "Spam",
		Priority: 0,
		IsSystem: true,
		Action:   "ignore",
	})

	core.InitEventBus()

	mock := testutil.NewMockChannel("test-channel", core.PriorityLow)

	d := dispatcher.NewTestDispatcher()
	dispatcher.Instance = d
	d.Register(mock)
	core.Bus.Subscribe(core.EventEmailReceived, d.HandleEventForTest)

	core.Bus.Publish(core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Source:   "account:1",
		Payload: map[string]any{
			"email_id":   "spam-email-1",
			"subject":    "Buy cheap stuff",
			"from":       "spammer@example.com",
			"mailbox":    "Spam",
			"mail_type":  "spam",
			"account_id": uint(1),
		},
	})

	// Wait briefly — event should NOT arrive
	if mock.WaitForEvent(1 * time.Second) {
		t.Error("expected no event delivery for ignored mail type, but got one")
	}
}

// TestPipeline_SilentMailTypeNotDelivered verifies that mail types with
// action="silent" are not delivered.
func TestPipeline_SilentMailTypeNotDelivered(t *testing.T) {
	db := testutil.SetupTestDB(t)

	db.Create(&models.MailType{
		Key:      "promotion",
		Name:     "Promotion",
		Priority: 0,
		IsSystem: true,
		Action:   "silent",
	})

	core.InitEventBus()

	mock := testutil.NewMockChannel("test-channel", core.PriorityLow)

	d := dispatcher.NewTestDispatcher()
	dispatcher.Instance = d
	d.Register(mock)
	core.Bus.Subscribe(core.EventEmailReceived, d.HandleEventForTest)

	core.Bus.Publish(core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Source:   "account:1",
		Payload: map[string]any{
			"email_id":   "promo-1",
			"subject":    "50% off sale",
			"from":       "promo@shop.com",
			"mailbox":    "Promotions",
			"mail_type":  "promotion",
			"account_id": uint(1),
		},
	})

	if mock.WaitForEvent(1 * time.Second) {
		t.Error("expected no event delivery for silent mail type, but got one")
	}
}

// TestPipeline_BlockPatternStopsDelivery verifies that block patterns
// prevent event delivery.
func TestPipeline_BlockPatternStopsDelivery(t *testing.T) {
	db := testutil.SetupTestDB(t)

	db.Create(&models.MailType{
		Key:      "primary",
		Name:     "Primary",
		Priority: 20,
		IsSystem: true,
		Action:   "notify",
	})

	core.InitEventBus()

	mock := testutil.NewMockChannel("test-channel", core.PriorityLow)

	d := dispatcher.NewDispatcherWithStrategy(dispatcher.StrategyConfig{
		BlockPatterns: []string{"blocked-sender"},
	})
	dispatcher.Instance = d
	d.Register(mock)
	core.Bus.Subscribe(core.EventEmailReceived, d.HandleEventForTest)

	core.Bus.Publish(core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Source:   "blocked-sender",
		Payload: map[string]any{
			"email_id":   "blocked-1",
			"subject":    "Hello",
			"from":       "blocked-sender@example.com",
			"mailbox":    "INBOX",
			"mail_type":  "primary",
			"account_id": uint(1),
		},
	})

	if mock.WaitForEvent(1 * time.Second) {
		t.Error("expected no event delivery for blocked source, but got one")
	}
}

// TestPipeline_TemplateRendering verifies that template data can be built
// from pipeline events with email records in DB.
func TestPipeline_TemplateRendering(t *testing.T) {
	db := testutil.SetupTestDB(t)

	db.Create(&models.MailType{
		Key:      "primary",
		Name:     "Primary",
		Priority: 20,
		IsSystem: true,
		Action:   "notify",
	})

	// Create an email record for BuildTemplateData to find
	db.Create(&models.Email{
		ID:          "test-tmpl-email",
		AccountID:   1,
		Subject:     "Template Test Subject",
		From:        "sender@example.com",
		To:          "me@example.com",
		TextBody:    "This is the body text for template testing.",
		MailType:    "primary",
		Mailbox:     "INBOX",
		MailboxPath: "INBOX",
		ReceivedAt:  time.Now(),
	})

	core.InitEventBus()

	mock := testutil.NewMockChannel("test-channel", core.PriorityLow)

	d := dispatcher.NewTestDispatcher()
	dispatcher.Instance = d
	d.Register(mock)
	core.Bus.Subscribe(core.EventEmailReceived, d.HandleEventForTest)

	core.Bus.Publish(core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityHigh,
		Source:   "account:1",
		Payload: map[string]any{
			"email_id":   "test-tmpl-email",
			"subject":    "Template Test Subject",
			"from":       "sender@example.com",
			"mailbox":    "INBOX",
			"mail_type":  "primary",
			"account_id": uint(1),
		},
	})

	if !mock.WaitForEvent(5 * time.Second) {
		t.Fatal("expected event delivery within 5s")
	}

	events := mock.Events()
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	// Verify template data can be built for this event
	data := dispatcher.BuildTemplateData(events[0])
	if data.Subject != "Template Test Subject" {
		t.Errorf("expected subject 'Template Test Subject', got %q", data.Subject)
	}
	if !strings.Contains(data.BodyPreview, "body text for template") {
		t.Errorf("expected body preview to contain email body, got %q", data.BodyPreview)
	}
}
