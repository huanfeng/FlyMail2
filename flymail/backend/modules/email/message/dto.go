package message

import (
	"encoding/json"

	"flymail-core/types"
)

// MessageListItem 是列表行的对外表示（无正文）。
type MessageListItem struct {
	ID            uint            `json:"id"`
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

func toListItem(m *Message) MessageListItem {
	var to []types.Address
	if m.ToJSON != "" {
		_ = json.Unmarshal([]byte(m.ToJSON), &to)
	}
	return MessageListItem{
		ID: m.ID, UID: m.UID, Subject: m.Subject, FromName: m.FromName, FromAddr: m.FromAddr,
		To: to, Date: m.Date.Format("2006-01-02T15:04:05Z07:00"), Size: m.Size,
		Seen: m.Seen, Flagged: m.Flagged, HasAttachment: m.HasAttachment, Snippet: m.Snippet,
	}
}
