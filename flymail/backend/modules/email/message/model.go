package message

import "time"

// Message 是一封邮件的元数据（M3 不含正文；正文在 M4 按需抓取另存）。
type Message struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	AccountID uint   `gorm:"index;index:idx_msg_dedupe,priority:1;not null" json:"account_id"`
	FolderID  uint   `gorm:"uniqueIndex:idx_msg_folder_uid;not null" json:"folder_id"`
	UID       uint32 `gorm:"uniqueIndex:idx_msg_folder_uid;not null" json:"uid"`

	// idx_msg_dedupe (account_id, message_id) 专供 dedupeSameMessage 的相关子查询：
	// 没有它，SQLite 会挑 account_id 单列索引，每一候选行都把该账户全部邮件扫一遍
	// （1.2 万封的库上实测每行 ~3ms，命中 800 封的搜索计数要 4 秒）。
	MessageID  string `gorm:"index;index:idx_msg_dedupe,priority:2" json:"message_id"`
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
