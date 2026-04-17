package model

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	UserID       uint           `gorm:"primaryKey;column:id" json:"user_id"`
	Username     string         `gorm:"unique;not null" json:"username"`
	Email        string         `json:"email"`
	Password     string         `gorm:"not null" json:"-"`
	PasswordHash string         `gorm:"column:password" json:"-"`
	IsAdmin      bool           `gorm:"default:false" json:"is_admin"`
	LastLogin    *time.Time     `json:"last_login"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

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
	IsFullSynced      bool           `json:"is_full_synced" gorm:"default:false"`  // Whether initial full sync is completed
	IsSyncing         bool           `json:"is_syncing" gorm:"default:false"`      // Whether currently syncing
	SyncProgress      int            `json:"sync_progress" gorm:"default:0"`       // Sync progress (0-100)
	SyncError         string         `json:"sync_error"`                           // Last sync error message
	InitialSyncOption string         `json:"initial_sync_option"`                  // Initial sync option: none/recent_days/recent_count/full
	InitialSyncDays   int            `json:"initial_sync_days" gorm:"default:7"`   // Sync recent N days of emails
	InitialSyncCount  int            `json:"initial_sync_count" gorm:"default:50"` // Sync recent N emails per folder
	LastSyncError     *time.Time     `json:"last_sync_error"`                      // Last sync error timestamp
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`

	User   User    `gorm:"foreignKey:UserID" json:"-"`
	Emails []Email `gorm:"foreignKey:AccountID" json:"-"`
}

type Email struct {
	EmailID    uint           `gorm:"primaryKey;column:id" json:"email_id"`
	AccountID  uint           `gorm:"not null;index:idx_account_folder_uid,unique" json:"account_id"`
	MessageID  string         `gorm:"not null;column:email_id" json:"message_id"`
	UID        uint32         `json:"uid" gorm:"index:idx_account_folder_uid,unique"` // IMAP UID
	Subject    string         `json:"subject"`
	From       string         `json:"from"`
	To         string         `json:"to"`
	CC         string         `json:"cc"`
	BCC        string         `json:"bcc"`
	Body       string         `json:"body"`
	BodyHTML   string         `json:"body_html"`
	IsRead     bool           `gorm:"default:false" json:"is_read"`
	IsStarred  bool           `gorm:"default:false" json:"is_starred"`
	Date       time.Time      `json:"date"`
	Size       int64          `json:"size"`
	FolderName string         `json:"folder_name" gorm:"index:idx_account_folder_uid,unique"` // Decoded folder name (UTF-8)
	FolderID   *uint          `json:"folder_id" gorm:"index"`                                 // Optional folder reference
	FolderType FolderType     `json:"folder_type" gorm:"default:0;index"`                     // Cached folder type for performance
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`

	Account     EmailAccount `gorm:"foreignKey:AccountID" json:"-"`
	Folder      *Folder      `gorm:"foreignKey:FolderID" json:"-"`
	Attachments []Attachment `gorm:"foreignKey:EmailID" json:"-"`
}

type Attachment struct {
	AttachmentID uint           `gorm:"primaryKey;column:id" json:"attachment_id"`
	EmailID      uint           `gorm:"not null" json:"email_id"`
	Filename     string         `gorm:"not null" json:"filename"`
	ContentType  string         `json:"content_type"`
	Size         int64          `json:"size"`
	Data         []byte         `json:"-"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	Email Email `gorm:"foreignKey:EmailID" json:"-"`
}

type Folder struct {
	FolderID      uint           `gorm:"primaryKey;column:id" json:"folder_id"`
	AccountID     uint           `gorm:"not null" json:"account_id"`
	Name          string         `gorm:"not null" json:"name"`        // Decoded folder name (UTF-8)
	RawName       string         `gorm:"not null" json:"raw_name"`    // Original folder name (UTF-7)
	Type          FolderType     `json:"type" gorm:"default:0"`       // Folder type enum
	Delimiter     string         `json:"delimiter"`                   // Folder hierarchy delimiter (e.g., "/", ".")
	ParentName    string         `json:"parent_name"`                 // Parent folder name
	Flags         string         `json:"flags"`                       // IMAP folder flags
	EmailCount    int64          `json:"email_count"`                 // Total email count in folder
	UnreadCount   int64          `json:"unread_count"`                // Unread email count
	UIDValidity   uint32         `json:"uid_validity"`                // IMAP UIDVALIDITY value
	UIDNext       uint32         `json:"uid_next"`                    // Next expected UID
	LastSyncedUID uint32         `json:"last_synced_uid"`             // Last synced UID
	LastSyncAt    *time.Time     `json:"last_sync_at"`                // Last sync timestamp
	SortOrder     int            `json:"sort_order" gorm:"default:0"` // Sort order for display
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

	Account EmailAccount `gorm:"foreignKey:AccountID" json:"-"`
}

type Setting struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Key       string         `gorm:"unique;not null" json:"key"`
	Value     string         `json:"value"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
