package message

import (
	"time"

	"gorm.io/gorm"
)

// Email represents an email message
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
	FolderType int            `json:"folder_type" gorm:"default:0;index"`                     // Cached folder type for performance
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`

	// Relations - not loaded by default
	Attachments []Attachment `gorm:"foreignKey:EmailID" json:"-"`
}

// Attachment represents an email attachment
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
}

// EmailFilter represents email filtering options
type EmailFilter struct {
	AccountID     uint   `json:"account_id"`
	FolderID      uint   `json:"folder_id"`
	FolderName    string `json:"folder_name"`
	VirtualFolder string `json:"virtual_folder"` // all-inbox, all-starred, all-unread, all-sent, all-drafts, all-trash
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
	Emails []*Email `json:"emails"`
	Total  int64    `json:"total"`
}
