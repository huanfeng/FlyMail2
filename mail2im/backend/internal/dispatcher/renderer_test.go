package dispatcher

import (
	"mail2im/internal/core"
	"strings"
	"testing"
)

func TestRenderTemplate_BasicVariables(t *testing.T) {
	data := SampleTemplateData()
	tmpl := "Subject: {{.Subject}}, From: {{.From}}"
	result := RenderTemplate(tmpl, data, "fallback")

	if !strings.Contains(result, data.Subject) {
		t.Errorf("expected result to contain subject %q, got %q", data.Subject, result)
	}
	if !strings.Contains(result, data.From) {
		t.Errorf("expected result to contain from %q, got %q", data.From, result)
	}
}

func TestRenderTemplate_EmptyTemplate(t *testing.T) {
	data := SampleTemplateData()
	result := RenderTemplate("", data, "fallback text")

	if result != "fallback text" {
		t.Errorf("expected fallback, got %q", result)
	}
}

func TestRenderTemplate_InvalidSyntax(t *testing.T) {
	data := SampleTemplateData()
	result := RenderTemplate("{{.Invalid", data, "fallback")

	if result != "fallback" {
		t.Errorf("expected fallback for invalid template, got %q", result)
	}
}

func TestRenderTemplateHTML_EscapesFields(t *testing.T) {
	data := TemplateData{
		Subject: `<script>alert("xss")</script>`,
		From:    `Test <test@example.com>`,
	}
	tmpl := "Subject: {{.Subject}}"
	result := RenderTemplateHTML(tmpl, data, "fallback")

	if strings.Contains(result, "<script>") {
		t.Errorf("expected HTML escaping, got %q", result)
	}
	if !strings.Contains(result, "&lt;script&gt;") {
		t.Errorf("expected escaped HTML entities, got %q", result)
	}
}

func TestDefaultFallbackMessage(t *testing.T) {
	data := TemplateData{
		EventType: string(core.EventEmailReceived),
		From:      "sender@example.com",
		Subject:   "Test Subject",
	}
	result := DefaultFallbackMessage(data)

	if !strings.Contains(result, "Test Subject") {
		t.Errorf("expected subject in fallback, got %q", result)
	}
	if !strings.Contains(result, "sender@example.com") {
		t.Errorf("expected from in fallback, got %q", result)
	}
}

func TestParseFromField(t *testing.T) {
	tests := []struct {
		input     string
		wantName  string
		wantEmail string
	}{
		{"John Doe <john@example.com>", "John Doe", "john@example.com"},
		{"john@example.com", "john@example.com", "john@example.com"},
		{"  Name  <email@test.com>  ", "Name", "email@test.com"},
	}

	for _, tt := range tests {
		name, email := parseFromField(tt.input)
		if name != tt.wantName {
			t.Errorf("parseFromField(%q) name = %q, want %q", tt.input, name, tt.wantName)
		}
		if email != tt.wantEmail {
			t.Errorf("parseFromField(%q) email = %q, want %q", tt.input, email, tt.wantEmail)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate short string: got %q", got)
	}
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("truncate long string: got %q", got)
	}
}

