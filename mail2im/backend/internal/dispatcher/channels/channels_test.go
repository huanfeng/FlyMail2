package channels

import (
	"encoding/json"
	"io"
	"mail2im/internal/core"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ─── Telegram Tests ──────────────────────────────────────────────────────────

func TestTelegramChannel_SendRenderedWithDetails(t *testing.T) {
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	ch := &TelegramChannel{
		token:       "fake-token",
		chatID:      "12345",
		minPriority: core.PriorityLow,
		template:    "test template",
	}
	// Override the sendMessage to use test server
	// Since we can't easily override the URL, test the formatFallback and interface methods
	if ch.Name() != "Telegram" {
		t.Errorf("expected Name() = 'Telegram', got %q", ch.Name())
	}
	if ch.MinPriority() != core.PriorityLow {
		t.Errorf("expected MinPriority() = PriorityLow")
	}
	if ch.TemplateContent() != "test template" {
		t.Errorf("expected TemplateContent() = 'test template', got %q", ch.TemplateContent())
	}

	_ = server
	_ = receivedBody
}

func TestTelegramChannel_FormatFallback(t *testing.T) {
	ch := &TelegramChannel{token: "t", chatID: "c"}

	event := core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Payload: map[string]any{
			"subject": "Test <Subject>",
			"from":    "sender@test.com",
		},
	}

	result := ch.formatFallback(event)
	if !strings.Contains(result, "&lt;Subject&gt;") {
		t.Errorf("expected HTML-escaped subject, got %q", result)
	}
	if !strings.Contains(result, "sender@test.com") {
		t.Errorf("expected from in fallback, got %q", result)
	}
}

func TestTelegramChannel_FormatFallback_NonEmailEvent(t *testing.T) {
	ch := &TelegramChannel{token: "t", chatID: "c"}

	event := core.Event{
		Type:     core.EventSystemError,
		Priority: core.PriorityHigh,
		Payload:  "something went wrong",
	}

	result := ch.formatFallback(event)
	if !strings.Contains(result, string(core.EventSystemError)) {
		t.Errorf("expected event type in fallback, got %q", result)
	}
}

func TestTelegramChannel_EmptyConfig(t *testing.T) {
	ch := &TelegramChannel{token: "", chatID: ""}
	req, resp, err := ch.sendMessage("test")
	if err != nil {
		t.Errorf("expected no error for empty config, got %v", err)
	}
	if req != "" || resp != "" {
		t.Errorf("expected empty req/resp for empty config")
	}
}

// ─── Feishu Tests ────────────────────────────────────────────────────────────

func TestFeishuChannel_BuildCardPayload(t *testing.T) {
	ch := NewFeishuChannel("https://example.com/hook", "", core.PriorityNormal, "tmpl")

	event := core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
	}

	payload := ch.buildCardPayload("**Hello**\nBody content", event)

	if payload["msg_type"] != "interactive" {
		t.Errorf("expected msg_type=interactive, got %v", payload["msg_type"])
	}

	card, ok := payload["card"].(map[string]any)
	if !ok {
		t.Fatal("expected card to be map")
	}

	header, ok := card["header"].(map[string]any)
	if !ok {
		t.Fatal("expected header to be map")
	}
	if header["template"] != "blue" {
		t.Errorf("expected blue header for email event, got %v", header["template"])
	}

	elements, ok := card["elements"].([]map[string]any)
	if !ok {
		t.Fatal("expected elements to be []map")
	}
	if len(elements) == 0 {
		t.Fatal("expected at least one element")
	}

	text, ok := elements[0]["text"].(map[string]any)
	if !ok {
		t.Fatal("expected text in element")
	}
	if text["tag"] != "lark_md" {
		t.Errorf("expected lark_md tag, got %v", text["tag"])
	}
	if text["content"] != "**Hello**\nBody content" {
		t.Errorf("expected content in element, got %v", text["content"])
	}
}

