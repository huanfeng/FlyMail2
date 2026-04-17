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