func TestPriorityLabel(t *testing.T) {
	tests := []struct {
		input core.EventPriority
		want  string
	}{
		{core.PriorityLow, "Low"},
		{core.PriorityNormal, "Normal"},
		{core.PriorityHigh, "High"},
		{core.PriorityCritical, "Critical"},
	}

	for _, tt := range tests {
		if got := priorityLabel(tt.input); got != tt.want {
			t.Errorf("priorityLabel(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetTemplateVariables(t *testing.T) {
	vars := GetTemplateVariables()
	if len(vars) == 0 {
		t.Fatal("expected non-empty variables list")
	}

	// Check that Subject variable exists
	found := false
	for _, v := range vars {
		if v.Name == "Subject" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Subject' in template variables")
	}
}

// ─── New tests for BodyContent and VerificationCode ──────────────────────────

func TestExtractVerificationCode(t *testing.T) {
	tests := []struct {
		name     string
		subject  string
		body     string
		wantCode string
		wantOK   bool
	}{
		{
			name:     "Chinese verification code",
			subject:  "验证码通知",
			body:     "您的验证码为：385724，请在5分钟内使用。",
			wantCode: "385724",
			wantOK:   true,
		},
		{
			name:     "English verification code",
			subject:  "Your verification code",
			body:     "Your verification code is 123456. It expires in 10 minutes.",
			wantCode: "123456",
			wantOK:   true,
		},
		{
			name:     "OTP code",
			subject:  "OTP",
			body:     "Your OTP: AB1234",
			wantCode: "AB1234",
			wantOK:   true,
		},
		{
			name:     "Code is pattern",
			subject:  "Login alert",
			body:     "Your code is 987654",
			wantCode: "987654",
			wantOK:   true,
		},
		{
			name:     "PIN code",
			subject:  "PIN Reset",
			body:     "Your new PIN: 8842",
			wantCode: "8842",
			wantOK:   true,
		},
		{
			name:     "Code in subject",
			subject:  "验证码: 556677",
			body:     "Please use this code to login.",
			wantCode: "556677",
			wantOK:   true,
		},
		{
			name:     "No code present",
			subject:  "Welcome to our service",
			body:     "Thank you for signing up! No code here.",
			wantCode: "",
			wantOK:   false,
		},
		{
			name:     "Security code",
			subject:  "Security alert",
			body:     "Your security code: ABCD1234",
			wantCode: "ABCD1234",
			wantOK:   true,
		},
		{
			name:     "One-time password",
			subject:  "Login",
			body:     "Your one-time password is 445566",
			wantCode: "445566",
			wantOK:   true,
		},
		{
			name:     "Chinese confirmation code",
			subject:  "确认码",
			body:     "您的确认码是 7788",
			wantCode: "7788",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, ok := extractVerificationCode(tt.subject, tt.body)
			if ok != tt.wantOK {
				t.Errorf("extractVerificationCode() ok = %v, want %v", ok, tt.wantOK)
			}
			if code != tt.wantCode {
				t.Errorf("extractVerificationCode() code = %q, want %q", code, tt.wantCode)
			}
		})
	}
}

func TestRenderTemplate_BodyContent(t *testing.T) {
	data := TemplateData{
		Subject:     "Test",
		BodyPreview: "short preview",
		BodyContent: "This is a much longer body content that includes full email text.",
	}

	tmpl := "Preview: {{.BodyPreview}}\nFull: {{.BodyContent}}"
	result := RenderTemplate(tmpl, data, "fallback")

	if !strings.Contains(result, "short preview") {
		t.Errorf("expected BodyPreview in result, got %q", result)
	}
	if !strings.Contains(result, "much longer body content") {
		t.Errorf("expected BodyContent in result, got %q", result)
	}
}

func TestRenderTemplate_VerificationCodeConditional(t *testing.T) {
	tmpl := `Subject: {{.Subject}}{{if .IsVerificationCode}}
Code: {{.VerificationCode}}{{end}}`

	// With verification code
	data := TemplateData{
		Subject:            "Your code",
		VerificationCode:   "123456",
		IsVerificationCode: true,
	}
	result := RenderTemplate(tmpl, data, "fallback")
	if !strings.Contains(result, "Code: 123456") {
		t.Errorf("expected verification code in result, got %q", result)
	}

	// Without verification code
	data2 := TemplateData{
		Subject:            "Newsletter",
		IsVerificationCode: false,
	}
	result2 := RenderTemplate(tmpl, data2, "fallback")
	if strings.Contains(result2, "Code:") {
		t.Errorf("expected no code block in result, got %q", result2)
	}
}

func TestRenderTemplateHTML_EscapesNewFields(t *testing.T) {
	data := TemplateData{
		BodyContent:      `<script>alert("xss")</script>`,
		VerificationCode: `<b>123</b>`,
	}
	tmpl := "Body: {{.BodyContent}} Code: {{.VerificationCode}}"
	result := RenderTemplateHTML(tmpl, data, "fallback")

	if strings.Contains(result, "<script>") {
		t.Errorf("expected BodyContent to be HTML-escaped, got %q", result)
	}
	if strings.Contains(result, "<b>123") {
		t.Errorf("expected VerificationCode to be HTML-escaped, got %q", result)
	}
}

func TestBodyLimitForChannel(t *testing.T) {
	tests := []struct {
		channelType string
		want        int
	}{
		{"telegram", BodyLimitTelegram},
		{"feishu", BodyLimitFeishu},
		{"discord", BodyLimitDiscord},
		{"console", BodyLimitDefault},
		{"unknown", BodyLimitDefault},
	}

	for _, tt := range tests {
		t.Run(tt.channelType, func(t *testing.T) {
			got := BodyLimitForChannel(tt.channelType)
			if got != tt.want {
				t.Errorf("BodyLimitForChannel(%q) = %d, want %d", tt.channelType, got, tt.want)
			}
		})
	}
}
