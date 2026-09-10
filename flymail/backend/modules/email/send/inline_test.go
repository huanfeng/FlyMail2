package send_test

import (
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"

	"flymail/modules/email/send"
)

// part 是解析后的一个 MIME 分段，便于断言。
type part struct {
	mediaType   string
	contentID   string
	disposition string
	filename    string
	body        []byte
	children    []part
}

// parseMIME 用标准库真正地解析邮件——断言结构而不是断言字符串，
// 才能证明 Gmail/Outlook 那种严格解析器也认得出来。
func parseMIME(t *testing.T, raw []byte) (*mail.Message, part) {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("解析邮件失败: %v", err)
	}
	return msg, parsePart(t, msg.Header.Get("Content-Type"), msg.Header.Get("Content-Transfer-Encoding"),
		msg.Header.Get("Content-ID"), msg.Header.Get("Content-Disposition"), msg.Body)
}

func parsePart(t *testing.T, ctype, cte, cid, disp string, body io.Reader) part {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(ctype)
	if err != nil {
		t.Fatalf("解析 Content-Type %q 失败: %v", ctype, err)
	}
	p := part{mediaType: mediaType, contentID: strings.Trim(cid, "<>")}
	if disp != "" {
		d, dparams, err := mime.ParseMediaType(disp)
		if err != nil {
			t.Fatalf("解析 Content-Disposition %q 失败: %v", disp, err)
		}
		p.disposition = d
		p.filename = dparams["filename"]
	}

	if !strings.HasPrefix(mediaType, "multipart/") {
		raw, err := io.ReadAll(body)
		if err != nil {
			t.Fatalf("读取 part 失败: %v", err)
		}
		if strings.EqualFold(cte, "base64") {
			decoded, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(raw)), ""))
			if err != nil {
				t.Fatalf("base64 解码失败: %v", err)
			}
			raw = decoded
		}
		p.body = raw
		return p
	}

	mr := multipart.NewReader(body, params["boundary"])
	for {
		sub, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("读取子 part 失败: %v", err)
		}
		p.children = append(p.children, parsePart(t,
			sub.Header.Get("Content-Type"), sub.Header.Get("Content-Transfer-Encoding"),
			sub.Header.Get("Content-ID"), sub.Header.Get("Content-Disposition"), sub))
	}
	return p
}

func baseReq() send.SendRequest {
	return send.SendRequest{
		AccountID: 1,
		To:        []string{"to@example.com"},
		Subject:   "主题",
		BodyHTML:  `<p>正文 <img src="cid:ii_abc123"></p>`,
	}
}

var testIdentity = send.Identity{Address: "sender@example.com"}

// TestBuildInlineOnly 只有内联图：顶层就是 multipart/related，首 part 必须是 HTML。
func TestBuildInlineOnly(t *testing.T) {
	req := baseReq()
	req.Attachments = []send.Attachment{
		{Filename: "a.png", ContentType: "image/png", Content: []byte("PNGDATA"), ContentID: "ii_abc123"},
	}
	raw, err := send.BuildRFC5322(testIdentity, req, "mid@x.com", time.Now())
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}
	_, root := parseMIME(t, raw)

	if root.mediaType != "multipart/related" {
		t.Fatalf("顶层应为 multipart/related，实际 %s", root.mediaType)
	}
	if len(root.children) != 2 {
		t.Fatalf("应有 2 个子 part，实际 %d", len(root.children))
	}
	// related 的首 part 必须是 HTML：省略 start 参数时接收方按首 part 认定根文档
	if root.children[0].mediaType != "text/html" {
		t.Errorf("related 首 part 应为 text/html，实际 %s", root.children[0].mediaType)
	}
	img := root.children[1]
	if img.contentID != "ii_abc123" {
		t.Errorf("Content-ID 应为 ii_abc123，实际 %q", img.contentID)
	}
	if img.disposition != "inline" {
		t.Errorf("内联资源的 disposition 应为 inline，实际 %q", img.disposition)
	}
	if string(img.body) != "PNGDATA" {
		t.Errorf("图片内容不符: %q", img.body)
	}
}

