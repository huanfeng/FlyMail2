package types

import "time"

// ParsedEmail represents a fully parsed email message at the protocol layer.
// This is the shared data structure produced by IMAP fetch + mail parsing.
// Each product maps this to its own persistence model.
type ParsedEmail struct {
	// Identifiers
	MessageID string `json:"message_id"` // RFC 5322 Message-ID
	UID       uint32 `json:"uid"`        // IMAP UID
	SeqNum    uint32 `json:"seq_num"`    // IMAP sequence number

	// Envelope
	Subject string    `json:"subject"`
	From    []Address `json:"from"`
	To      []Address `json:"to"`
	CC      []Address `json:"cc,omitempty"`
	BCC     []Address `json:"bcc,omitempty"`
	ReplyTo []Address `json:"reply_to,omitempty"`
	Date    time.Time `json:"date"`

	// Body
	TextBody string `json:"text_body,omitempty"`
	HTMLBody string `json:"html_body,omitempty"`

	// Metadata
	Size        int64        `json:"size"`
	Flags       []string     `json:"flags,omitempty"`
	IsRead      bool         `json:"is_read"`
	IsStarred   bool         `json:"is_starred"`
	FolderName  string       `json:"folder_name"`
	FolderPath  string       `json:"folder_path"` // raw IMAP path (may differ from FolderName for UTF-7 encoded names)
	FolderType  string       `json:"folder_type"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Attachment represents email attachment metadata.
// Actual content storage is handled by each product.
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	ContentID   string `json:"content_id,omitempty"` // for inline attachments (CID)
	IsInline    bool   `json:"is_inline"`
	Content     []byte `json:"-"` // 附件内容字节，仅 ExtractAttachments 填充；不序列化、不入库
}

// FromString returns the first From address as a formatted string, or empty.
func (e *ParsedEmail) FromString() string {
	if len(e.From) > 0 {
		return e.From[0].String()
	}
	return ""
}

// ToString returns all To addresses as a formatted string.
func (e *ParsedEmail) ToString() string {
	return FormatAddressList(e.To)
}
