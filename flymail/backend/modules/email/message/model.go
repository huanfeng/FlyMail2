package message

import "time"

// Message 是一封邮件的元数据（M3 不含正文；正文在 M4 按需抓取另存）。
type Message struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	AccountID uint   `gorm:"index;not null" json:"account_id"`
	FolderID  uint   `gorm:"uniqueIndex:idx_msg_folder_uid;not null" json:"folder_id"`
	UID       uint32 `gorm:"uniqueIndex:idx_msg_folder_uid;not null" json:"uid"`

	MessageID  string `gorm:"index" json:"message_id"`
	InReplyTo  string `json:"in_reply_to"`
	References string `gorm:"column:references_hdr" json:"references"`
	ThreadID   string `gorm:"index" json:"thread_id"`

	Subject  string `json:"subject"`
	FromName string `json:"from_name"`
	FromAddr string `json:"from_addr"`
	ToJSON   string `json:"-"`
	CcJSON   string `json:"-"`

	Date time.Time `gorm:"index" json:"date"`
	Size int64     `json:"size"`

	Seen     bool `gorm:"not null" json:"seen"`
	Flagged  bool `gorm:"not null" json:"flagged"`
	Answered bool `gorm:"not null" json:"answered"`
	Deleted  bool `gorm:"not null" json:"deleted"`

	HasAttachment bool   `gorm:"not null" json:"has_attachment"`
	Snippet       string `json:"snippet"`
	BodySynced    bool   `gorm:"not null" json:"body_synced"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Message) TableName() string { return "messages" }
