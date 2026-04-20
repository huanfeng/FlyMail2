package api

import (
	"bytes"
	"encoding/json"
	"mail2im/internal/core"
	"mail2im/internal/dispatcher"
	"mail2im/internal/models"
	"mail2im/internal/testutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	testutil.SetupTestDB(t)

	// Initialize a minimal dispatcher for ReloadChannels calls
	core.InitEventBus()
	d := dispatcher.NewTestDispatcher()
	dispatcher.Instance = d

	r := gin.New()
	r.GET("/channels", GetChannels)
	r.POST("/channels", CreateChannel)
	r.PUT("/channels/:id", UpdateChannel)
	r.DELETE("/channels/:id", DeleteChannel)
	r.POST("/channels/test", TestChannel)
	return r
}

func TestGetChannels_Empty(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/channels", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var channels []models.Channel
	if err := json.Unmarshal(w.Body.Bytes(), &channels); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(channels) != 0 {
		t.Errorf("expected empty channels, got %d", len(channels))
	}
}

func TestCreateChannel_Telegram(t *testing.T) {
	r := setupTestRouter(t)

	body := map[string]any{
		"name":   "My Telegram",
		"type":   "telegram",
		"status": "enabled",
		"config": `{"token":"bot123","chat_id":"456"}`,
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/channels", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var ch models.Channel
	if err := json.Unmarshal(w.Body.Bytes(), &ch); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if ch.Name != "My Telegram" {
		t.Errorf("expected name 'My Telegram', got %q", ch.Name)
	}
	if ch.Type != "telegram" {
		t.Errorf("expected type 'telegram', got %q", ch.Type)
	}
}

func TestCreateChannel_Feishu(t *testing.T) {
	r := setupTestRouter(t)

	body := map[string]any{
		"name":   "My Feishu",
		"type":   "feishu",
		"status": "enabled",
		"config": `{"webhook_url":"https://open.feishu.cn/open-apis/bot/v2/hook/xxx","sign_secret":"sec"}`,
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/channels", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var ch models.Channel
	json.Unmarshal(w.Body.Bytes(), &ch)
	if ch.Name != "My Feishu" {
		t.Errorf("expected name 'My Feishu', got %q", ch.Name)
	}
	if ch.Type != "feishu" {
		t.Errorf("expected type 'feishu', got %q", ch.Type)
	}
}

func TestUpdateChannel(t *testing.T) {
	r := setupTestRouter(t)

	// Create a channel first
	core.DB.Create(&models.Channel{
		Name:   "Old Name",
		Type:   "telegram",
		Status: "enabled",
		Config: `{"token":"t","chat_id":"c"}`,
	})

	body := map[string]any{
		"name":   "New Name",
		"type":   "telegram",
		"status": "disabled",
		"config": `{"token":"t2","chat_id":"c2"}`,
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/channels/1", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var ch models.Channel
	json.Unmarshal(w.Body.Bytes(), &ch)
	if ch.Name != "New Name" {
		t.Errorf("expected name 'New Name', got %q", ch.Name)
	}
}

func TestDeleteChannel(t *testing.T) {
	r := setupTestRouter(t)

	core.DB.Create(&models.Channel{
		Name:   "To Delete",
		Type:   "telegram",
		Status: "enabled",
		Config: `{"token":"t","chat_id":"c"}`,
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/channels/1", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify deletion
	var count int64
	core.DB.Model(&models.Channel{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 channels after delete, got %d", count)
	}
}

func TestDeleteChannel_NotFound(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/channels/999", nil)
	r.ServeHTTP(w, req)

	// Should still return 204 (GORM soft delete doesn't error on missing)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTestChannel_Telegram(t *testing.T) {
	r := setupTestRouter(t)

	// Test with invalid config to verify parsing
	body := map[string]any{
		"type":       "telegram",
		"config":     `{"token":"","chat_id":""}`,
		"event_type": "system",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/channels/test", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Empty token/chatID returns 204 (no-op send)
	if w.Code != 200 {
		t.Fatalf("expected 200 for empty config test, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTestChannel_Feishu(t *testing.T) {
	r := setupTestRouter(t)

	body := map[string]any{
		"type":       "feishu",
		"config":     `{"webhook_url":"","sign_secret":""}`,
		"event_type": "system",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/channels/test", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Empty webhook returns 204 (no-op send)
	if w.Code != 200 {
		t.Fatalf("expected 200 for empty feishu config test, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTestChannel_UnsupportedType(t *testing.T) {
	r := setupTestRouter(t)

	body := map[string]any{
		"type":       "unknown_type",
		"config":     `{}`,
		"event_type": "system",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/channels/test", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for unsupported type, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTestChannel_InvalidConfig(t *testing.T) {
	r := setupTestRouter(t)

	body := map[string]any{
		"type":       "telegram",
		"config":     `not valid json`,
		"event_type": "system",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/channels/test", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid config, got %d: %s", w.Code, w.Body.String())
	}
}
