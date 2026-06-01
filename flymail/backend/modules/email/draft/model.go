package draft

import "time"

// Draft 本地草稿模型。
type Draft struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	AccountID  uint      `gorm:"index;not null" json:"account_id"`
	ToStr      string    `json:"-"`
	CcStr      string    `json:"-"`
	BccStr     string    `json:"-"`
	Subject    string    `json:"subject"`
	BodyHTML   string    `json:"body_html"`
	InReplyTo  string    `json:"in_reply_to"`
	References string    `json:"references"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName 指定 gorm 表名。
func (Draft) TableName() string { return "drafts" }
