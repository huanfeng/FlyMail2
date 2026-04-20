package channels

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mail2im/internal/core"
	"net/http"
	"strconv"
	"time"
)

type FeishuChannel struct {
	webhookURL  string
	signSecret  string
	minPriority core.EventPriority
	template    string
	client      *http.Client
}

func NewFeishuChannel(webhookURL, signSecret string, minPriority core.EventPriority, tmpl string) *FeishuChannel {
	return &FeishuChannel{
		webhookURL:  webhookURL,
		signSecret:  signSecret,
		minPriority: minPriority,
		template:    tmpl,
		client:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (f *FeishuChannel) Name() string {
	return "Feishu"
}

func (f *FeishuChannel) MinPriority() core.EventPriority {
	return f.minPriority
}

func (f *FeishuChannel) TemplateContent() string {
	return f.template
}

func (f *FeishuChannel) Send(event core.Event) error {
	_, _, err := f.SendWithDetails(event)
	return err
}

func (f *FeishuChannel) SendWithDetails(event core.Event) (string, string, error) {
	message := f.formatFallback(event)
	return f.sendCard(f.buildCardPayload(message, event))
}

func (f *FeishuChannel) SendRendered(rendered string, event core.Event) error {
	_, _, err := f.SendRenderedWithDetails(rendered, event)
	return err
}

func (f *FeishuChannel) SendRenderedWithDetails(rendered string, event core.Event) (string, string, error) {
	return f.sendCard(f.buildCardPayload(rendered, event))
}

func (f *FeishuChannel) buildCardPayload(content string, event core.Event) map[string]any {
	title := "Mail2IM Notification"
	headerTemplate := "blue"

	switch event.Type {
	case core.EventEmailReceived:
		title = "📧 New Email"
		headerTemplate = "blue"
	case core.EventAuthFailed:
		title = "🔐 Auth Failed"
		headerTemplate = "red"
	case core.EventSystemError:
		title = "⚠️ System Error"
		headerTemplate = "orange"
	}

	elements := []map[string]any{
		{
			"tag":  "div",
			"text": map[string]any{"tag": "lark_md", "content": content},
		},
	}

	return map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"header": map[string]any{
				"title":    map[string]any{"tag": "plain_text", "content": title},
				"template": headerTemplate,
			},
			"elements": elements,
		},
	}
}

func (f *FeishuChannel) sendCard(payload map[string]any) (string, string, error) {
	if f.webhookURL == "" {
		return "", "", nil
	}

	// Add signature if secret is configured
	if f.signSecret != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		sign, err := f.genSign(timestamp)
		if err != nil {
			return "", "", fmt.Errorf("feishu sign error: %w", err)
		}
		payload["timestamp"] = timestamp
		payload["sign"] = sign
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	reqDetail := string(body)

	resp, err := f.client.Post(f.webhookURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return reqDetail, "", err
	}
	defer resp.Body.Close()

	var respBuf bytes.Buffer
	_, _ = respBuf.ReadFrom(resp.Body)
	respDetail := fmt.Sprintf("%s %s", resp.Status, respBuf.String())

	if resp.StatusCode != http.StatusOK {
		return reqDetail, respDetail, fmt.Errorf("feishu webhook returned status: %s", resp.Status)
	}

	// Check response body for error code
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(respBuf.Bytes(), &result); err == nil && result.Code != 0 {
		return reqDetail, respDetail, fmt.Errorf("feishu api error: code=%d msg=%s", result.Code, result.Msg)
	}

	return reqDetail, respDetail, nil
}

// genSign computes the HMAC-SHA256 signature for Feishu webhook verification.
func (f *FeishuChannel) genSign(timestamp string) (string, error) {
	stringToSign := timestamp + "\n" + f.signSecret
	h := hmac.New(sha256.New, []byte(stringToSign))
	_, err := h.Write([]byte{})
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}

func (f *FeishuChannel) formatFallback(event core.Event) string {
	switch event.Type {
	case core.EventEmailReceived:
		payload, ok := event.Payload.(map[string]any)
		if !ok {
			return "**New Email Received**"
		}
		subject, _ := payload["subject"].(string)
		from, _ := payload["from"].(string)
		return fmt.Sprintf("**%s**\nFrom: %s", subject, from)
	default:
		return fmt.Sprintf("**%s**\n%v", event.Type, event.Payload)
	}
}
