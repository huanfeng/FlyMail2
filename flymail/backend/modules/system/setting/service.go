package setting

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// Service interface for setting operations
type Service interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
	UpdateSetting(ctx context.Context, key, value string) error
	DeleteSetting(ctx context.Context, key string) error
	GetAllSettings(ctx context.Context) (map[string]string, error)
	UpdateSettings(ctx context.Context, settings map[string]string) error
	GetAppSettings(ctx context.Context) (*AppSettings, error)
	UpdateAppSettings(ctx context.Context, settings *AppSettings) error
	GetEmailMonitorSettings(ctx context.Context) (*EmailMonitorSettings, error)
	UpdateEmailMonitorSettings(ctx context.Context, settings *EmailMonitorSettings) error
	ExportSettings(ctx context.Context) (string, error)
	ImportSettings(ctx context.Context, jsonData string) error
}

// service implements Service interface
type service struct {
	repo Repository
}

// NewService creates a new setting service
func NewService(repo Repository) Service {
	return &service{
		repo: repo,
	}
}

// GetSetting retrieves a setting by key
func (s *service) GetSetting(ctx context.Context, key string) (string, error) {
	setting, err := s.repo.Get(ctx, key)
	if err != nil {
		return "", err
	}

	if setting == nil {
		return "", nil // Return empty string for non-existent settings
	}

	return setting.Value, nil
}

// GetAllSettings retrieves all settings
func (s *service) GetAllSettings(ctx context.Context) (map[string]string, error) {
	settings, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, setting := range settings {
		result[setting.Key] = setting.Value
	}

	return result, nil
}

// SetSetting sets a setting value
func (s *service) SetSetting(ctx context.Context, key, value string) error {
	// Validate setting key
	if !isValidSettingKey(key) {
		return fmt.Errorf("invalid setting key: %s", key)
	}

	return s.repo.Set(ctx, key, value)
}

// DeleteSetting deletes a setting
func (s *service) DeleteSetting(ctx context.Context, key string) error {
	return s.repo.Delete(ctx, key)
}

// GetSettings retrieves multiple settings by keys
func (s *service) GetSettings(ctx context.Context, keys []string) (map[string]string, error) {
	settings, err := s.repo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, setting := range settings {
		result[setting.Key] = setting.Value
	}

	// Include keys that don't exist with empty values
	for _, key := range keys {
		if _, exists := result[key]; !exists {
			result[key] = ""
		}
	}

	return result, nil
}

// UpdateSettings updates multiple settings
func (s *service) UpdateSettings(ctx context.Context, settings map[string]string) error {
	// Validate all keys first
	for key := range settings {
		if !isValidSettingKey(key) {
			return fmt.Errorf("invalid setting key: %s", key)
		}
	}

	return s.repo.SetMultiple(ctx, settings)
}

