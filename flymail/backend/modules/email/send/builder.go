package send

import (
	"encoding/base64"
	"fmt"
	"mime"
	"net/mail"
	"strings"
	"time"
)

// SendRequest 发送邮件请求。
type SendRequest struct {
	AccountID  uint     `json:"account_id"`
	To         []string `json:"to"`
	Cc         []string `json:"cc,omitempty"`
	Bcc        []string `json:"bcc,omitempty"`
	Subject    string   `json:"subject"`
	BodyHTML   string   `json:"body_html"`
	InReplyTo  string   `json:"in_reply_to,omitempty"`
	References string   `json:"references,omitempty"`
}

// BuildRFC5322 构建合规的 RFC 5322 邮件原始字节。
// Bcc 收件人不写入头部；BodyHTML 以 base64（每 76 字符一行）编码。
func BuildRFC5322(from string, req SendRequest, messageID string, date time.Time) ([]byte, error) {
	var sb strings.Builder

	// From
	fromAddr := (&mail.Address{Address: from}).String()
	fmt.Fprintf(&sb, "From: %s\r\n", fromAddr)

	// To
	toAddrs := encodeAddressList(req.To)
	fmt.Fprintf(&sb, "To: %s\r\n", toAddrs)

	// Cc（可选）
	if len(req.Cc) > 0 {
		fmt.Fprintf(&sb, "Cc: %s\r\n", encodeAddressList(req.Cc))
	}

	// Subject（B-encoding，支持中文）
	encodedSubject := mime.BEncoding.Encode("UTF-8", req.Subject)
	fmt.Fprintf(&sb, "Subject: %s\r\n", encodedSubject)

	// Date
	fmt.Fprintf(&sb, "Date: %s\r\n", date.Format("Mon, 02 Jan 2006 15:04:05 -0700"))

	// Message-ID
	fmt.Fprintf(&sb, "Message-ID: <%s>\r\n", messageID)

	// In-Reply-To（仅当非空）
	if req.InReplyTo != "" {
		val := strings.TrimSpace(req.InReplyTo)
		val = strings.TrimPrefix(val, "<")
		val = strings.TrimSuffix(val, ">")
		fmt.Fprintf(&sb, "In-Reply-To: <%s>\r\n", val)
	}

	// References（仅当非空）
	if req.References != "" {
		fmt.Fprintf(&sb, "References: %s\r\n", req.References)
	}

	// MIME 头
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	sb.WriteString("Content-Transfer-Encoding: base64\r\n")

	// 空行分隔头与正文
	sb.WriteString("\r\n")

	// BodyHTML base64，每 76 字符插入 CRLF
	encoded := base64.StdEncoding.EncodeToString([]byte(req.BodyHTML))
	for len(encoded) > 76 {
		sb.WriteString(encoded[:76])
		sb.WriteString("\r\n")
		encoded = encoded[76:]
	}
	if len(encoded) > 0 {
		sb.WriteString(encoded)
		sb.WriteString("\r\n")
	}

	return []byte(sb.String()), nil
}

// encodeAddressList 将地址列表编码为逗号分隔的 RFC 5322 地址字符串。
func encodeAddressList(addrs []string) string {
	parts := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		parts = append(parts, (&mail.Address{Address: addr}).String())
	}
	return strings.Join(parts, ", ")
}
