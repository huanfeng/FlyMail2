package service

import (
	"context"
	"flymail/shared/store/model"
	"time"
)

// SendEmailRequest represents the request to send an email
type SendEmailRequest struct {
	To          []string `json:"to" binding:"required"`
	Cc          []string `json:"cc"`
	Bcc         []string `json:"bcc"`
	Subject     string   `json:"subject" binding:"required"`
	Body        string   `json:"body" binding:"required"`
	IsHTML      bool     `json:"is_html"`
	ContentType string   `json:"content_type"`
}

// AuthResponse represents the authentication response
type AuthResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// AccountStats represents statistics for an email account
type AccountStats struct {
	TotalEmails  int64 `json:"total_emails"`
	UnreadEmails int64 `json:"unread_emails"`
	TotalFolders int64 `json:"total_folders"`
	StorageUsed  int64 `json:"storage_used"`
}

// EmailFilter represents email filtering options
type EmailFilter struct {
	AccountID     uint   `json:"account_id"`
	FolderID      uint   `json:"folder_id"`
	FolderName    string `json:"folder_name"`
	VirtualFolder string `json:"virtual_folder"` // all-inbox, all-starred, all-unread
	IsRead        *bool  `json:"is_read"`
	IsStarred     *bool  `json:"is_starred"`
	Search        string `json:"search"`
	Page          int    `json:"page"`
	PageSize      int    `json:"page_size"`
	SortBy        string `json:"sort_by"`    // date, subject
	SortOrder     string `json:"sort_order"` // asc, desc
}

// EmailList represents a paginated list of emails
type EmailList struct {
	Emails []*model.Email `json:"emails"`
	Total  int64          `json:"total"`
}

// SyncResult represents the result of an email sync operation
type SyncResult struct {
	AccountID    uint   `json:"account_id"`
	NewEmails    int    `json:"new_emails"`
	TotalEmails  int    `json:"total_emails"`
	UpdatedCount int    `json:"updated_count"`
	DeletedCount int    `json:"deleted_count"`
	Error        string `json:"error,omitempty"`
}

// TestConnectionResult represents the result of a connection test
type TestConnectionResult struct {
	IMAP         bool     `json:"imap"`
	SMTP         bool     `json:"smtp"`
	SupportsIDLE bool     `json:"supports_idle"`
	Capabilities []string `json:"capabilities"`
}

// SystemStatus represents system-level status
type SystemStatus struct {
	Uptime     int64          `json:"uptime"`     // 运行时长（秒）
	Memory     MemoryStatus   `json:"memory"`     // 内存状态
	GC         GCStatus       `json:"gc"`         // GC状态
	Goroutines int            `json:"goroutines"` // Goroutine数量
	Database   DatabaseStatus `json:"database"`   // 数据库状态
}

// MemoryStatus represents memory usage status
type MemoryStatus struct {
	Used         uint64  `json:"used"`          // 已使用内存（字节）
	Total        uint64  `json:"total"`         // 系统分配的总内存（字节）
	HeapAlloc    uint64  `json:"heap_alloc"`    // 堆内存分配（字节）
	HeapInuse    uint64  `json:"heap_inuse"`    // 堆内存使用（字节）
	UsagePercent float64 `json:"usage_percent"` // 内存使用百分比
}

// GCStatus represents garbage collection status
type GCStatus struct {
	NumGC      uint32 `json:"num_gc"`         // GC运行次数
	LastGC     int64  `json:"last_gc"`        // 上次GC时间（Unix时间戳）
	PauseTotal uint64 `json:"pause_total_ns"` // GC总暂停时间（纳秒）
	PauseAvg   uint64 `json:"pause_avg_ns"`   // 平均GC暂停时间（纳秒）
}

// DatabaseStatus represents database connection status
type DatabaseStatus struct {
	Connected       bool `json:"connected"`        // 是否连接
	OpenConnections int  `json:"open_connections"` // 打开的连接数
	InUse           int  `json:"in_use"`           // 正在使用的连接数
	Idle            int  `json:"idle"`             // 空闲连接数
	MaxOpen         int  `json:"max_open"`         // 最大连接数
}

// ServiceHealth represents health status of a service
type ServiceHealth struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message,omitempty"`
}