// GetAppSettings retrieves common application settings
func (s *service) GetAppSettings(ctx context.Context) (*AppSettings, error) {
	// Define keys for app settings
	keys := []string{
		"email_sync_interval",
		"email_sync_enabled",
		"max_emails_per_sync",
		"delete_after_days",
		"enable_monitor",
		"enable_debug_log",
		"session_timeout",
		"max_upload_size",
		"theme",
		"language",
		"notify_language",
		"timezone",
		"date_format",
		"enable_two_factor",
		"password_min_length",
		"password_require_upper",
		"password_require_lower",
		"password_require_number",
		"password_require_special",
	}

	settings, err := s.GetSettings(ctx, keys)
	if err != nil {
		return nil, err
	}

	// Parse settings with defaults
	appSettings := &AppSettings{
		EmailSettings: EmailSettings{
			MaxEmailSize:     10 * 1024 * 1024, // 10MB
			DefaultSyncLimit: 100,
			SyncInterval:     "30m",
		},
		SecuritySettings: SecuritySettings{
			PasswordMinLength: 8,
			RequireUppercase:  true,
			RequireNumbers:    true,
			RequireSpecial:    false,
		},
		NotificationSettings: NotificationSettings{
			EmailNotifications: true,
			PushNotifications:  false,
		},
		LanguageSettings: LanguageSettings{
			Language:       "auto,en-US", // 默认自动检测，备选英文
			NotifyLanguage: "auto",       // 默认使用主语言设置
		},
	}

	// Override with actual settings
	if val := settings["email_sync_interval"]; val != "" {
		appSettings.EmailSettings.SyncInterval = val
	}
	if val := settings["max_email_size"]; val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			appSettings.EmailSettings.MaxEmailSize = i
		}
	}

	// Security settings
	if val := settings["password_min_length"]; val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			appSettings.SecuritySettings.PasswordMinLength = i
		}
	}
	if val := settings["password_require_upper"]; val != "" {
		appSettings.SecuritySettings.RequireUppercase = val == "true"
	}
	if val := settings["password_require_number"]; val != "" {
		appSettings.SecuritySettings.RequireNumbers = val == "true"
	}
	if val := settings["password_require_special"]; val != "" {
		appSettings.SecuritySettings.RequireSpecial = val == "true"
	}

	// Language settings
	if val := settings["language"]; val != "" {
		appSettings.LanguageSettings.Language = val
	}
	if val := settings["notify_language"]; val != "" {
		appSettings.LanguageSettings.NotifyLanguage = val
	}

	return appSettings, nil
}

// UpdateAppSettings updates common application settings
func (s *service) UpdateAppSettings(ctx context.Context, settings *AppSettings) error {
	// Convert to map - only use fields that exist in AppSettings
	settingsMap := map[string]string{
		"email_sync_interval":      settings.EmailSettings.SyncInterval,
		"max_email_size":           strconv.Itoa(settings.EmailSettings.MaxEmailSize),
		"default_sync_limit":       strconv.Itoa(settings.EmailSettings.DefaultSyncLimit),
		"password_min_length":      strconv.Itoa(settings.SecuritySettings.PasswordMinLength),
		"password_require_upper":   strconv.FormatBool(settings.SecuritySettings.RequireUppercase),
		"password_require_number":  strconv.FormatBool(settings.SecuritySettings.RequireNumbers),
		"password_require_special": strconv.FormatBool(settings.SecuritySettings.RequireSpecial),
		"email_notifications":      strconv.FormatBool(settings.NotificationSettings.EmailNotifications),
		"push_notifications":       strconv.FormatBool(settings.NotificationSettings.PushNotifications),
		"language":                 settings.LanguageSettings.Language,
		"notify_language":          settings.LanguageSettings.NotifyLanguage,
	}

	return s.UpdateSettings(ctx, settingsMap)
}

// isValidSettingKey validates a setting key
func isValidSettingKey(key string) bool {
	// Define allowed setting keys
	allowedKeys := map[string]bool{
		// Email sync settings
		"email_sync_interval": true,
		"email_sync_enabled":  true,
		"max_emails_per_sync": true,
		"delete_after_days":   true,

		// Email monitor settings
		"email_monitor_enabled":        true,
		"email_monitor_enable_idle":    true,
		"email_monitor_day_start":      true,
		"email_monitor_day_end":        true,
		"email_monitor_day_interval":   true,
		"email_monitor_night_interval": true,
		"email_monitor_retry_interval": true,
		"email_monitor_max_retries":    true,

		// System settings
		"enable_monitor":   true,
		"enable_debug_log": true,
		"session_timeout":  true,
		"max_upload_size":  true,

		// UI settings
		"theme":           true,
		"language":        true,
		"notify_language": true,
		"timezone":        true,
		"date_format":     true,

		// Security settings
		"enable_two_factor":        true,
		"password_min_length":      true,
		"password_require_upper":   true,
		"password_require_lower":   true,
		"password_require_number":  true,
		"password_require_special": true,

		// Custom settings (allow any key starting with "custom_")
	}

	if allowedKeys[key] {
		return true
	}

	// Allow custom settings
	if len(key) > 7 && key[:7] == "custom_" {
		return true
	}

	return false
}

