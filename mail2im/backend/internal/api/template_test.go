package api

import (
	"bytes"
	"encoding/json"
	"mail2im/internal/core"
	"mail2im/internal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupTemplateRouter(t *testing.T) *gin.Engine {
	t.Helper()
	setupTestRouter(t) // reuse DB + dispatcher setup

	r := gin.New()
	r.GET("/templates", GetTemplates)
	r.POST("/templates", CreateTemplate)
	r.PUT("/templates/:id", UpdateTemplate)
	r.DELETE("/templates/:id", DeleteTemplate)
	r.POST("/templates/preview", PreviewTemplate)
	r.GET("/templates/variables", GetTemplateVariables)
	r.GET("/templates/defaults", GetDefaultTemplates)
	return r
}

func TestGetTemplates_Empty(t *testing.T) {
	r := setupTemplateRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/templates", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTemplate(t *testing.T) {
	r := setupTemplateRouter(t)

	body := map[string]any{
		"name":         "Test Template",
		"content":      "Subject: {{.Subject}}",
		"channel_type": "telegram",
		"is_default":   false,
		"description":  "A test template",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/templates", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]any)
	if data["name"] != "Test Template" {
		t.Errorf("expected name 'Test Template', got %v", data["name"])
	}
}

func TestUpdateTemplate(t *testing.T) {
	r := setupTemplateRouter(t)

	// Create template
	core.DB.Create(&models.NotificationTemplate{
		Name:        "Original",
		Content:     "old content",
		ChannelType: "telegram",
	})

	body := map[string]any{
		"name":         "Updated",
		"content":      "new content {{.From}}",
		"channel_type": "all",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/templates/1", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]any)
	if data["name"] != "Updated" {
		t.Errorf("expected name 'Updated', got %v", data["name"])
	}
}

func TestUpdateTemplate_NotFound(t *testing.T) {
	r := setupTemplateRouter(t)

	body := map[string]any{"name": "x", "content": "y"}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/templates/999", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteTemplate(t *testing.T) {
	r := setupTemplateRouter(t)

	core.DB.Create(&models.NotificationTemplate{
		Name:    "To Delete",
		Content: "content",
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/templates/1", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPreviewTemplate_Telegram(t *testing.T) {
	r := setupTemplateRouter(t)

	body := map[string]any{
		"content":      "<b>{{.Subject}}</b> from {{.From}}",
		"channel_type": "telegram",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/templates/preview", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]any)
	preview := data["preview"].(string)
	// Should contain the rendered subject
	if !strings.Contains(preview, "Your order #12345") {
		t.Errorf("expected rendered subject in preview, got %q", preview)
	}
}

func TestPreviewTemplate_EmptyContent(t *testing.T) {
	r := setupTemplateRouter(t)

	body := map[string]any{
		"content":      "",
		"channel_type": "all",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/templates/preview", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for empty content, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPreviewTemplate_InvalidSyntax(t *testing.T) {
	r := setupTemplateRouter(t)

	body := map[string]any{
		"content":      "{{.Invalid",
		"channel_type": "all",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/templates/preview", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid template, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetTemplateVariables(t *testing.T) {
	r := setupTemplateRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/templates/variables", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]any)
	if len(data) == 0 {
		t.Error("expected non-empty variables list")
	}

	// Check VerificationCode variable is present
	found := false
	for _, v := range data {
		item := v.(map[string]any)
		if item["name"] == "VerificationCode" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'VerificationCode' in template variables")
	}
}

func TestGetDefaultTemplates(t *testing.T) {
	r := setupTemplateRouter(t)

	// seedInitialData was called by SetupTestDB → AutoMigrate
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/templates/defaults", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]any)

	// Should have at least Telegram, Discord, Feishu, and General defaults
	if len(data) < 4 {
		t.Errorf("expected at least 4 default templates, got %d", len(data))
	}

	// Verify Feishu template exists
	feishuFound := false
	for _, item := range data {
		tmpl := item.(map[string]any)
		if tmpl["channel_type"] == "feishu" {
			feishuFound = true
			break
		}
	}
	if !feishuFound {
		t.Error("expected Feishu default template")
	}
}
