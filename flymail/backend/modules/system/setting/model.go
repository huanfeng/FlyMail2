package setting

import (
	"time"
)

// Setting represents a system setting
type Setting struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"uniqueIndex;not null" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AppSettings represents application settings
type AppSettings struct {
	EmailSettings        EmailSettings        `json:"email_settings"`
	SecuritySettings     SecuritySettings     `json:"security_settings"`
	NotificationSettings NotificationSettings `json:"notification_settings"`
	LanguageSettings     LanguageSettings     `json:"language_settings"`
}

// EmailSettings represents email-related settings
type EmailSettings struct {
	MaxEmailSize     int    `json:"max_email_size"`
	DefaultSyncLimit int    `json:"default_sync_limit"`
	SyncInterval     string `json:"sync_interval"`
}

// SecuritySettings represents security-related settings
type SecuritySettings struct {
	PasswordMinLength int  `json:"password_min_length"`
	RequireUppercase  bool `json:"require_uppercase"`
	RequireNumbers    bool `json:"require_numbers"`
	RequireSpecial    bool `json:"require_special"`
}

// NotificationSettings represents notification-related settings
type NotificationSettings struct {
	EmailNotifications bool `json:"email_notifications"`
	PushNotifications  bool `json:"push_notifications"`
}

// LanguageSettings represents language-related settings
type LanguageSettings struct {
	Language       string `json:"language"`        // 主语言设置：auto、en-US、zh-CN 等
	NotifyLanguage string `json:"notify_language"` // 通知语言设置：auto（使用主语言）、en-US、zh-CN 等
}

// EmailMonitorSettings represents email monitor settings
type EmailMonitorSettings struct {
	Enabled               bool   `json:"enabled"`
	EnableIdleSupport     bool   `json:"enable_idle_support"`
	DayTimeStart          int    `json:"day_time_start"`
	DayTimeEnd            int    `json:"day_time_end"`
	DayTimePollInterval   string `json:"day_time_poll_interval"`
	NightTimePollInterval string `json:"night_time_poll_interval"`
	RetryInterval         string `json:"retry_interval"`
	MaxRetries            int    `json:"max_retries"`
	CheckInterval         int    `json:"check_interval"`
	IdleTimeout           string `json:"idle_timeout"`
}