// ExportSettings exports settings to JSON
func (s *service) ExportSettings(ctx context.Context) (string, error) {
	settings, err := s.GetAllSettings(ctx)
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// ImportSettings imports settings from JSON
func (s *service) ImportSettings(ctx context.Context, jsonData string) error {
	var settings map[string]string
	if err := json.Unmarshal([]byte(jsonData), &settings); err != nil {
		return err
	}

	return s.UpdateSettings(ctx, settings)
}

// GetEmailMonitorSettings retrieves email monitor settings
func (s *service) GetEmailMonitorSettings(ctx context.Context) (*EmailMonitorSettings, error) {
	keys := []string{
		"email_monitor_enabled",
		"email_monitor_enable_idle",
		"email_monitor_day_start",
		"email_monitor_day_end",
		"email_monitor_day_interval",
		"email_monitor_night_interval",
		"email_monitor_retry_interval",
		"email_monitor_max_retries",
		"email_monitor_check_interval",
		"email_monitor_idle_timeout",
	}

	settings, err := s.GetSettings(ctx, keys)
	if err != nil {
		return nil, err
	}

	// Parse settings with defaults
	monitorSettings := &EmailMonitorSettings{
		Enabled:               true,
		EnableIdleSupport:     true,
		DayTimeStart:          8,
		DayTimeEnd:            22,
		DayTimePollInterval:   "1m",
		NightTimePollInterval: "10m",
		RetryInterval:         "30s",
		MaxRetries:            3,
		CheckInterval:         300,
		IdleTimeout:           "30m",
	}

	// Override with actual settings
	if val := settings["email_monitor_enabled"]; val != "" {
		monitorSettings.Enabled = val == "true"
	}
	if val := settings["email_monitor_enable_idle"]; val != "" {
		monitorSettings.EnableIdleSupport = val == "true"
	}
	if val := settings["email_monitor_day_start"]; val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			monitorSettings.DayTimeStart = i
		}
	}
	if val := settings["email_monitor_day_end"]; val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			monitorSettings.DayTimeEnd = i
		}
	}
	if val := settings["email_monitor_day_interval"]; val != "" {
		monitorSettings.DayTimePollInterval = val
	}
	if val := settings["email_monitor_night_interval"]; val != "" {
		monitorSettings.NightTimePollInterval = val
	}
	if val := settings["email_monitor_retry_interval"]; val != "" {
		monitorSettings.RetryInterval = val
	}
	if val := settings["email_monitor_max_retries"]; val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			monitorSettings.MaxRetries = i
		}
	}
	if val := settings["email_monitor_check_interval"]; val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			monitorSettings.CheckInterval = i
		}
	}
	if val := settings["email_monitor_idle_timeout"]; val != "" {
		monitorSettings.IdleTimeout = val
	}

	return monitorSettings, nil
}

// UpdateEmailMonitorSettings updates email monitor settings
func (s *service) UpdateEmailMonitorSettings(ctx context.Context, settings *EmailMonitorSettings) error {
	// Convert to map
	settingsMap := map[string]string{
		"email_monitor_enabled":        strconv.FormatBool(settings.Enabled),
		"email_monitor_enable_idle":    strconv.FormatBool(settings.EnableIdleSupport),
		"email_monitor_day_start":      strconv.Itoa(settings.DayTimeStart),
		"email_monitor_day_end":        strconv.Itoa(settings.DayTimeEnd),
		"email_monitor_day_interval":   settings.DayTimePollInterval,
		"email_monitor_night_interval": settings.NightTimePollInterval,
		"email_monitor_retry_interval": settings.RetryInterval,
		"email_monitor_max_retries":    strconv.Itoa(settings.MaxRetries),
		"email_monitor_check_interval": strconv.Itoa(settings.CheckInterval),
		"email_monitor_idle_timeout":   settings.IdleTimeout,
	}

	return s.UpdateSettings(ctx, settingsMap)
}

// UpdateSetting is an alias for SetSetting
func (s *service) UpdateSetting(ctx context.Context, key, value string) error {
	return s.SetSetting(ctx, key, value)
}
