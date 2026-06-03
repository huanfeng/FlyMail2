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
}

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
