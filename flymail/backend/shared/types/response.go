package dto

import (
	"time"
)

// EmailListResponse represents email data in list responses
type EmailListResponse struct {
	EmailID       uint      `json:"email_id"`
	UID           string    `json:"uid"`
	MessageID     string    `json:"message_id"`
	AccountID     uint      `json:"account_id"`
	Subject       string    `json:"subject"`
	From          string    `json:"from"`
	To            string    `json:"to"`
	CC            string    `json:"cc,omitempty"`
	BCC           string    `json:"bcc,omitempty"`
	Date          time.Time `json:"date"`
	IsRead        bool      `json:"is_read"`
	IsStarred     bool      `json:"is_starred"`
	HasAttachment bool      `json:"has_attachment"`
	FolderName    string    `json:"folder_name"`
	RawFolderName string    `json:"raw_folder_name"`
	Preview       string    `json:"preview,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// EmailDetailResponse represents email data in detail responses
type EmailDetailResponse struct {
	EmailListResponse
	TextBody    string       `json:"text_body,omitempty"`
	HTMLBody    string       `json:"html_body,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Account     *AccountInfo `json:"account,omitempty"`
}

// AccountInfo represents basic account info in responses
type AccountInfo struct {
	AccountID uint   `json:"account_id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Type      string `json:"type"`
	IsActive  bool   `json:"is_active"`
}

// FolderResponse represents folder data in responses
type FolderResponse struct {
	FolderID    uint       `json:"folder_id"`
	AccountID   uint       `json:"account_id"`
	Name        string     `json:"name"`
	RawName     string     `json:"raw_name"`
	Type        int        `json:"type"`
	Delimiter   string     `json:"delimiter,omitempty"`
	ParentName  string     `json:"parent_name,omitempty"`
	Flags       string     `json:"flags,omitempty"`
	EmailCount  int64      `json:"email_count"`
	UnreadCount int64      `json:"unread_count"`
	SortOrder   int        `json:"sort_order"`
	LastSyncAt  *time.Time `json:"last_sync_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// AccountResponse represents account data in responses
type AccountResponse struct {
	AccountID  uint       `json:"account_id"`
	UserID     uint       `json:"user_id"`
	Name       string     `json:"name"`
	Email      string     `json:"email"`
	Type       string     `json:"type"`
	ImapServer string     `json:"imap_server,omitempty"`
	ImapPort   int        `json:"imap_port,omitempty"`
	ImapSSL    bool       `json:"imap_ssl,omitempty"`
	SmtpServer string     `json:"smtp_server,omitempty"`
	SmtpPort   int        `json:"smtp_port,omitempty"`
	SmtpSSL    bool       `json:"smtp_ssl,omitempty"`
	Username   string     `json:"username,omitempty"`
	IsActive   bool       `json:"is_active"`
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Attachment represents attachment info
type Attachment struct {
	AttachmentID uint   `json:"attachment_id"`
	Filename     string `json:"filename"`
	ContentType  string `json:"content_type"`
	Size         int64  `json:"size"`
}

// AccountStatsResponse represents account statistics
type AccountStatsResponse struct {
	AccountID    uint  `json:"account_id"`
	TotalEmails  int64 `json:"total_emails"`
	UnreadEmails int64 `json:"unread_emails"`
	TotalFolders int64 `json:"total_folders"`
}

// UserResponse represents user data in responses
type UserResponse struct {
	UserID    uint       `json:"user_id"`
	Username  string     `json:"username"`
	Email     string     `json:"email"`
	IsAdmin   bool       `json:"is_admin"`
	CreatedAt time.Time  `json:"created_at"`
	LastLogin *time.Time `json:"last_login,omitempty"`
}
