package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"time"
)

const (
	DefaultMailpitSMTP = "localhost:1025"
	DefaultMailpitAPI  = "http://localhost:8025"
	DefaultTimeout     = 10 * time.Second
)

// SendTestEmail sends an email via Mailpit's SMTP server.
func SendTestEmail(from, to, subject, body string) error {
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		from, to, subject, body)

	return smtp.SendMail(DefaultMailpitSMTP, nil, from, []string{to}, []byte(msg))
}

// MailpitMessage represents a message from the Mailpit API.
type MailpitMessage struct {
	ID      string `json:"ID"`
	Subject string `json:"Subject"`
}

// MailpitResponse represents the Mailpit messages API response.
type MailpitResponse struct {
	Messages []MailpitMessage `json:"messages"`
	Total    int              `json:"total"`
}

// WaitForEmail polls Mailpit API until at least expectedCount emails arrive or timeout.
func WaitForEmail(expectedCount int, timeout time.Duration) ([]MailpitMessage, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(DefaultMailpitAPI + "/api/v1/messages")
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		defer resp.Body.Close()

		data, _ := io.ReadAll(resp.Body)
		var result MailpitResponse
		if err := json.Unmarshal(data, &result); err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if result.Total >= expectedCount {
			return result.Messages, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("timeout waiting for %d emails", expectedCount)
}

// ClearMailpit deletes all messages in Mailpit.
func ClearMailpit() error {
	req, _ := http.NewRequest("DELETE", DefaultMailpitAPI+"/api/v1/messages", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
