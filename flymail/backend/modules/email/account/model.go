package account

import (
	"time"

	"gorm.io/gorm"
)

// EmailAccount represents an email account
type EmailAccount struct {
	AccountID           uint       `gorm:"primaryKey;column:id" json:"account_id"`
	UserID              uint       `gorm:"not null" json:"user_id"`
	Name                string     `gorm:"not null" json:"name"`
	Email               string     `gorm:"not null" json:"email"`
	Type                string     `gorm:"not null" json:"type"` // smtp, imap, oauth
	ImapServer          string     `json:"imap_server"`
	ImapPort            int        `json:"imap_port"`
	ImapSSL             bool       `json:"imap_ssl"`
	SmtpServer          string     `json:"smtp_server"`
	SmtpPort            int        `json:"smtp_port"`
	SmtpSSL             bool       `json:"smtp_ssl"`
	Username            string     `json:"username"`
	Password            string     `json:"-"`
	OAuthToken          string     `json:"-"`
	OAuthRefreshToken   string     `json:"-"`
	IsActive            bool       `gorm:"default:true" json:"is_active"`
	LastSync            *time.Time `json:"last_sync"`
	SupportsIDLE        *bool      `json:"supports_idle" gorm:"column:supports_idle"` // Whether IMAP server supports IDLE
	Capabilities        string     `json:"capabilities" gorm:"column:capabilities"`   // Cached IMAP capabilities
	LastCapabilityCheck *time.Time `json:"last_capability_check" gorm:"column:last_capability_check"`
	SortOrder           int        `json:"sort_order" gorm:"default:0"` // Sort order for display
	// Sync status management
	IsFullSynced      bool           `json:"is_full_synced" gorm:"default:false"` // Whether initial full sync is completed
	IsSyncing         bool           `json:"is_syncing" gorm:"default:false"`     // Whether currently syncing
	SyncProgress      int            `json:"sync_progress" gorm:"default:0"`      // Sync progress (0-100)
	SyncError         string         `json:"sync_error"`                          // Last sync error message
	InitialSyncOption string         `json:"initial_sync_option"`                 // Initial sync option: none/recent_days/recent_count/full
	InitialSyncDays   int            `json:"initial_sync_days" gorm:"default:7"`  // Sync recent N days of emails
	InitialSyncCount  int            `json:"initial_sync_count" gorm:"default:0"` // Sync recent N emails
	LastSyncError     *time.Time     `json:"last_sync_error"`                     // Last sync error timestamp
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// AccountStats represents statistics for an email account
type AccountStats struct {
	TotalEmails  int64 `json:"total_emails"`
	UnreadEmails int64 `json:"unread_emails"`
	TotalFolders int64 `json:"total_folders"`
	StorageUsed  int64 `json:"storage_used"`
}

// TestConnectionResult represents the result of a connection test
type TestConnectionResult struct {
	IMAP         bool     `json:"imap"`
	SMTP         bool     `json:"smtp"`
	SupportsIDLE bool     `json:"supports_idle"`
	Capabilities []string `json:"capabilities"`
}
