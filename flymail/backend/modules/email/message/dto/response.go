package dto

import (
	"time"
)

// EmailListResponse represents an email in list view
type EmailListResponse struct {
	EmailID       uint      `json:"email_id"`
	UID           string    `json:"uid"`
	MessageID     string    `json:"message_id"`
	AccountID     uint      `json:"account_id"`
	Subject       string    `json:"subject"`
	From          string    `json:"from"`
	To            string    `json:"to"`
	CC            string    `json:"cc"`
	BCC           string    `json:"bcc"`
	Date          time.Time `json:"date"`
	IsRead        bool      `json:"is_read"`
	IsStarred     bool      `json:"is_starred"`
	HasAttachment bool      `json:"has_attachment"`
	Size          int64     `json:"size"`
	Preview       string    `json:"preview"`
	FolderName    string    `json:"folder_name"`
	FolderType    int       `json:"folder_type"`
	InternalDate  time.Time `json:"internal_date"`
	CreatedAt     time.Time `json:"created_at"`
}

// EmailDetailResponse represents an email in detail view
type EmailDetailResponse struct {
	EmailListResponse
	TextBody    string                `json:"text_body"`
	HTMLBody    string                `json:"html_body"`
	Attachments []*AttachmentResponse `json:"attachments,omitempty"`
	Headers     map[string]string     `json:"headers,omitempty"`
	UpdatedAt   time.Time             `json:"updated_at"`
}

// AttachmentResponse represents an email attachment
type AttachmentResponse struct {
	AttachmentID uint   `json:"attachment_id"`
	Filename     string `json:"filename"`
	Size         int64  `json:"size"`
	ContentType  string `json:"content_type"`
	ContentID    string `json:"content_id,omitempty"`
	IsInline     bool   `json:"is_inline"`
}

// EmailListResult represents the result of listing emails
type EmailListResult struct {
	Emails      []*EmailListResponse `json:"emails"`
	TotalCount  int64                `json:"total_count"`
	UnreadCount int64                `json:"unread_count,omitempty"`
	HasMore     bool                 `json:"has_more"`
}

// BatchOperationResult represents the result of a batch operation
type BatchOperationResult struct {
	Success []uint   `json:"success"`
	Failed  []uint   `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}
