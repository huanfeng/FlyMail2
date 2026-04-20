package api

import (
	"bytes"
	"encoding/json"
	"mail2im/internal/core"
	"mail2im/internal/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupPolicyRouter(t *testing.T) *gin.Engine {
	t.Helper()
	setupTestRouter(t)

	r := gin.New()
	r.GET("/notification-policy", GetNotificationPolicy)
	r.PUT("/notification-policy/:key", UpdateNotificationPolicy)
	return r
}

func TestGetNotificationPolicy_WithSeededData(t *testing.T) {
	r := setupPolicyRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/notification-policy", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]any)

	// Should have seeded mail types (primary, bill, notification, etc.)
	if len(data) < 5 {
		t.Errorf("expected at least 5 mail types from seed, got %d", len(data))
	}
}

func TestGetNotificationPolicy_WithChannels(t *testing.T) {
	r := setupPolicyRouter(t)

	// Create enabled channels
	core.DB.Create(&models.Channel{Name: "TG", Type: "telegram", Status: "enabled", Config: `{}`})
	core.DB.Create(&models.Channel{Name: "FS", Type: "feishu", Status: "enabled", Config: `{}`})
	core.DB.Create(&models.Channel{Name: "Disabled", Type: "telegram", Status: "disabled", Config: `{}`})

	// Set primary mail type to route to channel 1
	core.DB.Model(&models.MailType{}).Where("key = ?", "primary").Update("channel_ids", "[1]")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/notification-policy", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]any)

	// Find primary in results
	for _, item := range data {
		mt := item.(map[string]any)
		if mt["key"] != "primary" {
			continue
		}

		channels := mt["channels"].([]any)
		// Should only list enabled channels (2 enabled, 1 disabled)
		if len(channels) != 2 {
			t.Errorf("expected 2 enabled channels for primary, got %d", len(channels))
		}

		// First channel (ID=1) should be selected
		ch1 := channels[0].(map[string]any)
		if ch1["selected"] != true {
			t.Error("expected channel 1 to be selected for primary")
		}
		// Second channel (ID=2) should NOT be selected
		ch2 := channels[1].(map[string]any)
		if ch2["selected"] != false {
			t.Error("expected channel 2 to NOT be selected for primary")
		}
		return
	}
	t.Fatal("primary mail type not found in response")
}

func TestUpdateNotificationPolicy_Action(t *testing.T) {
	r := setupPolicyRouter(t)

	body := map[string]any{
		"action": "silent",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/notification-policy/primary", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify in DB
	var mt models.MailType
	core.DB.Where("key = ?", "primary").First(&mt)
	if mt.Action != "silent" {
		t.Errorf("expected action 'silent', got %q", mt.Action)
	}
}

func TestUpdateNotificationPolicy_ChannelIDs(t *testing.T) {
	r := setupPolicyRouter(t)

	body := map[string]any{
		"channel_ids": "[1,2,3]",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/notification-policy/primary", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var mt models.MailType
	core.DB.Where("key = ?", "primary").First(&mt)
	if mt.ChannelIDs != "[1,2,3]" {
		t.Errorf("expected channel_ids '[1,2,3]', got %q", mt.ChannelIDs)
	}
}

func TestUpdateNotificationPolicy_InvalidAction(t *testing.T) {
	r := setupPolicyRouter(t)

	body := map[string]any{
		"action": "invalid_action",
	}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/notification-policy/primary", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for invalid action, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateNotificationPolicy_NotFound(t *testing.T) {
	r := setupPolicyRouter(t)

	body := map[string]any{"action": "notify"}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/notification-policy/nonexistent", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateNotificationPolicy_NoUpdates(t *testing.T) {
	r := setupPolicyRouter(t)

	body := map[string]any{}
	jsonBody, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/notification-policy/primary", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400 for no updates, got %d: %s", w.Code, w.Body.String())
	}
}

func TestParseChannelIDs(t *testing.T) {
	tests := []struct {
		input string
		want  map[uint]bool
	}{
		{"", map[uint]bool{}},
		{"[]", map[uint]bool{}},
		{"[1,2,3]", map[uint]bool{1: true, 2: true, 3: true}},
		{"invalid", map[uint]bool{}},
	}

	for _, tt := range tests {
		got := parseChannelIDs(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseChannelIDs(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for k, v := range tt.want {
			if got[k] != v {
				t.Errorf("parseChannelIDs(%q)[%d] = %v, want %v", tt.input, k, got[k], v)
			}
		}
	}
}