// TestBuildInlineAndAttachment 两者都有：mixed 包 related，内联图必须在 related 内。
func TestBuildInlineAndAttachment(t *testing.T) {
	req := baseReq()
	req.Attachments = []send.Attachment{
		{Filename: "doc.pdf", ContentType: "application/pdf", Content: []byte("PDF")},
		{Filename: "a.png", ContentType: "image/png", Content: []byte("PNG"), ContentID: "ii_abc123"},
	}
	raw, err := send.BuildRFC5322(testIdentity, req, "mid@x.com", time.Now())
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}
	_, root := parseMIME(t, raw)

	if root.mediaType != "multipart/mixed" {
		t.Fatalf("顶层应为 multipart/mixed，实际 %s", root.mediaType)
	}
	if len(root.children) != 2 {
		t.Fatalf("mixed 应有 2 个子 part（related + 附件），实际 %d", len(root.children))
	}
	related := root.children[0]
	if related.mediaType != "multipart/related" {
		t.Fatalf("mixed 首 part 应为 multipart/related，实际 %s", related.mediaType)
	}
	// 内联图必须在 related 内部——放到外层 mixed 里 Outlook 会当独立附件、图裂
	if len(related.children) != 2 || related.children[1].contentID != "ii_abc123" {
		t.Errorf("内联图未落在 related 容器内: %+v", related.children)
	}
	if root.children[1].disposition != "attachment" || root.children[1].filename != "doc.pdf" {
		t.Errorf("普通附件应在 mixed 层且 disposition=attachment，实际 %+v", root.children[1])
	}
}

// TestBuildNoAttachmentsUnchanged 无附件无内联时保持历史的单一 text/html 形态。
func TestBuildNoAttachmentsUnchanged(t *testing.T) {
	req := baseReq()
	req.BodyHTML = "<p>纯文本正文</p>"
	raw, err := send.BuildRFC5322(testIdentity, req, "mid@x.com", time.Now())
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}
	_, root := parseMIME(t, raw)
	if root.mediaType != "text/html" {
		t.Fatalf("应为单一 text/html，实际 %s", root.mediaType)
	}
	if string(root.body) != "<p>纯文本正文</p>" {
		t.Errorf("正文不符: %q", root.body)
	}
}

// TestContentIDRejectsHeaderInjection cid 会原样写进 Content-ID 头，
// 混进 CRLF 就是一次头注入，必须在构建阶段就拒掉。
func TestContentIDRejectsHeaderInjection(t *testing.T) {
	bad := []string{
		"ii_a\r\nBcc: attacker@evil.com",
		"ii_a\nX-Injected: 1",
		"ii_a b",
		"ii_<script>",
		"",
		strings.Repeat("a", 129),
	}
	for _, cid := range bad {
		if send.ValidContentID(cid) {
			t.Errorf("ValidContentID 不应接受 %q", cid)
		}
		req := baseReq()
		req.Attachments = []send.Attachment{
			{Filename: "a.png", ContentType: "image/png", Content: []byte("X"), ContentID: cid},
		}
		if cid == "" {
			continue // 空 cid 表示普通附件，不走内联分支
		}
		if _, err := send.BuildRFC5322(testIdentity, req, "mid@x.com", time.Now()); err == nil {
			t.Errorf("cid %q 应被拒绝", cid)
		}
	}
	for _, cid := range []string{"ii_abc123", "a.b_c-d", "A1"} {
		if !send.ValidContentID(cid) {
			t.Errorf("ValidContentID 应接受 %q", cid)
		}
	}
}

// TestBuildFromAliasDisplayName 别名发信时 From 头带显示名且正确编码。
func TestBuildFromAliasDisplayName(t *testing.T) {
	req := baseReq()
	req.BodyHTML = "<p>x</p>"
	id := send.Identity{Address: "sales@example.com", Name: "销售部"}
	raw, err := send.BuildRFC5322(id, req, "mid@x.com", time.Now())
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}
	msg, _ := parseMIME(t, raw)
	addr, err := mail.ParseAddress(msg.Header.Get("From"))
	if err != nil {
		t.Fatalf("From 头不可解析: %v", err)
	}
	if addr.Address != "sales@example.com" {
		t.Errorf("From 地址应为别名，实际 %q", addr.Address)
	}
	if addr.Name != "销售部" {
		t.Errorf("From 显示名应为「销售部」，实际 %q", addr.Name)
	}
}