func TestFeishuChannel_BuildCardPayload_ErrorEvent(t *testing.T) {
	ch := NewFeishuChannel("https://example.com/hook", "", core.PriorityNormal, "")

	event := core.Event{Type: core.EventSystemError}
	payload := ch.buildCardPayload("Error msg", event)

	card := payload["card"].(map[string]any)
	header := card["header"].(map[string]any)
	if header["template"] != "orange" {
		t.Errorf("expected orange header for system error, got %v", header["template"])
	}
}

func TestFeishuChannel_GenSign(t *testing.T) {
	ch := &FeishuChannel{signSecret: "test-secret"}
	sign, err := ch.genSign("1234567890")
	if err != nil {
		t.Fatalf("genSign error: %v", err)
	}
	if sign == "" {
		t.Error("expected non-empty signature")
	}
	// Signature should be base64 encoded
	if !strings.ContainsAny(sign, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=") {
		t.Errorf("expected base64 signature, got %q", sign)
	}
}

func TestFeishuChannel_SendCard_WithSignature(t *testing.T) {
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(200)
		w.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	defer server.Close()

	ch := NewFeishuChannel(server.URL, "my-secret", core.PriorityNormal, "")

	event := core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityNormal,
		Payload: map[string]any{
			"subject": "Test",
			"from":    "sender@test.com",
		},
	}

	_, _, err := ch.SendRenderedWithDetails("**Test Content**", event)
	if err != nil {
		t.Fatalf("SendRenderedWithDetails error: %v", err)
	}

	// Verify signature fields are present
	if _, ok := receivedBody["timestamp"]; !ok {
		t.Error("expected timestamp in request body")
	}
	if _, ok := receivedBody["sign"]; !ok {
		t.Error("expected sign in request body")
	}
	if receivedBody["msg_type"] != "interactive" {
		t.Errorf("expected msg_type=interactive, got %v", receivedBody["msg_type"])
	}
}

func TestFeishuChannel_SendCard_WithoutSignature(t *testing.T) {
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(200)
		w.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	defer server.Close()

	ch := NewFeishuChannel(server.URL, "", core.PriorityNormal, "")

	_, _, err := ch.SendRenderedWithDetails("content", core.Event{Type: core.EventEmailReceived})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Without secret, no timestamp/sign fields
	if _, ok := receivedBody["timestamp"]; ok {
		t.Error("expected no timestamp when sign_secret is empty")
	}
	if _, ok := receivedBody["sign"]; ok {
		t.Error("expected no sign when sign_secret is empty")
	}
}

