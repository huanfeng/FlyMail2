package send

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net/mail"
	"strings"
	"time"
)

// Attachment 一个待发送的附件（内容已完整读入内存）。
// ContentID 非空表示这是被正文 <img src="cid:..."> 引用的内联资源，
// 它必须与 HTML 同处一个 multipart/related 容器，否则 Outlook 会当独立附件、图裂。
type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
	ContentID   string
}

// IsInline 判断是否为内联资源。
func (a Attachment) IsInline() bool { return a.ContentID != "" }

// Identity 发信身份：地址 + 显示名（别名发信时两者都来自别名配置）。
type Identity struct {
	Address string
	Name    string
}

// ErrInvalidContentID cid 含非法字符时返回。
var ErrInvalidContentID = errors.New("invalid content id")

// ValidContentID 限定 cid 字符集。Content-ID 是邮件头，cid 里混进 CRLF
// 就是一次头注入——这个校验是安全边界，不是格式洁癖。
func ValidContentID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
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
	// FromAlias 以该账户下的某个别名地址发信；空 = 用账户主地址。
	FromAlias string `json:"from_alias,omitempty"`
	// InlineCIDs 与 multipart 表单里 inline 文件字段按下标一一对应。
	InlineCIDs []string `json:"inline_cids,omitempty"`
}

// BuildRFC5322 构建合规的 RFC 5322 邮件原始字节。Bcc 收件人不写入头部。
//
// 按「有无内联图 × 有无普通附件」取四种结构之一：
//
//	text/html                              都没有（与历史字节保持兼容）
//	multipart/related                      只有内联图
//	multipart/mixed                        只有普通附件
//	multipart/mixed > multipart/related    两者都有
//
// related 的第一个 part 必须是 HTML：省略 start 参数时，接收方按首 part 认定根文档。
func BuildRFC5322(from Identity, req SendRequest, messageID string, date time.Time) ([]byte, error) {
	inline, regular := splitAttachments(req.Attachments)
	for _, att := range inline {
		if !ValidContentID(att.ContentID) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidContentID, att.ContentID)
		}
	}

	var sb strings.Builder

	// ── 公共头部 ────────────────────────────────────────────────────────────────
	fromAddr := (&mail.Address{Name: from.Name, Address: from.Address}).String()
	fmt.Fprintf(&sb, "From: %s\r\n", fromAddr)
	fmt.Fprintf(&sb, "To: %s\r\n", encodeAddressList(req.To))
	if len(req.Cc) > 0 {
		fmt.Fprintf(&sb, "Cc: %s\r\n", encodeAddressList(req.Cc))
	}
	fmt.Fprintf(&sb, "Subject: %s\r\n", mime.BEncoding.Encode("UTF-8", req.Subject))
	fmt.Fprintf(&sb, "Date: %s\r\n", date.Format("Mon, 02 Jan 2006 15:04:05 -0700"))
	fmt.Fprintf(&sb, "Message-ID: <%s>\r\n", messageID)
	if req.InReplyTo != "" {
		if v, ok := cleanMsgIDList(req.InReplyTo, 1); ok {
			fmt.Fprintf(&sb, "In-Reply-To: <%s>\r\n", v)
		}
	}
	if req.References != "" {
		if v, ok := cleanMsgIDList(req.References, 255); ok {
			fmt.Fprintf(&sb, "References: %s\r\n", v)
		}
	}
	sb.WriteString("MIME-Version: 1.0\r\n")

	// ── 都没有：单一 text/html part（保持与历史字节兼容）────────────────────────
	if len(inline) == 0 && len(regular) == 0 {
		writeHTMLPart(&sb, req.BodyHTML)
		return []byte(sb.String()), nil
	}

	// ── 只有内联图：顶层就是 related ────────────────────────────────────────────
	if len(regular) == 0 {
		if err := writeRelated(&sb, req.BodyHTML, inline); err != nil {
			return nil, err
		}
		return []byte(sb.String()), nil
	}

	// ── 有普通附件：顶层 mixed；有内联图则再套一层 related 当首 part ─────────────
	boundary, err := randomBoundary()
	if err != nil {
		return nil, fmt.Errorf("generate boundary: %w", err)
	}
	fmt.Fprintf(&sb, "Content-Type: multipart/mixed; boundary=%q\r\n", boundary)
	sb.WriteString("\r\n")

	fmt.Fprintf(&sb, "--%s\r\n", boundary)
	if len(inline) == 0 {
		writeHTMLPart(&sb, req.BodyHTML)
	} else if err := writeRelated(&sb, req.BodyHTML, inline); err != nil {
		return nil, err
	}

	for _, att := range regular {
		fmt.Fprintf(&sb, "--%s\r\n", boundary)
		writeAttachmentPart(&sb, att)
	}

	fmt.Fprintf(&sb, "--%s--\r\n", boundary)
	return []byte(sb.String()), nil
}