// TestInlineDataURIImages data: 图转 cid: 内联附件——草稿直发路径的关键一步。
func TestInlineDataURIImages(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d}
	b64 := base64.StdEncoding.EncodeToString(png)
	html := `<p>a</p><img src="data:image/png;base64,` + b64 + `"><img src='data:image/jpeg;base64,` + b64 + `'>`

	out, atts, err := send.InlineDataURIImages(html)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if len(atts) != 2 {
		t.Fatalf("应产出 2 个内联附件，实际 %d", len(atts))
	}
	if strings.Contains(out, "data:image/") {
		t.Errorf("转换后不应残留 data: URI: %s", out)
	}
	for _, att := range atts {
		if !send.ValidContentID(att.ContentID) {
			t.Errorf("生成的 cid 不合法: %q", att.ContentID)
		}
		if !strings.Contains(out, "cid:"+att.ContentID) {
			t.Errorf("正文未引用 cid %q", att.ContentID)
		}
		if string(att.Content) != string(png) {
			t.Errorf("解码内容不符: %v", att.Content)
		}
	}
	if atts[0].ContentType != "image/png" || atts[1].ContentType != "image/jpeg" {
		t.Errorf("Content-Type 不符: %q / %q", atts[0].ContentType, atts[1].ContentType)
	}
	if !strings.HasSuffix(atts[1].Filename, ".jpg") {
		t.Errorf("jpeg 的扩展名应为 .jpg，实际 %q", atts[1].Filename)
	}
}

// TestInlineDataURIImagesNoop 无 data: 图时原样返回。
func TestInlineDataURIImagesNoop(t *testing.T) {
	html := `<p>纯文本 <img src="cid:ii_x"> <a href="https://example.com">链接</a></p>`
	out, atts, err := send.InlineDataURIImages(html)
	if err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	if out != html || len(atts) != 0 {
		t.Errorf("应原样返回，实际 %q / %d 个附件", out, len(atts))
	}
}

// TestInlineDataURIImagesBadBase64 解不开的 data: URI 原样保留，不能让整封发不出去。
func TestInlineDataURIImagesBadBase64(t *testing.T) {
	html := `<img src="data:image/png;base64,!!!not-base64!!!">`
	out, atts, err := send.InlineDataURIImages(html)
	if err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	if len(atts) != 0 {
		t.Errorf("不应产出附件，实际 %d", len(atts))
	}
	if out != html {
		t.Errorf("应原样保留，实际 %q", out)
	}
}

// TestFromDisplayNameCannotInjectHeaders 别名显示名是用户自由填写的，
// 它会进 From 头——含 CRLF 就可能注入 Bcc 之类的头。
func TestFromDisplayNameCannotInjectHeaders(t *testing.T) {
	req := baseReq()
	req.BodyHTML = "<p>x</p>"
	crlf := string([]byte{13, 10})
	id := send.Identity{
		Address: "sales@example.com",
		Name:    "正常" + crlf + "Bcc: attacker@evil.com" + crlf + "X-Evil: 1",
	}
	raw, err := send.BuildRFC5322(id, req, "mid@x.com", time.Now())
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}
	// 邮件必须仍然可解析，且没有多出来的头
	msg, _ := parseMIME(t, raw)
	if bcc := msg.Header.Get("Bcc"); bcc != "" {
		t.Errorf("显示名注入出了 Bcc 头: %q", bcc)
	}
	if evil := msg.Header.Get("X-Evil"); evil != "" {
		t.Errorf("显示名注入出了 X-Evil 头: %q", evil)
	}
	addr, err := mail.ParseAddress(msg.Header.Get("From"))
	if err != nil {
		t.Fatalf("From 头不可解析: %v", err)
	}
	if addr.Address != "sales@example.com" {
		t.Errorf("From 地址被篡改: %q", addr.Address)
	}
}

// TestInlineDataURIOnlyRealURLAttributes data: 图只能从真正的资源属性里取。
// 原先按 `src=` 子串匹配没有前置边界：data-src 会被改成 cid:（属性名没改，
// 收件方看到的是裂图外加一个没人引用的 inline part），正文里贴一段讲 data URI
// 的代码也会被静默篡改。
func TestInlineDataURIOnlyRealURLAttributes(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G'})
	html := `<div data-src="data:image/png;base64,` + b64 + `">` +
		`<code>写法是 data:image/png;base64,` + b64 + `</code></div>`

	out, atts, err := send.InlineDataURIImages(html)
	if err != nil {
		t.Fatalf("不该报错: %v", err)
	}
	if len(atts) != 0 {
		t.Errorf("非资源属性与正文文本不应产出内联附件，实际 %d 个", len(atts))
	}
	if out != html {
		t.Errorf("应原样返回，实际 %q", out)
	}
}