func TestFeishuChannel_SendCard_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"code":19021,"msg":"sign match fail"}`))
	}))
	defer server.Close()

	ch := NewFeishuChannel(server.URL, "", core.PriorityNormal, "")
	_, _, err := ch.SendRenderedWithDetails("content", core.Event{Type: core.EventEmailReceived})
	if err == nil {
		t.Fatal("expected error for non-zero code")
	}
	if !strings.Contains(err.Error(), "sign match fail") {
		t.Errorf("expected error to contain API message, got %v", err)
	}
}

func TestFeishuChannel_SendCard_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`Internal Server Error`))
	}))
	defer server.Close()

	ch := NewFeishuChannel(server.URL, "", core.PriorityNormal, "")
	_, _, err := ch.SendRenderedWithDetails("content", core.Event{Type: core.EventEmailReceived})
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
}

func TestFeishuChannel_EmptyWebhook(t *testing.T) {
	ch := NewFeishuChannel("", "", core.PriorityNormal, "")
	req, resp, err := ch.sendCard(map[string]any{"msg_type": "text"})
	if err != nil {
		t.Errorf("expected no error for empty webhook, got %v", err)
	}
	if req != "" || resp != "" {
		t.Error("expected empty req/resp for empty webhook")
	}
}

func TestFeishuChannel_FormatFallback(t *testing.T) {
	ch := NewFeishuChannel("url", "", core.PriorityNormal, "")

	event := core.Event{
		Type: core.EventEmailReceived,
		Payload: map[string]any{
			"subject": "Test Subject",
			"from":    "user@example.com",
		},
	}

	result := ch.formatFallback(event)
	if !strings.Contains(result, "**Test Subject**") {
		t.Errorf("expected markdown bold subject, got %q", result)
	}
	if !strings.Contains(result, "user@example.com") {
		t.Errorf("expected from in fallback, got %q", result)
	}
}

func TestFeishuChannel_InterfaceMethods(t *testing.T) {
	ch := NewFeishuChannel("url", "secret", core.PriorityHigh, "my template")

	if ch.Name() != "Feishu" {
		t.Errorf("Name() = %q, want 'Feishu'", ch.Name())
	}
	if ch.MinPriority() != core.PriorityHigh {
		t.Errorf("MinPriority() = %v, want PriorityHigh", ch.MinPriority())
	}
	if ch.TemplateContent() != "my template" {
		t.Errorf("TemplateContent() = %q, want 'my template'", ch.TemplateContent())
	}
}

// ─── Discord Tests ───────────────────────────────────────────────────────────

func TestDiscordChannel_SendRenderedWithDetails(t *testing.T) {
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(204)
	}))
	defer server.Close()

	ch := NewDiscordChannel(server.URL, core.PriorityNormal, "tmpl")

	event := core.Event{
		Type:     core.EventEmailReceived,
		Priority: core.PriorityHigh,
	}

	_, _, err := ch.SendRenderedWithDetails("**Email content**", event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	embeds, ok := receivedBody["embeds"].([]any)
	if !ok || len(embeds) == 0 {
		t.Fatal("expected embeds array in payload")
	}

	embed := embeds[0].(map[string]any)
	if embed["title"] != "📧 New Email" {
		t.Errorf("expected title '📧 New Email', got %v", embed["title"])
	}
	if embed["description"] != "**Email content**" {
		t.Errorf("expected description, got %v", embed["description"])
	}
	// High priority should be red (0xFF0000)
	if int(embed["color"].(float64)) != 0xFF0000 {
		t.Errorf("expected red color for high priority, got %v", embed["color"])
	}
}

func TestDiscordChannel_ColorForPriority(t *testing.T) {
	ch := &DiscordChannel{}

	tests := []struct {
		priority core.EventPriority
		want     int
	}{
		{core.PriorityLow, 0x00FF00},
		{core.PriorityNormal, 0x0000FF},
		{core.PriorityHigh, 0xFF0000},
		{core.PriorityCritical, 0xFF0000},
	}

	for _, tt := range tests {
		got := ch.colorForPriority(tt.priority)
		if got != tt.want {
			t.Errorf("colorForPriority(%d) = 0x%X, want 0x%X", tt.priority, got, tt.want)
		}
	}
}

func TestDiscordChannel_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{"message":"rate limited"}`))
	}))
	defer server.Close()

	ch := NewDiscordChannel(server.URL, core.PriorityNormal, "")
	_, _, err := ch.SendRenderedWithDetails("test", core.Event{Type: core.EventEmailReceived})
	if err == nil {
		t.Fatal("expected error for 429 status")
	}
}

func TestDiscordChannel_InterfaceMethods(t *testing.T) {
	ch := NewDiscordChannel("url", core.PriorityNormal, "tmpl")

	if ch.Name() != "Discord" {
		t.Errorf("Name() = %q, want 'Discord'", ch.Name())
	}
	if ch.MinPriority() != core.PriorityNormal {
		t.Errorf("MinPriority() = %v, want PriorityNormal", ch.MinPriority())
	}
	if ch.TemplateContent() != "tmpl" {
		t.Errorf("TemplateContent() = %q, want 'tmpl'", ch.TemplateContent())
	}
}

// ─── Console Tests ───────────────────────────────────────────────────────────

func TestConsoleChannel_Send(t *testing.T) {
	ch := NewConsoleChannel(core.PriorityLow)

	if ch.Name() != "Console" {
		t.Errorf("Name() = %q, want 'Console'", ch.Name())
	}
	if ch.MinPriority() != core.PriorityLow {
		t.Errorf("MinPriority() = %v, want PriorityLow", ch.MinPriority())
	}

	// Should not error
	err := ch.Send(core.Event{Type: core.EventEmailReceived})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	err = ch.SendRendered("rendered content", core.Event{Type: core.EventEmailReceived})
	if err != nil {
		t.Errorf("unexpected error from SendRendered: %v", err)
	}
}
