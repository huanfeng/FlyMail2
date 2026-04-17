package dispatcher

import (
	"bytes"
	"fmt"
	"html"
	"mail2im/internal/core"
	"mail2im/internal/models"
	"strings"
	"text/template"
	"time"
)

// TemplateData is the unified data structure passed to all notification templates.
// All channels share this same data for consistent rendering.
type TemplateData struct {
	// Email info
	Subject     string // Email subject
	From        string // Full sender (John <john@ex.com>)
	FromName    string // Sender display name
	FromEmail   string // Sender email address
	To          string // Recipient
	BodyPreview string // Plain text first 200 chars

	// Metadata
	Mailbox    string // Folder name
	MailType   string // Classification key
	ReceivedAt string // Formatted time

	// Account
	AccountName  string
	AccountEmail string

	// System
	Priority  string // Low/Normal/High/Critical
	EventType string
	ViewLink  string // Online view link (requires base_url config)
}

// BuildTemplateData constructs a TemplateData from an Event.
func BuildTemplateData(event core.Event) TemplateData {
	data := TemplateData{
		EventType: string(event.Type),
		Priority:  priorityLabel(event.Priority),
	}

	payload, ok := event.Payload.(map[string]any)
	if !ok {
		return data
	}

	data.Subject, _ = payload["subject"].(string)
	data.From, _ = payload["from"].(string)
	data.To, _ = payload["to"].(string)
	data.Mailbox, _ = payload["mailbox"].(string)
	data.MailType, _ = payload["mail_type"].(string)

	// Parse From into name and email
	data.FromName, data.FromEmail = parseFromField(data.From)

	// ReceivedAt
	if t, ok := payload["received_at"].(time.Time); ok {
		data.ReceivedAt = t.In(core.GetSystemLocation()).Format("2006-01-02 15:04:05")
	} else {
		data.ReceivedAt = time.Now().In(core.GetSystemLocation()).Format("2006-01-02 15:04:05")
	}

	// Body preview — fetch from DB if we have email_id
	if emailID, ok := payload["email_id"].(string); ok && emailID != "" {
		var email models.Email
		if err := core.DB.Select("text_body, `to`").First(&email, "id = ?", emailID).Error; err == nil {
			data.BodyPreview = truncate(email.TextBody, 200)
			if data.To == "" {
				data.To = email.To
			}
		}
	}

	// Account info
	if accountID, ok := extractUint(payload, "account_id"); ok && accountID > 0 {
		var account models.Account
		if err := core.DB.Select("email, display_name").First(&account, accountID).Error; err == nil {
			data.AccountEmail = account.Email
			data.AccountName = account.DisplayName
			if data.AccountName == "" {
				data.AccountName = account.Email
			}
		}
	}

	// View link
	baseURL := core.GetSystemSettingWithDefault("base_url", "")
	if baseURL != "" {
		if emailID, ok := payload["email_id"].(string); ok && emailID != "" {
			data.ViewLink = strings.TrimRight(baseURL, "/") + "/emails/" + emailID
		}
	}

	return data
}

// RenderTemplate renders a Go template string with the given TemplateData.
// Returns the rendered string, or the fallback if template is empty or rendering fails.
func RenderTemplate(tmplContent string, data TemplateData, fallback string) string {
	if tmplContent == "" {
		return fallback
	}

	tmpl, err := template.New("notification").Parse(tmplContent)
	if err != nil {
		return fallback
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fallback
	}

	result := buf.String()
	if strings.TrimSpace(result) == "" {
		return fallback
	}
	return result
}

// RenderTemplateHTML renders a template and HTML-escapes the data fields first.
// Used for channels that interpret HTML (e.g., Telegram).
func RenderTemplateHTML(tmplContent string, data TemplateData, fallback string) string {
	escaped := data
	escaped.Subject = html.EscapeString(data.Subject)
	escaped.From = html.EscapeString(data.From)
	escaped.FromName = html.EscapeString(data.FromName)
	escaped.FromEmail = html.EscapeString(data.FromEmail)
	escaped.To = html.EscapeString(data.To)
	escaped.BodyPreview = html.EscapeString(data.BodyPreview)
	escaped.Mailbox = html.EscapeString(data.Mailbox)
	escaped.AccountName = html.EscapeString(data.AccountName)
	escaped.AccountEmail = html.EscapeString(data.AccountEmail)

	return RenderTemplate(tmplContent, escaped, fallback)
}

