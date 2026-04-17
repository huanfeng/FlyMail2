package channels

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"mail2im/internal/core"
	"net/http"
)

type TelegramChannel struct {
	minPriority core.EventPriority
	token       string
	chatID      string
	template    string
}

func NewTelegramChannelWithConfig(token, chatID string, minPriority core.EventPriority, tmpl string) *TelegramChannel {
	return &TelegramChannel{
		minPriority: minPriority,
		token:       token,
		chatID:      chatID,
		template:    tmpl,
	}
}

func (t *TelegramChannel) Name() string {
	return "Telegram"
}

func (t *TelegramChannel) MinPriority() core.EventPriority {
	return t.minPriority
}

func (t *TelegramChannel) TemplateContent() string {
	return t.template
}

// Send sends an event using the legacy formatting (fallback).
func (t *TelegramChannel) Send(event core.Event) error {
	_, _, err := t.SendWithDetails(event)
	return err
}

func (t *TelegramChannel) SendWithDetails(event core.Event) (string, string, error) {
	message := t.formatFallback(event)
	return t.sendMessage(message)
}

// SendRendered sends a pre-rendered template message.
func (t *TelegramChannel) SendRendered(rendered string, event core.Event) error {
	_, _, err := t.SendRenderedWithDetails(rendered, event)
	return err
}

func (t *TelegramChannel) SendRenderedWithDetails(rendered string, event core.Event) (string, string, error) {
	return t.sendMessage(rendered)
}

func (t *TelegramChannel) sendMessage(message string) (string, string, error) {
	if t.token == "" || t.chatID == "" {
		return "", "", nil
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	payload := map[string]any{
		"chat_id":    t.chatID,
		"text":       message,
		"parse_mode": "HTML",
	}

	jsonPayload, _ := json.Marshal(payload)
	reqDetail := string(jsonPayload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return reqDetail, "", err
	}
	defer resp.Body.Close()

	var respBuf bytes.Buffer
	_, _ = respBuf.ReadFrom(resp.Body)
	respDetail := fmt.Sprintf("%s %s", resp.Status, respBuf.String())

	if resp.StatusCode != http.StatusOK {
		return reqDetail, respDetail, fmt.Errorf("telegram api returned status: %s", resp.Status)
	}

	return reqDetail, respDetail, nil
}

// formatFallback is the legacy format used when no template is rendered upstream.
func (t *TelegramChannel) formatFallback(event core.Event) string {
	switch event.Type {
	case core.EventEmailReceived:
		payload, ok := event.Payload.(map[string]any)
		if !ok {
			return "📧 <b>New Email Received</b>"
		}
		subject, _ := payload["subject"].(string)
		from, _ := payload["from"].(string)
		subject = html.EscapeString(subject)
		from = html.EscapeString(from)
		return fmt.Sprintf("📧 <b>New Email Received</b>\nFrom: %s\nSubject: %s", from, subject)
	default:
		return fmt.Sprintf("🔔 <b>%s</b>\n%v", event.Type, event.Payload)
	}
}
