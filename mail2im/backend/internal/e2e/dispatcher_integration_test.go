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

// TestPipeline_ChannelIDRouting verifies that MailType.ChannelIDs correctly
// routes events only to the specified channels.
func TestPipeline_ChannelIDRouting(t *testing.T) {
	db := testutil.SetupTestDB(t)
	core.InitEventBus()

	// Create two channels in DB
	db.Create(&models.Channel{Name: "ch1", Type: "telegram", Status: "enabled", Config: `{"token":"t","chat_id":"c"}`})
	db.Create(&models.Channel{Name: "ch2", Type: "telegram", Status: "enabled", Config: `{"token":"t","chat_id":"c"}`})

	// primary routes ONLY to channel 1
	db.Model(&models.MailType{}).Where("key = ?", "primary").Updates(map[string]any{
		"channel_ids": "[1]",
		"action":      "notify",
	})

	mock1 := testutil.NewMockChannel("ch1", core.PriorityLow)
	mock2 := testutil.NewMockChannel("ch2", core.PriorityLow)

	d := dispatcher.NewTestDispatcher()
	dispatcher.Instance = d

	// Register with IDs matching DB
	d.RegisterWithID(1, "ch1", "telegram", mock1, "")
	d.RegisterWithID(2, "ch2", "telegram", mock2, "")

	core.Bus.Subscribe(core.EventEmailReceived, d.HandleEventForTest)

	core.Bus.Publish(core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Source:   "account:1",
		Payload: map[string]any{
			"email_id":   "route-test-1",
			"subject":    "Routing test",
			"from":       "test@example.com",
			"mailbox":    "INBOX",
			"mail_type":  "primary",
			"account_id": uint(1),
		},
	})

	// Channel 1 should receive
	if !mock1.WaitForEvent(3 * time.Second) {
		t.Error("expected ch1 to receive event (target channel)")
	}
	// Channel 2 should NOT receive
	if mock2.WaitForEvent(500 * time.Millisecond) {
		t.Error("expected ch2 to NOT receive event (not in channel_ids)")
	}
}

// TestPipeline_PriorityFiltering verifies that channels with high min_priority
// do not receive low-priority events.
func TestPipeline_PriorityFiltering(t *testing.T) {
	testutil.SetupTestDB(t)
	core.InitEventBus()

	mockLow := testutil.NewMockChannel("low-ch", core.PriorityLow)
	mockHigh := testutil.NewMockChannel("high-ch", core.PriorityHigh)

	d := dispatcher.NewTestDispatcher()
	dispatcher.Instance = d
	d.Register(mockLow)
	d.Register(mockHigh)

	core.Bus.Subscribe(core.EventEmailReceived, d.HandleEventForTest)

	core.Bus.Publish(core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal, // Normal < High
		Source:   "account:1",
		Payload: map[string]any{
			"email_id":   "prio-1",
			"subject":    "Low prio event",
			"from":       "test@example.com",
			"mailbox":    "INBOX",
			"mail_type":  "primary",
			"account_id": uint(1),
		},
	})

	if !mockLow.WaitForEvent(3 * time.Second) {
		t.Error("expected low-threshold channel to receive normal-priority event")
	}
	if mockHigh.WaitForEvent(500 * time.Millisecond) {
		t.Error("expected high-threshold channel to NOT receive normal-priority event")
	}
}

// TestPipeline_QuietModeOff verifies that a channel with quiet_mode=off
// always receives events even during global quiet hours.
func TestPipeline_QuietModeOff(t *testing.T) {
	testutil.SetupTestDB(t)
	core.InitEventBus()

	mock := testutil.NewMockChannel("quiet-off", core.PriorityLow)

	// Create dispatcher with global quiet always active (00:00-23:59)
	d := dispatcher.NewDispatcherWithStrategy(dispatcher.StrategyConfig{
		QuietEnabled:    true,
		QuietHoursStart: "00:00",
		QuietHoursEnd:   "23:59",
	})
	dispatcher.Instance = d

	// Register with quiet mode = "off" (bypasses global quiet)
	d.RegisterWithQuiet(mock, "off", false, "", "")

	core.Bus.Subscribe(core.EventEmailReceived, d.HandleEventForTest)

	core.Bus.Publish(core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Source:   "account:1",
		Payload: map[string]any{
			"email_id":   "quiet-1",
			"subject":    "During quiet hours",
			"from":       "test@example.com",
			"mailbox":    "INBOX",
			"mail_type":  "primary",
			"account_id": uint(1),
		},
	})

	if !mock.WaitForEvent(3 * time.Second) {
		t.Error("expected channel with quiet_mode=off to receive event during quiet hours")
	}
}

// TestPipeline_VerificationCodeInTemplateData verifies that BuildTemplateData
// correctly extracts verification codes from email body.
func TestPipeline_VerificationCodeInTemplateData(t *testing.T) {
	db := testutil.SetupTestDB(t)

	db.Create(&models.Email{
		ID:          "vcode-email-1",
		AccountID:   1,
		Subject:     "Your login code",
		From:        "noreply@service.com",
		To:          "user@example.com",
		TextBody:    "Your verification code is 847291. Valid for 5 minutes.",
		MailType:    "primary",
		Mailbox:     "INBOX",
		MailboxPath: "INBOX",
		ReceivedAt:  time.Now(),
	})

	event := core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Source:   "account:1",
		Payload: map[string]any{
			"email_id":   "vcode-email-1",
			"subject":    "Your login code",
			"from":       "noreply@service.com",
			"mailbox":    "INBOX",
			"mail_type":  "primary",
			"account_id": uint(1),
		},
	}

	data := dispatcher.BuildTemplateData(event)

	if !data.IsVerificationCode {
		t.Error("expected IsVerificationCode=true")
	}
	if data.VerificationCode != "847291" {
		t.Errorf("expected VerificationCode='847291', got %q", data.VerificationCode)
	}
}

// TestPipeline_BodyContentFromDB verifies that BuildTemplateData populates
// BodyContent with full email body and BodyPreview with truncated version.
func TestPipeline_BodyContentFromDB(t *testing.T) {
	db := testutil.SetupTestDB(t)

	longBody := strings.Repeat("This is a sentence with many words. ", 20) // ~720 chars
	db.Create(&models.Email{
		ID:          "body-email-1",
		AccountID:   1,
		Subject:     "Long email",
		From:        "sender@example.com",
		To:          "user@example.com",
		TextBody:    longBody,
		MailType:    "primary",
		Mailbox:     "INBOX",
		MailboxPath: "INBOX",
		ReceivedAt:  time.Now(),
	})

	event := core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Payload: map[string]any{
			"email_id":   "body-email-1",
			"subject":    "Long email",
			"from":       "sender@example.com",
			"mailbox":    "INBOX",
			"mail_type":  "primary",
			"account_id": uint(1),
		},
	}

	data := dispatcher.BuildTemplateData(event)

	// BodyPreview should be truncated to 200 chars
	if len([]rune(data.BodyPreview)) > 203 { // 200 + "..."
		t.Errorf("expected BodyPreview <= 203 runes, got %d", len([]rune(data.BodyPreview)))
	}

	// BodyContent should contain more than BodyPreview
	if len(data.BodyContent) <= len(data.BodyPreview) {
		t.Errorf("expected BodyContent longer than BodyPreview, got %d vs %d",
			len(data.BodyContent), len(data.BodyPreview))
	}

	// BodyContent should contain the full body (under 10000 cap)
	if data.BodyContent != longBody {
		t.Errorf("expected BodyContent to equal full body")
	}
}
