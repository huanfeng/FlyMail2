package message

import (
	"encoding/json"

	"flymail-core/types"
)

// MessageListItem 是列表行的对外表示（无正文）。
type MessageListItem struct {
	ID            uint            `json:"id"`
	AccountID     uint            `json:"account_id"`
	FolderID      uint            `json:"folder_id"`
	UID           uint32          `json:"uid"`
	Subject       string          `json:"subject"`
	FromName      string          `json:"from_name"`
	FromAddr      string          `json:"from_addr"`
	To            []types.Address `json:"to"`
	Date          string          `json:"date"`
	Size          int64           `json:"size"`
	Seen          bool            `json:"seen"`
	Flagged       bool            `json:"flagged"`
	HasAttachment bool            `json:"has_attachment"`
	Snippet       string          `json:"snippet"`
}

// MessageDetail 邮件详情（含正文与附件）。
type MessageDetail struct {
	MessageListItem
	Cc          []types.Address `json:"cc"`
	TextBody    string          `json:"text_body"`
	HTMLBody    string          `json:"html_body"`
	Attachments []Attachment    `json:"attachments"`
	BodySynced  bool            `json:"body_synced"`
	MessageID   string          `json:"message_id"`
	InReplyTo   string          `json:"in_reply_to"`
	References  string          `json:"references"`
	// ThreadID 让前端在会话视图下按单封 id（通知跳转、深链）定位到所属会话
	ThreadID string `json:"thread_id"`
	// RemoteCount 是净化时数出的远程资源引用个数；RemoteAllowed 表示 HTMLBody 里保留了这些引用
	// （用户要求显示，或发件人在信任名单里）。两者由详情接口在净化后填写。
	RemoteCount   int  `json:"remote_count"`
	RemoteAllowed bool `json:"remote_allowed"`
	// AttachmentToken 是限定这一封、短时效的附件令牌：前端拼 cid 内联图与附件链接时用它，
	// 不把长期 access token 写进邮件 HTML 所在的文档。
	AttachmentToken string `json:"attachment_token"`
}

// Contact 是收件人自动补全的候选项（来自历史往来地址）。
type Contact struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ToListItem 供其它模块（规则试运行）把存储行转成列表项。
func ToListItem(m *Message) MessageListItem { return toListItem(m) }

func toListItem(m *Message) MessageListItem {
	var to []types.Address
	if m.ToJSON != "" {
		_ = json.Unmarshal([]byte(m.ToJSON), &to)
	}
	return MessageListItem{
		ID: m.ID, AccountID: m.AccountID, FolderID: m.FolderID,
		UID: m.UID, Subject: m.Subject, FromName: m.FromName, FromAddr: m.FromAddr,
		To: to, Date: m.Date.Format("2006-01-02T15:04:05Z07:00"), Size: m.Size,
		Seen: m.Seen, Flagged: m.Flagged, HasAttachment: m.HasAttachment, Snippet: m.Snippet,
	}
}
