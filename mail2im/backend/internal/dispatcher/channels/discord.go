package channels

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mail2im/internal/core"
	"net/http"
	"time"
)

type DiscordChannel struct {
	WebhookURL  string
	minPriority core.EventPriority
	template    string
	client      *http.Client
}

func NewDiscordChannel(webhookURL string, minPriority core.EventPriority, tmpl string) *DiscordChannel {
	return &DiscordChannel{
		WebhookURL:  webhookURL,
		minPriority: minPriority,
		template:    tmpl,
		client:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *DiscordChannel) Name() string {
	return "Discord"
}

func (c *DiscordChannel) MinPriority() core.EventPriority {
	return c.minPriority
}

func (c *DiscordChannel) TemplateContent() string {
	return c.template
}

// Send sends an event using legacy formatting.
func (c *DiscordChannel) Send(event core.Event) error {
	_, _, err := c.SendWithDetails(event)
	return err
}

func (c *DiscordChannel) SendWithDetails(event core.Event) (string, string, error) {
	description := fmt.Sprintf("**Source:** %s\n**Priority:** %d", event.Source, event.Priority)
	return c.sendEmbed(fmt.Sprintf("Mail2IM: %s", event.Type), description, c.colorForPriority(event.Priority))
}

// SendRendered sends a pre-rendered template message in a Discord embed.
func (c *DiscordChannel) SendRendered(rendered string, event core.Event) error {
	_, _, err := c.SendRenderedWithDetails(rendered, event)
	return err
}

func (c *DiscordChannel) SendRenderedWithDetails(rendered string, event core.Event) (string, string, error) {
	title := "Mail2IM Notification"
	if event.Type == core.EventEmailReceived {
		title = "📧 New Email"
	}
	return c.sendEmbed(title, rendered, c.colorForPriority(event.Priority))
}

func (c *DiscordChannel) sendEmbed(title, description string, color int) (string, string, error) {
	embed := map[string]any{
		"title":       title,
		"description": description,
		"color":       color,
		"timestamp":   time.Now().Format(time.RFC3339),
	}

	payload := map[string]any{
		"embeds": []any{embed},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	reqDetail := string(body)

	resp, err := c.client.Post(c.WebhookURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return reqDetail, "", err
	}
	defer resp.Body.Close()

	var respBuf bytes.Buffer
	_, _ = respBuf.ReadFrom(resp.Body)
	respDetail := fmt.Sprintf("%s %s", resp.Status, respBuf.String())

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return reqDetail, respDetail, fmt.Errorf("discord webhook returned status: %s", resp.Status)
	}

	return reqDetail, respDetail, nil
}

func (c *DiscordChannel) colorForPriority(p core.EventPriority) int {
	switch {
	case p >= core.PriorityHigh:
		return 0xFF0000 // Red
	case p == core.PriorityNormal:
		return 0x0000FF // Blue
	default:
		return 0x00FF00 // Green
	}
}
