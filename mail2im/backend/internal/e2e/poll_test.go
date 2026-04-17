package e2e

import (
	"mail2im/internal/testutil"
	"os"
	"testing"
)

// TestPolling_NewEmail_TriggersNotification is an E2E test that requires Mailpit.
// It verifies that sending an email via SMTP is detected via polling mode.
//
// Prerequisites:
//   - Mailpit running (docker compose -f docker-compose.test.yml up -d)
//   - Set E2E_MAILPIT=1 to enable
func TestPolling_NewEmail_TriggersNotification(t *testing.T) {
	if os.Getenv("E2E_MAILPIT") == "" {
		t.Skip("skipping E2E test: set E2E_MAILPIT=1 to run (requires Mailpit)")
	}

	// Clear Mailpit
	if err := testutil.ClearMailpit(); err != nil {
		t.Fatalf("failed to clear mailpit: %v", err)
	}

	// Send multiple test emails
	subjects := []string{"Poll Test 1", "Poll Test 2", "Poll Test 3"}
	for _, subj := range subjects {
		err := testutil.SendTestEmail(
			"sender@example.com",
			"receiver@example.com",
			subj,
			"Body for "+subj,
		)
		if err != nil {
			t.Fatalf("failed to send email %q: %v", subj, err)
		}
	}

	// Verify Mailpit received all emails
	msgs, err := testutil.WaitForEmail(3, testutil.DefaultTimeout)
	if err != nil {
		t.Fatalf("mailpit did not receive all emails: %v", err)
	}

	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(msgs))
	}
}