// cleanMsgIDList 从原始字符串里提取并校验 msg-id 列表。
//
// In-Reply-To / References 会原样写进邮件头，混进 CRLF 或空格就能注入新头——
// 与 cid 的字符集校验同一道理。整个值里出现 CR/LF 直接整条丢弃（那必然是注入尝试，
// 不能只剥掉注入段再把剩余部分拼回去，那会改变语义）。合法 id 重新拼装。
// 提取规则：接受带或不带尖括号的 id，逗号/空白分隔。
func cleanMsgIDList(raw string, max int) (string, bool) {
	if strings.ContainsAny(raw, "\r\n") {
		return "", false
	}
	var ids []string
	for _, tok := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ' ' || r == '	' || r == ','
	}) {
		id := tok
		if len(id) >= 2 && id[0] == '<' && id[len(id)-1] == '>' {
			id = id[1 : len(id)-1]
		}
		if !ValidMsgID(id) {
			continue // 非法 id 静默丢弃，不连累其他 id
		}
		ids = append(ids, id)
		if len(ids) >= max {
			break
		}
	}
	if len(ids) == 0 {
		return "", false
	}
	return strings.Join(ids, "> <"), true
}

// ValidMsgID 校验一个 msg-id 的内容（不含尖括号）。
// RFC 5322 的 msg-id 由 atext、点与 @ 组成；除了能注入头的 CRLF/空格，任何其他
// 字符（引号、括号、<、>）都会让严格解析器拒绝或歪曲这封邮件。
func ValidMsgID(s string) bool {
	if s == "" || len(s) > 255 {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at != strings.LastIndexByte(s, '@') || at == len(s)-1 {
		return false
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '!' || r == '#' || r == '$' || r == '%' || r == '&' || r == '*' ||
			r == '+' || r == '-' || r == '/' || r == '=' || r == '?' || r == '^' ||
			r == '_' || r == '`' || r == '{' || r == '|' || r == '}' || r == '~' ||
			r == '.' || r == '@'
		if !ok {
			return false
		}
	}
	return true
}

// splitAttachments 按 ContentID 分出内联资源与普通附件，各自保持原有顺序。
func splitAttachments(atts []Attachment) (inline, regular []Attachment) {
	for _, att := range atts {
		if att.IsInline() {
			inline = append(inline, att)
		} else {
			regular = append(regular, att)
		}
	}
	return inline, regular
}

// writeRelated 写出一个完整的 multipart/related 块：HTML 在前，内联资源在后。
func writeRelated(sb *strings.Builder, html string, inline []Attachment) error {
	boundary, err := randomBoundary()
	if err != nil {
		return fmt.Errorf("generate boundary: %w", err)
	}
	fmt.Fprintf(sb, "Content-Type: multipart/related; type=\"text/html\"; boundary=%q\r\n", boundary)
	sb.WriteString("\r\n")

	fmt.Fprintf(sb, "--%s\r\n", boundary)
	writeHTMLPart(sb, html)

	for _, att := range inline {
		fmt.Fprintf(sb, "--%s\r\n", boundary)
		writeInlinePart(sb, att)
	}

	fmt.Fprintf(sb, "--%s--\r\n", boundary)
	return nil
}

// writeHTMLPart 写正文 part（含头部、空行与 base64 内容）。
func writeHTMLPart(sb *strings.Builder, html string) {
	sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	sb.WriteString("Content-Transfer-Encoding: base64\r\n")
	sb.WriteString("\r\n")
	writeBase64Wrapped(sb, []byte(html))
}

// writeInlinePart 写内联资源 part：Content-ID 供正文以 cid: 引用。
func writeInlinePart(sb *strings.Builder, att Attachment) {
	fmt.Fprintf(sb, "Content-Type: %s; name=%s\r\n", contentTypeOrDefault(att), encodeParamValue(att.Filename))
	sb.WriteString("Content-Transfer-Encoding: base64\r\n")
	fmt.Fprintf(sb, "Content-ID: <%s>\r\n", att.ContentID)
	fmt.Fprintf(sb, "Content-Disposition: inline; %s\r\n", dispositionFilename(att.Filename))
	sb.WriteString("\r\n")
	writeBase64Wrapped(sb, att.Content)
}

// writeAttachmentPart 写普通附件 part。
func writeAttachmentPart(sb *strings.Builder, att Attachment) {
	fmt.Fprintf(sb, "Content-Type: %s; name=%s\r\n", contentTypeOrDefault(att), encodeParamValue(att.Filename))
	sb.WriteString("Content-Transfer-Encoding: base64\r\n")
	fmt.Fprintf(sb, "Content-Disposition: attachment; %s\r\n", dispositionFilename(att.Filename))
	sb.WriteString("\r\n")
	writeBase64Wrapped(sb, att.Content)
}

func contentTypeOrDefault(att Attachment) string {
	if att.ContentType == "" {
		return "application/octet-stream"
	}
	return att.ContentType
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
