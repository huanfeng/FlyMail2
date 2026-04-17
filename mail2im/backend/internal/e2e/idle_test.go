package e2e

import (
	"mail2im/internal/testutil"
	"os"
	"testing"
)

// TestIDLE_NewEmail_TriggersNotification is an E2E test that requires Mailpit.
// It verifies that sending an email via SMTP triggers notification via IDLE mode.
//
// Prerequisites:
//   - Mailpit running (docker compose -f docker-compose.test.yml up -d)
//   - Set E2E_MAILPIT=1 to enable
func TestIDLE_NewEmail_TriggersNotification(t *testing.T) {
	if os.Getenv("E2E_MAILPIT") == "" {
		t.Skip("skipping E2E test: set E2E_MAILPIT=1 to run (requires Mailpit)")
	}

	// Clear Mailpit
	if err := testutil.ClearMailpit(); err != nil {
		t.Fatalf("failed to clear mailpit: %v", err)
	}

	// Send a test email
	err := testutil.SendTestEmail(
		"sender@example.com",
		"receiver@example.com",
		"IDLE Test Email",
		"This email tests IDLE detection.",
	)
	if err != nil {
		t.Fatalf("failed to send test email: %v", err)
	}

	// Verify Mailpit received it
	msgs, err := testutil.WaitForEmail(1, testutil.DefaultTimeout)
	if err != nil {
		t.Fatalf("mailpit did not receive email: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("expected at least 1 message in mailpit")
	}

	found := false
	for _, m := range msgs {
		if m.Subject == "IDLE Test Email" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'IDLE Test Email' in mailpit messages")
	}
}