// TestInlineDataURIOtherCarriers data: 图不只出现在 img src 上：
// srcset、CSS background 的 url()、以及大写的 DATA:IMAGE 都得转，
// 漏掉的那些发出去就是被 Gmail/Outlook 屏蔽的裂图。
func TestInlineDataURIOtherCarriers(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G'})
	html := `<img srcset="data:image/png;base64,` + b64 + ` 2x">` +
		`<div style="background:url('data:image/gif;base64,` + b64 + `')"></div>` +
		`<img src="DATA:IMAGE/PNG;BASE64,` + b64 + `">` +
		`<style>.a{background-image:url(data:image/webp;base64,` + b64 + `)}</style>`

	out, atts, err := send.InlineDataURIImages(html)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if len(atts) != 4 {
		t.Fatalf("srcset / style / 大写 / <style> 块各一张，应有 4 个内联附件，实际 %d", len(atts))
	}
	if strings.Contains(strings.ToLower(out), "data:image/") {
		t.Errorf("转换后不应残留 data: URI: %s", out)
	}
	for _, att := range atts {
		if !send.ValidContentID(att.ContentID) {
			t.Errorf("生成的 cid 不合法: %q", att.ContentID)
		}
		if !strings.Contains(out, "cid:"+att.ContentID) {
			t.Errorf("正文未引用 cid %q: %s", att.ContentID, out)
		}
	}
	// srcset 的描述符（2x）必须留着，否则高清屏上尺寸算错
	if !strings.Contains(out, " 2x") {
		t.Errorf("srcset 描述符丢失: %s", out)
	}
	if atts[2].ContentType != "image/png" {
		t.Errorf("大写 DATA:IMAGE/PNG 的类型应归一化为 image/png，实际 %q", atts[2].ContentType)
	}
}

// TestInlineDataURIKeepsSrcsetSiblings srcset 里 data: 图与普通候选混排时，
// 不能把别的候选一起吃掉——data: URI 自己就带一个逗号，按逗号切会切坏。
func TestInlineDataURIKeepsSrcsetSiblings(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G'})
	html := `<img srcset="https://example.com/a.png 1x, data:image/png;base64,` + b64 + ` 2x">`

	out, atts, err := send.InlineDataURIImages(html)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if len(atts) != 1 {
		t.Fatalf("应只转换 data: 那一个候选，实际 %d 个附件", len(atts))
	}
	if !strings.Contains(out, "https://example.com/a.png 1x") {
		t.Errorf("普通候选被破坏: %s", out)
	}
	if !strings.Contains(out, "cid:"+atts[0].ContentID+" 2x") {
		t.Errorf("data: 候选未正确替换: %s", out)
	}
}

// TestInlineDataURIStyleSelfClosing <style/> 这种自闭合写法照样是样式块：
// tokenizer 给的是 SelfClosingTagToken，但后续内容仍按 rawtext 交出。
// 只认 StartTagToken 就会让里面的背景图原样发出去——收件方那边被屏蔽成裂图，
// 而这个函数存在的理由正是防这一点。
func TestInlineDataURIStyleSelfClosing(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G'})
	html := `<style/>body{background:url(data:image/png;base64,` + b64 + `)}</style><p>x</p>`

	out, atts, err := send.InlineDataURIImages(html)
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if len(atts) != 1 {
		t.Fatalf("<style/> 里的背景图应被转换，实际产出 %d 个附件: %s", len(atts), out)
	}
	if strings.Contains(strings.ToLower(out), "data:image/") {
		t.Errorf("转换后不应残留 data: URI: %s", out)
	}
	if !strings.Contains(out, "cid:"+atts[0].ContentID) {
		t.Errorf("样式块未引用 cid %q: %s", atts[0].ContentID, out)
	}
}

// TestInlineDataURIEmptyPayloadIgnored 空的 data: URI 是占位符（撰写器图片加载失败时很常见），
// 不能凭它挂一个 0 字节的 image/png inline part——部分客户端会显示成"损坏的附件"。
func TestInlineDataURIEmptyPayloadIgnored(t *testing.T) {
	for _, html := range []string{
		`<img src="data:image/png;base64,">`,
		`<img src="data:image/png;base64,   ">`,
		`<img srcset="data:image/png;base64,,b.png 2x">`,
		`<div style="background:url(data:image/gif;base64,)"></div>`,
	} {
		out, atts, err := send.InlineDataURIImages(html)
		if err != nil {
			t.Errorf("%s: 不该报错: %v", html, err)
			continue
		}
		if len(atts) != 0 {
			t.Errorf("%s: 空 payload 不应产出附件，实际 %d 个（首个 %d 字节）",
				html, len(atts), len(atts[0].Content))
		}
		if out != html {
			t.Errorf("%s: 引用应保持原样，实际 %q", html, out)
		}
	}
}
