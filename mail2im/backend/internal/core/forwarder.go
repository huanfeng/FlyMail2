package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type WebhookPayload struct {
	Text    string `json:"text"` // Generic text field for many IMs
	Subject string `json:"subject"`
	From    string `json:"from"`
	Snippet string `json:"snippet"`
	Link    string `json:"link,omitempty"` // Optional link to view email
}

func ForwardEmail(webhookURL, webhookToken, from, subject, snippet string) error {
	if webhookURL == "" {
		return fmt.Errorf("webhook URL is not configured")
	}

	payload := WebhookPayload{
		Text:    fmt.Sprintf("📧 *New Email*\n**From:** %s\n**Subject:** %s\n\n%s", from, subject, snippet),
		Subject: subject,
		From:    from,
		Snippet: snippet,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if webhookToken != "" {
		req.Header.Set("Authorization", "Bearer "+webhookToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status: %d", resp.StatusCode)
	}

	return nil
}
