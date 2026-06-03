package send

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"net/mail"
	"strings"
	"time"
)

// Attachment 一个待发送的附件（内容已完整读入内存）。
type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// SendRequest 发送邮件请求。
// Attachments 不参与 JSON 反序列化（二进制经 multipart/form-data 上传后由 handler 填充）。
type SendRequest struct {
	AccountID   uint         `json:"account_id"`
	To          []string     `json:"to"`
	Cc          []string     `json:"cc,omitempty"`
	Bcc         []string     `json:"bcc,omitempty"`
	Subject     string       `json:"subject"`
	BodyHTML    string       `json:"body_html"`
	InReplyTo   string       `json:"in_reply_to,omitempty"`
	References  string       `json:"references,omitempty"`
	Attachments []Attachment `json:"-"`
}

// BuildRFC5322 构建合规的 RFC 5322 邮件原始字节。
// Bcc 收件人不写入头部。无附件时为单一 text/html part；有附件时为 multipart/mixed。
func BuildRFC5322(from string, req SendRequest, messageID string, date time.Time) ([]byte, error) {
	var sb strings.Builder

	// ── 公共头部 ────────────────────────────────────────────────────────────────
	fromAddr := (&mail.Address{Address: from}).String()
	fmt.Fprintf(&sb, "From: %s\r\n", fromAddr)
	fmt.Fprintf(&sb, "To: %s\r\n", encodeAddressList(req.To))
	if len(req.Cc) > 0 {
		fmt.Fprintf(&sb, "Cc: %s\r\n", encodeAddressList(req.Cc))
	}
	fmt.Fprintf(&sb, "Subject: %s\r\n", mime.BEncoding.Encode("UTF-8", req.Subject))
	fmt.Fprintf(&sb, "Date: %s\r\n", date.Format("Mon, 02 Jan 2006 15:04:05 -0700"))
	fmt.Fprintf(&sb, "Message-ID: <%s>\r\n", messageID)
	if req.InReplyTo != "" {
		val := strings.TrimSpace(req.InReplyTo)
		val = strings.TrimPrefix(val, "<")
		val = strings.TrimSuffix(val, ">")
		fmt.Fprintf(&sb, "In-Reply-To: <%s>\r\n", val)
	}
	if req.References != "" {
		fmt.Fprintf(&sb, "References: %s\r\n", req.References)
	}
	sb.WriteString("MIME-Version: 1.0\r\n")

	// ── 无附件：单一 text/html part（保持与历史字节兼容）──────────────────────────
	if len(req.Attachments) == 0 {
		sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		sb.WriteString("Content-Transfer-Encoding: base64\r\n")
		sb.WriteString("\r\n")
		writeBase64Wrapped(&sb, []byte(req.BodyHTML))
		return []byte(sb.String()), nil
	}

	// ── 有附件：multipart/mixed ──────────────────────────────────────────────────
	boundary, err := randomBoundary()
	if err != nil {
		return nil, fmt.Errorf("generate boundary: %w", err)
	}
	fmt.Fprintf(&sb, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary)
	sb.WriteString("\r\n")

	// 正文 part
	fmt.Fprintf(&sb, "--%s\r\n", boundary)
	sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	sb.WriteString("Content-Transfer-Encoding: base64\r\n")
	sb.WriteString("\r\n")
	writeBase64Wrapped(&sb, []byte(req.BodyHTML))

	// 附件 part
	for _, att := range req.Attachments {
		ctype := att.ContentType
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		fmt.Fprintf(&sb, "--%s\r\n", boundary)
		fmt.Fprintf(&sb, "Content-Type: %s; name=%s\r\n", ctype, encodeParamValue(att.Filename))
		sb.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&sb, "Content-Disposition: attachment; %s\r\n", dispositionFilename(att.Filename))
		sb.WriteString("\r\n")
		writeBase64Wrapped(&sb, att.Content)
	}

	// 结束边界
	fmt.Fprintf(&sb, "--%s--\r\n", boundary)

	return []byte(sb.String()), nil
}

// writeBase64Wrapped 将 data 以 base64 编码后每 76 字符插入 CRLF 写入 sb。
func writeBase64Wrapped(sb *strings.Builder, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 76 {
		sb.WriteString(encoded[:76])
		sb.WriteString("\r\n")
		encoded = encoded[76:]
	}
	if len(encoded) > 0 {
		sb.WriteString(encoded)
		sb.WriteString("\r\n")
	}
}

// randomBoundary 生成随机的 MIME 边界串。
func randomBoundary() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "flymail_" + hex.EncodeToString(b), nil
}

// encodeParamValue 编码 Content-Type 的 name 参数：ASCII 直接加引号，非 ASCII 用 RFC 2047 编码字。
func encodeParamValue(s string) string {
	if isASCII(s) {
		return "\"" + strings.ReplaceAll(s, "\"", "") + "\""
	}
	return mime.BEncoding.Encode("UTF-8", s)
}

// dispositionFilename 生成 Content-Disposition 的 filename 参数。
// ASCII 用 filename="..."；非 ASCII 额外提供 RFC 5987 的 filename*=UTF-8”... 以最大化客户端兼容。
func dispositionFilename(s string) string {
	if isASCII(s) {
		return fmt.Sprintf("filename=\"%s\"", strings.ReplaceAll(s, "\"", ""))
	}
	return fmt.Sprintf("filename*=UTF-8''%s", pctEncode(s))
}

// isASCII 判断字符串是否仅含可打印 ASCII（含空格）。
func isASCII(s string) bool {
	for _, r := range s {
		if r > 0x7e || r < 0x20 {
			return false
		}
	}
	return true
}

// pctEncode 按 RFC 5987 对非 attr-char 字节做百分号编码。
func pctEncode(s string) string {
	const safe = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!#$&+-.^_`|~"
	var sb strings.Builder
	for _, b := range []byte(s) {
		if strings.IndexByte(safe, b) >= 0 {
			sb.WriteByte(b)
		} else {
			fmt.Fprintf(&sb, "%%%02X", b)
		}
	}
	return sb.String()
}

// encodeAddressList 将地址列表编码为逗号分隔的 RFC 5322 地址字符串。
func encodeAddressList(addrs []string) string {
	parts := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		parts = append(parts, (&mail.Address{Address: addr}).String())
	}
	return strings.Join(parts, ", ")
}
