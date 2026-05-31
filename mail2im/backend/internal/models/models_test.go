package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	return db
}

func TestAutoMigrate_CreatesAllTables(t *testing.T) {
	db := setupTestDB(t)

	tables := []string{
		"system_settings", "proxies", "accounts", "forward_logs",
		"mailboxes", "emails", "channels", "one_time_tokens",
		"users", "refresh_tokens", "mail_types", "folder_rules",
		"notification_templates",
	}

	for _, table := range tables {
		if !db.Migrator().HasTable(table) {
			t.Errorf("expected table %q to exist", table)
		}
	}
}

func TestSeedInitialData_MailTypes(t *testing.T) {
	db := setupTestDB(t)

	var mailTypes []MailType
	db.Find(&mailTypes)

	if len(mailTypes) < 10 {
		t.Errorf("expected at least 10 seeded mail types, got %d", len(mailTypes))
	}

	// Verify key mail types exist
	requiredKeys := []string{"primary", "bill", "notification", "promotion", "social", "spam"}
	for _, key := range requiredKeys {
		found := false
		for _, mt := range mailTypes {
			if mt.Key == key {
				found = true
				if mt.Action == "" {
					t.Errorf("mail type %q should have a non-empty action", key)
				}
				break
			}
		}
		if !found {
			t.Errorf("expected mail type %q to be seeded", key)
		}
	}
}

func TestSeedInitialData_DefaultTemplates(t *testing.T) {
	db := setupTestDB(t)

	var templates []NotificationTemplate
	db.Where("is_default = ?", true).Find(&templates)

	if len(templates) < 4 {
		t.Errorf("expected at least 4 default templates, got %d", len(templates))
	}

	// Verify each channel type has a default template
	expectedTypes := map[string]bool{
		"telegram": false,
		"discord":  false,
		"feishu":   false,
		"all":      false,
	}

	for _, tmpl := range templates {
		if _, ok := expectedTypes[tmpl.ChannelType]; ok {
			expectedTypes[tmpl.ChannelType] = true
		}
		if tmpl.Content == "" {
			t.Errorf("default template %q has empty content", tmpl.Name)
		}
	}

	for chType, found := range expectedTypes {
		if !found {
			t.Errorf("expected default template for channel type %q", chType)
		}
	}
}

func TestSeedInitialData_FolderRules(t *testing.T) {
	db := setupTestDB(t)

	var rules []FolderRule
	db.Find(&rules)

	if len(rules) < 5 {
		t.Errorf("expected at least 5 folder rules, got %d", len(rules))
	}

	// Verify key rules exist
	requiredNames := []string{"Spam", "Trash", "Inbox", "Sent"}
	for _, name := range requiredNames {
		found := false
		for _, r := range rules {
			if r.Name == name {
				found = true
				if r.Pattern == "" {
					t.Errorf("folder rule %q should have a non-empty pattern", name)
				}
				break
			}
		}
		if !found {
			t.Errorf("expected folder rule %q to be seeded", name)
		}
	}
}

func TestSeedInitialData_Idempotent(t *testing.T) {
	db := setupTestDB(t)

	// Run seed again — should not create duplicates
	if err := seedInitialData(db); err != nil {
		t.Fatalf("second seedInitialData call failed: %v", err)
	}

	var count int64
	db.Model(&MailType{}).Count(&count)

	// Run seed a third time
	if err := seedInitialData(db); err != nil {
		t.Fatalf("third seedInitialData call failed: %v", err)
	}

	var countAfter int64
	db.Model(&MailType{}).Count(&countAfter)

	if count != countAfter {
		t.Errorf("seedInitialData is not idempotent: count changed from %d to %d", count, countAfter)
	}
}

func TestSeedInitialData_TemplatesContainVerificationCode(t *testing.T) {
	db := setupTestDB(t)

	var templates []NotificationTemplate
	db.Where("is_default = ?", true).Find(&templates)

	for _, tmpl := range templates {
		if tmpl.ChannelType == "all" || tmpl.ChannelType == "telegram" || tmpl.ChannelType == "discord" || tmpl.ChannelType == "feishu" {
			if !containsString(tmpl.Content, "IsVerificationCode") {
				t.Errorf("default template %q (%s) should contain IsVerificationCode conditional", tmpl.Name, tmpl.ChannelType)
			}
			if !containsString(tmpl.Content, "VerificationCode") {
				t.Errorf("default template %q (%s) should contain VerificationCode variable", tmpl.Name, tmpl.ChannelType)
			}
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
