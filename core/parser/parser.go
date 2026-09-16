package parser

import (
	"io"
	"strings"

	message "github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"

	"flymail-core/types"
)

func init() {
	RegisterCharsets()
}

// ParseBody reads an RFC 5322 message body from r and populates
// text/html body fields and attachment metadata on the target ParsedEmail.
//
// Envelope fields (Subject, From, To, etc.) are NOT set here — the caller
// typically gets those from the IMAP ENVELOPE response which is more reliable.
// If fallbackHeaders is true, missing envelope fields will be filled from
// the message headers as a fallback.
func ParseBody(r io.Reader, email *types.ParsedEmail, fallbackHeaders bool) error {
	mr, err := mail.CreateReader(r)
	if err != nil {
		return err
	}

	// Optionally fill envelope fields from headers
	if fallbackHeaders {
		fillFromHeaders(mr, email)
	}
	// 线程头不在 ENVELOPE 里（ENVELOPE 只有 In-Reply-To，没有 References），整封抓取时一律从头里取，
	// 不受 fallbackHeaders 控制。
	fillThreadHeaders(mr, email)

	// 展示路径：不读取内容到 Content（captureContent=false），只取元数据与大小。
	text, html, atts := walkParts(mr, false)
	if email.TextBody == "" {
		email.TextBody = text
	}
	if email.HTMLBody == "" {
		email.HTMLBody = html
	}
	for _, a := range atts {
		email.Attachments = append(email.Attachments, types.Attachment{
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Size:        a.Size,
			ContentID:   a.ContentID,
			IsInline:    a.IsInline,
		})
	}

	return nil
}

// AttachmentData 是解析期的附件载体；与 types.Attachment 不同，它可携带原始内容字节。
// ExtractAttachments 走下载路径时填充 Content；ParseBody 走展示路径时仅填充元数据。
type AttachmentData struct {
	Filename    string
	ContentType string
	ContentID   string
	IsInline    bool
	Size        int64
	Content     []byte
	// Err 记录 capture=true 时读取该附件内容的错误（如解码失败）；调用方据此避免
	// 把截断/空内容当作成功返回。展示路径（capture=false）始终为 nil。
	Err error
}

// ExtractAttachments 解析整封邮件，返回所有附件（含内联部件）及其原始内容字节，顺序与
// ParseBody 产生的 email.Attachments 完全一致。供 M7 附件下载使用。
func ExtractAttachments(r io.Reader) ([]AttachmentData, error) {
	mr, err := mail.CreateReader(r)
	if err != nil {
		return nil, err
	}
	_, _, atts := walkParts(mr, true)
	return atts, nil
}

// walkParts 遍历 MIME 部件，统一供展示路径（ParseBody）与下载路径（ExtractAttachments）使用，
// 以保证两条路径产生的附件顺序一致。
//
// text/plain 与 text/html 内联部件归入正文（text/html）；其余内联部件（如内联图）与
// 普通附件均归入 atts。captureContent 为 true 时读取内容字节到 Content，否则仅丢弃读取以计算大小。
func walkParts(mr *mail.Reader, captureContent bool) (text, html string, atts []AttachmentData) {
	// 连续多少个「不知道会不会前进」的错误之后收尾。取小值即可：
	// 真实邮件里坏部件是零星的，而卡死那类错误第一次就会无限重复。
	const maxConsecutivePartErrs = 8
	consecutiveErrs := 0

	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			// ⚠ 判据是「读取器还会不会前进」，不是「错误严不严重」。
			//
			// 两类错误长得一样，后果天差地别：
			//
			//   会前进  未知字符集 / 未知传输编码——坏的是这一个部件的头，
			//           边界已经吃掉了，下一次调用能拿到后面的部件。跳过是对的。
			//   不前进  multipart 被截断、结束边界缺失——底层
			//           textproto.MultipartReader 每次都返回同一个
			//           `multipart: NextPart: unexpected EOF` 且**不消耗任何输入**，
			//           而 mail.Reader 只在 io.EOF 时才把这个读取器弹出栈，
			//           返回其它错误时栈原样不动。于是 continue 就是死循环。
			//
			// 2026-09-16 线上就是栽在第二类：某封 163 邮件的 multipart 被截断，
			// 同步 goroutine 占满一个核、那个账户从此不再同步；而它既没报错也没断连，
			// 诊断接口还显示 connected=true，外面完全看不出异常。
			//
			// 所以：已知会前进的按原样跳过；其余的允许连续错几次（可能是一串坏部件），
			// 超过就收尾——把「不前进」的情况变成有界的，而不是赌它一定会前进。
			if message.IsUnknownCharset(err) || message.IsUnknownEncoding(err) {
				continue
			}
			consecutiveErrs++
			if consecutiveErrs > maxConsecutivePartErrs {
				break
			}
			continue
		}
		consecutiveErrs = 0

		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			ct, _, _ := h.ContentType()
			// 文本内联部件归入正文
			if strings.HasPrefix(ct, "text/plain") || strings.HasPrefix(ct, "text/html") {
				b, readErr := io.ReadAll(p.Body)
				if readErr != nil {
					continue
				}
				if strings.HasPrefix(ct, "text/plain") && text == "" {
					text = string(b)
				} else if strings.HasPrefix(ct, "text/html") && html == "" {
					html = string(b)
				}
				continue
			}
			// 非文本内联部件（内联图等）作为内联附件。
			// InlineHeader 不提供 Filename()，从其内嵌的 message.Header 自行推导。
			fn := inlineFilename(&h.Header)
			cid := strings.Trim(h.Get("Content-Id"), "<>")
			atts = append(atts, readAttachment(p.Body, fn, ct, cid, true, captureContent))

		case *mail.AttachmentHeader:
			fn, _ := h.Filename()
			ct, _, _ := h.ContentType()
			cid := strings.Trim(h.Get("Content-Id"), "<>")
			atts = append(atts, readAttachment(p.Body, fn, ct, cid, false, captureContent))
		}
	}
	return text, html, atts
}