// DefaultFallbackMessage returns a simple notification string for an event.
func DefaultFallbackMessage(data TemplateData) string {
	switch core.EventType(data.EventType) {
	case core.EventEmailReceived:
		msg := fmt.Sprintf("New Email\nFrom: %s\nSubject: %s", data.From, data.Subject)
		if data.BodyPreview != "" {
			msg += "\n" + data.BodyPreview
		}
		return msg
	case core.EventAuthFailed:
		return fmt.Sprintf("Auth Failed: %s", data.AccountEmail)
	case core.EventSystemError:
		return fmt.Sprintf("System Error from %s", data.AccountEmail)
	default:
		return fmt.Sprintf("Event: %s", data.EventType)
	}
}

// DefaultFallbackHTML returns an HTML formatted fallback for Telegram-like channels.
func DefaultFallbackHTML(data TemplateData) string {
	switch core.EventType(data.EventType) {
	case core.EventEmailReceived:
		msg := fmt.Sprintf("📧 <b>New Email</b>\nFrom: %s\nSubject: %s",
			html.EscapeString(data.From), html.EscapeString(data.Subject))
		if data.BodyPreview != "" {
			msg += "\n\n" + html.EscapeString(data.BodyPreview)
		}
		return msg
	case core.EventAuthFailed:
		return fmt.Sprintf("🔐 <b>Auth Failed</b>\nAccount: %s", html.EscapeString(data.AccountEmail))
	case core.EventSystemError:
		return fmt.Sprintf("⚠️ <b>System Error</b>\nAccount: %s", html.EscapeString(data.AccountEmail))
	default:
		return fmt.Sprintf("🔔 <b>%s</b>", html.EscapeString(data.EventType))
	}
}

// SampleTemplateData returns example data for template preview.
func SampleTemplateData() TemplateData {
	return TemplateData{
		Subject:      "Your order #12345 has been shipped",
		From:         "Amazon <noreply@amazon.com>",
		FromName:     "Amazon",
		FromEmail:    "noreply@amazon.com",
		To:           "user@example.com",
		BodyPreview:  "Your package is on its way! Track your delivery at...",
		Mailbox:      "INBOX",
		MailType:     "primary",
		ReceivedAt:   time.Now().Format("2006-01-02 15:04:05"),
		AccountName:  "My Gmail",
		AccountEmail: "user@gmail.com",
		Priority:     "Normal",
		EventType:    "email_received",
		ViewLink:     "https://mail2im.example.com/emails/abc123",
	}
}

// TemplateVariableInfo describes a template variable for the API.
type TemplateVariableInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Example     string `json:"example"`
}

// GetTemplateVariables returns all available template variables with descriptions.
func GetTemplateVariables() []TemplateVariableInfo {
	sample := SampleTemplateData()
	return []TemplateVariableInfo{
		{Name: "Subject", Description: "Email subject line", Example: sample.Subject},
		{Name: "From", Description: "Full sender (name + email)", Example: sample.From},
		{Name: "FromName", Description: "Sender display name", Example: sample.FromName},
		{Name: "FromEmail", Description: "Sender email address", Example: sample.FromEmail},
		{Name: "To", Description: "Recipient email", Example: sample.To},
		{Name: "BodyPreview", Description: "Plain text preview (first 200 chars)", Example: sample.BodyPreview},
		{Name: "Mailbox", Description: "Folder name", Example: sample.Mailbox},
		{Name: "MailType", Description: "Classification (primary, bill, spam, etc.)", Example: sample.MailType},
		{Name: "ReceivedAt", Description: "Formatted receive time", Example: sample.ReceivedAt},
		{Name: "AccountName", Description: "Account display name", Example: sample.AccountName},
		{Name: "AccountEmail", Description: "Account email address", Example: sample.AccountEmail},
		{Name: "Priority", Description: "Event priority (Low/Normal/High/Critical)", Example: sample.Priority},
		{Name: "EventType", Description: "Event type (email_received, auth_failed, etc.)", Example: sample.EventType},
		{Name: "ViewLink", Description: "Link to view email online (requires base_url)", Example: sample.ViewLink},
	}
}

// --- helpers ---

func parseFromField(from string) (name, email string) {
	// Format: "Name <email@example.com>" or just "email@example.com"
	if idx := strings.Index(from, "<"); idx > 0 {
		name = strings.TrimSpace(from[:idx])
		email = strings.Trim(from[idx:], "<> ")
	} else {
		email = strings.TrimSpace(from)
		name = email
	}
	return
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func priorityLabel(p core.EventPriority) string {
	switch p {
	case core.PriorityLow:
		return "Low"
	case core.PriorityNormal:
		return "Normal"
	case core.PriorityHigh:
		return "High"
	case core.PriorityCritical:
		return "Critical"
	default:
		return "Normal"
	}
}

func extractUint(m map[string]any, key string) (uint, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case float64:
		return uint(val), true
	case int:
		return uint(val), true
	case uint:
		return val, true
	}
	return 0, false
}
