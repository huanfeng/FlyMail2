package parser

import (
	"bytes"
	"io"
	"strings"
	"unicode/utf8"

	message "github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	"golang.org/x/text/encoding/simplifiedchinese"

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

// ParseHeaders 只从一段 RFC 5322 邮件头填充信封字段与线程头，不碰正文与附件。
//
// ── 为什么有这个函数：不再依赖服务端的 ENVELOPE ─────────────────────────────
//
// 元数据抓取原先要 ENVELOPE，由服务端把邮件头解析成十个字段返给我们。问题是
// **服务端可能解析错**，而且错了没有补救：
//
//	QQ   某些邮件只给九个字段（缺 To/Cc/Bcc 之一时不补 NIL 占位，后面的
//	     地址列表整体左移），go-imap 按 RFC 严格解析，直接报
//	     `expected SP, got ")"`，并且**连带拆掉整条连接**——一封坏邮件
//	     让整个账户的同步再也跑不完。2026-09-16 线上就是这么卡死的。
//	     这不是新问题：MailKit 2018 年就为 QQ 的同一类缺陷加过绕行。
//	GreenMail  ENVELOPE 里根本不带 In-Reply-To（实测）。
//
// ENVELOPE 本来就只是服务端对这些头的一次解析。既然我们自己有解析器，
// 就没有理由把这一步外包给一个可能算错的实现——直接取头自己解析，
// 少一个出错来源，也顺带消掉整类「某某服务商的信封不标准」的兼容问题。
//
// 与 ParseBody 的关系：整封抓取走 ParseBody（头和正文一起解析），
// 元数据抓取走这里。两条路填信封字段用的是同一组函数，不会有两套行为。
func ParseHeaders(r io.Reader, email *types.ParsedEmail) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	// ⚠ 有的服务器返回的头区段不以空行结尾，补一个让解析器正常收尾。
	body := io.MultiReader(bytes.NewReader(decodeRawHeaderBytes(raw)), strings.NewReader("\r\n\r\n"))
	mr, err := mail.CreateReader(body)
	if mr == nil {
		return err
	}
	fillFromHeaders(mr, email)
	fillThreadHeaders(mr, email)
	return nil
}

// decodeRawHeaderBytes 把一段邮件头字节转成合法 UTF-8。
//
// ── 为什么需要它 ────────────────────────────────────────────────────────────
//
// RFC 2047 规定邮件头里的非 ASCII 必须写成 =?charset?B?...?= 的编码字，但**大量
// 旧邮件直接塞原始 8 位字节**。QQ 收件箱里 1579 封有 76 封是这样：
//
//	From: "QQ\xbf\xd5\xbc\xe4\xcf\xee\xc4\xbf\xd7\xe9" <qzone@tencent.com>
//	                └─ GBK 的「空间项目组」，不是编码字
//
// 之前没暴露是因为 ENVELOPE 替我们挡着：QQ 生成信封时会把这些字节转码成 UTF-8
// 再包成编码字。改成自己解析邮件头之后，这层转码就得自己做了——不做的话
// 主题变成乱码字节，**发件人直接整个丢失**（Go 的地址解析器遇到非法 UTF-8
// 会报错，于是一个地址都取不到）。所以必须在解析**之前**整段转码，
// 按字段解码救不回地址。
//
// ── 猜测顺序 ────────────────────────────────────────────────────────────────
//
// 合法 UTF-8 原样放行（现代邮件与全 ASCII 的头都走这条，行为不变）；否则按
// GB18030 解（简体中文邮件的事实标准，且与 GBK/GB2312 向下兼容）；再不行退到
// Latin-1 逐字节映射——它不会失败，至少落库的是合法字符串而不是坏字节。
//
// ⚠ 已知局限：繁体中文的 Big5 原始字节会被当成 GB18030 解出别的汉字。
// 两者在字节层面无法区分，只能按主要用户群选一个。
func decodeRawHeaderBytes(b []byte) []byte {
	if utf8.Valid(b) {
		return b
	}
	if out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(b); err == nil && utf8.Valid(out) {
		return out
	}
	runes := make([]rune, 0, len(b))
	for _, c := range b {
		runes = append(runes, rune(c))
	}
	return []byte(string(runes))
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

// addressList 取一个地址头，严格解析失败时退回宽松提取。
//
// ⚠ 为什么需要宽松那一步：真实邮件里的地址头经常不合 RFC 5322，而 Go 的解析器
// 是全有或全无——一个字符不合语法，**整个头一个地址都取不到**，那一行在列表上
// 就没有任何来源信息。实测撞到的一种：
//
//	From: 458889595@qq.com <n428b3992826@sina.com>
//	      └─ 显示名是个没加引号的邮箱，@ 在 phrase 里是非法字符
//
// 以前这类由服务端的 ENVELOPE 兜着（服务端自己宽松解析过一遍），改成自己解析
// 之后就得自己兜。严格解析成功时一律走严格的，宽松只在它交白卷时才出场。
func addressList(mr *mail.Reader, key string) []types.Address {
	if addrs, err := mr.Header.AddressList(key); err == nil && len(addrs) > 0 {
		out := make([]types.Address, 0, len(addrs))
		for _, a := range addrs {
			out = append(out, types.Address{Name: DecodeMIMEHeader(a.Name), Email: a.Address})
		}
		return out
	}
	return lenientAddresses(mr.Header.Get(key))
}

// lenientAddresses 从一行畸形的地址头里尽量捞出地址。
//
// 只认尖括号里的那种写法（`名字 <a@b>`），因为它是唯一能可靠切分的形式：
// 尖括号内是地址，上一个逗号到尖括号之间是显示名。没有尖括号时按逗号切，
// 只留含 @ 的片段。两条都刻意保守——宁可少认，也不要把一句话当成地址存进去。
func lenientAddresses(raw string) []types.Address {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []types.Address
	if strings.Contains(raw, "<") {
		rest := raw
		for {
			lt := strings.Index(rest, "<")
			if lt < 0 {
				break
			}
			gt := strings.Index(rest[lt:], ">")
			if gt < 0 {
				break
			}
			gt += lt
			addr := strings.TrimSpace(rest[lt+1 : gt])
			name := strings.Trim(strings.TrimSpace(rest[:lt]), `",;`)
			name = strings.TrimSpace(strings.TrimSuffix(name, ","))
			if strings.Contains(addr, "@") {
				out = append(out, types.Address{Name: DecodeMIMEHeader(name), Email: addr})
			}
			rest = rest[gt+1:]
		}
		return out
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "@") && !strings.ContainsAny(part, " \t") {
			out = append(out, types.Address{Email: part})
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
		email.From = addressList(mr, "From")
	}

	if len(email.To) == 0 {
		email.To = addressList(mr, "To")
	}

	if len(email.CC) == 0 {
		email.CC = addressList(mr, "Cc")
	}

	if len(email.BCC) == 0 {
		email.BCC = addressList(mr, "Bcc")
	}

	if len(email.ReplyTo) == 0 {
		email.ReplyTo = addressList(mr, "Reply-To")
	}

	// ⚠ 只在 INTERNALDATE 缺失时才用 Date 头，与原先「ENVELOPE 的日期只作兜底」
	// 一致。两者语义不同：INTERNALDATE 是这封信到达服务器的时间，Date 是发件方
	// 自己写的，可以是任意值（伪造、时钟错乱、草稿沿用旧日期）。按 Date 排序会让
	// 列表被一封声称来自 2030 年的垃圾邮件顶到最上面。
	if email.Date.IsZero() {
		if d, err := mr.Header.Date(); err == nil && !d.IsZero() {
			email.Date = d
		}
	}
}