// inlineFilename 从内联部件的 message.Header 推导文件名：优先取 Content-Disposition 的 filename
// 参数，缺失时回退到 Content-Type 的 name 参数（与 mail.AttachmentHeader.Filename 行为一致）。
func inlineFilename(h *message.Header) string {
	if _, params, err := h.ContentDisposition(); err == nil {
		if fn, ok := params["filename"]; ok && fn != "" {
			return fn
		}
	}
	if _, params, err := h.ContentType(); err == nil {
		if fn, ok := params["name"]; ok {
			return fn
		}
	}
	return ""
}

// readAttachment 从 body 读取附件。capture 为 true 时把内容读入 Content，否则丢弃读取以计算 Size。
func readAttachment(body io.Reader, filename, ct, cid string, inline, capture bool) AttachmentData {
	a := AttachmentData{Filename: filename, ContentType: ct, ContentID: cid, IsInline: inline}
	if capture {
		b, err := io.ReadAll(body)
		a.Content = b
		a.Size = int64(len(b))
		a.Err = err
	} else {
		a.Size, _ = io.Copy(io.Discard, body)
	}
	return a
}

// fillThreadHeaders 从 In-Reply-To / References 头填线程字段（已有值时不覆盖——
// 元数据抓取阶段可能已经通过 HEADER.FIELDS 拿到过）。
func fillThreadHeaders(mr *mail.Reader, email *types.ParsedEmail) {
	if email.InReplyTo == "" {
		if ids := MessageIDs(mr.Header.Get("In-Reply-To")); len(ids) > 0 {
			email.InReplyTo = ids[0]
		}
	}
	if email.References == "" {
		email.References = strings.Join(MessageIDs(mr.Header.Get("References")), " ")
	}
}

// MessageIDs 从 In-Reply-To / References 这类头的原始值里提取 Message-ID 列表（去掉尖括号，保持顺序）。
// 规范写法是 <a@x> <b@y>，但实际邮件里见过：没有尖括号的裸 id、逗号分隔、id 之间夹着注释文本
// （旧版 Outlook 会在 In-Reply-To 里写一段人类可读的引用）。策略：有尖括号就只认尖括号里的；
// 完全没有尖括号才按空白切分，且只保留含 @ 的片段。
func MessageIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if strings.Contains(raw, "<") {
		for {
			start := strings.IndexByte(raw, '<')
			if start < 0 {
				break
			}
			end := strings.IndexByte(raw[start:], '>')
			if end < 0 {
				break
			}
			if id := strings.TrimSpace(raw[start+1 : start+end]); id != "" {
				out = append(out, id)
			}
			raw = raw[start+end+1:]
		}
		return out
	}
	for _, f := range strings.FieldsFunc(raw, func(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == ',' }) {
		if strings.Contains(f, "@") {
			out = append(out, f)
		}
	}
	return out
}

// fillFromHeaders populates ParsedEmail envelope fields from message headers
// only when those fields are still empty.
func fillFromHeaders(mr *mail.Reader, email *types.ParsedEmail) {
	if email.Subject == "" {
		if subj, err := mr.Header.Text("Subject"); err == nil && subj != "" {
			email.Subject = DecodeMIMEHeader(subj)
		}
	}

	if email.MessageID == "" {
		if mid, err := mr.Header.Text("Message-ID"); err == nil {
			email.MessageID = strings.Trim(mid, "<>")
		}
	}

	if len(email.From) == 0 {
		if addrs, err := mr.Header.AddressList("From"); err == nil {
			for _, a := range addrs {
				email.From = append(email.From, types.Address{
					Name:  DecodeMIMEHeader(a.Name),
					Email: a.Address,
				})
			}
		}
	}

	if len(email.To) == 0 {
		if addrs, err := mr.Header.AddressList("To"); err == nil {
			for _, a := range addrs {
				email.To = append(email.To, types.Address{
					Name:  DecodeMIMEHeader(a.Name),
					Email: a.Address,
				})
			}
		}
	}

	if len(email.CC) == 0 {
		if addrs, err := mr.Header.AddressList("Cc"); err == nil {
			for _, a := range addrs {
				email.CC = append(email.CC, types.Address{
					Name:  DecodeMIMEHeader(a.Name),
					Email: a.Address,
				})
			}
		}
	}

	if len(email.BCC) == 0 {
		if addrs, err := mr.Header.AddressList("Bcc"); err == nil {
			for _, a := range addrs {
				email.BCC = append(email.BCC, types.Address{
					Name:  DecodeMIMEHeader(a.Name),
					Email: a.Address,
				})
			}
		}
	}
}