// HealthStatus represents overall health status
type HealthStatus struct {
	Healthy  bool                     `json:"healthy"`
	Services map[string]ServiceHealth `json:"services"`
	Uptime   int64                    `json:"uptime"` // 运行时长（秒）
}

// RealtimeStatus represents real-time monitoring status
type RealtimeStatus struct {
	ActiveConnections int         `json:"active_connections"` // 活跃连接数
	RunningTasks      int         `json:"running_tasks"`      // 正在运行的任务数
	PendingTasks      int         `json:"pending_tasks"`      // 等待中的任务数
	RecentErrors      []ErrorInfo `json:"recent_errors"`      // 最近的错误信息
	RequestRate       float64     `json:"request_rate"`       // 请求速率（每秒）
}

// ErrorInfo represents error information
type ErrorInfo struct {
	Timestamp int64  `json:"timestamp"`
	Operation string `json:"operation"`
	Error     string `json:"error"`
	Count     int    `json:"count"`
}

// MonitorSummary represents a summary of all monitoring data
type MonitorSummary struct {
	Timestamp time.Time      `json:"timestamp"`
	System    SystemStatus   `json:"system"`
	Health    HealthStatus   `json:"health"`
	Realtime  RealtimeStatus `json:"realtime"`
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

// AuthService defines the interface for authentication operations
type AuthService interface {
	Login(ctx context.Context, username, password string) (*AuthResponse, error)
	RefreshToken(ctx context.Context, refreshToken string) (*AuthResponse, error)
	ValidateToken(ctx context.Context, token string) (uint, error)
	GetUser(ctx context.Context, userID uint) (*model.User, error)
	UpdatePassword(ctx context.Context, userID uint, oldPassword, newPassword string) error
	UpdateAdminCredentials(ctx context.Context, adminUserID uint, newUsername, newEmail, newPassword, oldPassword string) error
}

// AccountService defines the interface for email account operations
type AccountService interface {
	CreateAccount(ctx context.Context, userID uint, account *model.EmailAccount) error
	GetAccount(ctx context.Context, userID uint, accountID uint) (*model.EmailAccount, error)
	GetAccounts(ctx context.Context, userID uint) ([]model.EmailAccount, error)
	UpdateAccount(ctx context.Context, userID uint, accountID uint, updates map[string]interface{}) error
	DeleteAccount(ctx context.Context, userID uint, accountID uint) error
	GetAccountStats(ctx context.Context, userID uint, accountID uint) (*AccountStats, error)
}

// EmailService defines the interface for email operations
type EmailService interface {
	SendEmail(ctx context.Context, accountID uint, req *SendEmailRequest) error
	SyncAccount(ctx context.Context, accountID uint) (*SyncResult, error)
	SyncAccountWithLimit(ctx context.Context, accountID uint, limit int, force bool) (*SyncResult, error)
	SyncAllAccounts(ctx context.Context, userID uint) (map[uint]*SyncResult, error)
	TestConnection(ctx context.Context, account *model.EmailAccount) error
	GetEmails(ctx context.Context, userID uint, filter *EmailFilter) (*EmailList, error)
	GetEmail(ctx context.Context, userID uint, emailID uint) (*model.Email, error)
	UpdateEmailStatus(ctx context.Context, userID uint, emailID uint, updates map[string]interface{}) error
	DeleteEmail(ctx context.Context, userID uint, emailID uint, deleteFromServer bool) error
	TestConnectionAndUpdateCapabilities(ctx context.Context, account *model.EmailAccount) (*TestConnectionResult, error)
}

// SettingService defines the interface for settings operations
type SettingService interface {
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
}

// MonitorService defines the interface for monitoring operations
type MonitorService interface {
	GetSystemStatus(ctx context.Context) (*SystemStatus, error)
	GetHealthStatus(ctx context.Context) (*HealthStatus, error)
	GetRealtimeStatus(ctx context.Context) (*RealtimeStatus, error)
	GetMonitorSummary(ctx context.Context) (*MonitorSummary, error)
}

// Registry defines the service registry interface
type Registry interface {
	GetAuthService() AuthService
	GetAccountService() AccountService
	GetEmailService() EmailService
	GetSettingService() SettingService
	GetMonitorService() MonitorService
}
