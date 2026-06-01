package message

import "time"

// MessageBody 单独分表，避免列表查询拉大正文。
type MessageBody struct {
	ID        uint      `gorm:"primaryKey" json:"-"`
	MessageID uint      `gorm:"uniqueIndex;not null" json:"-"`
	TextBody  string    `json:"text_body"`
	HTMLBody  string    `json:"html_body"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

func (MessageBody) TableName() string { return "message_bodies" }

// Attachment 附件元数据（M4 只存元数据，不下载内容）。
type Attachment struct {
	ID          uint   `gorm:"primaryKey" json:"-"`
	MessageID   uint   `gorm:"index;not null" json:"-"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	ContentID   string `json:"content_id,omitempty"`
	IsInline    bool   `json:"is_inline"`
}

func (Attachment) TableName() string { return "attachments" }
