package send

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// 草稿把内联图存成 data: URI（自包含，不需要服务端暂存区），但直接发出去是坏的：
// Outlook 与 Gmail 都屏蔽 data: 图片——这正是它们防追踪的手段之一。
// 所以发送路径上必须做一次 data: → cid: 的转换，草稿直发与前端漏网的两条路一次覆盖。
//
// 职责边界：本文件不是净化器，净化是 internal/htmlsan 的事。两者方向相反——
// htmlsan 是收信侧的白名单净化，这里是发信侧的定点改写：只碰命中 data: 图的那几个
// 标签，其余 token 原样拷回。命中的标签会被重新序列化（标签名/属性名转小写、
// 属性统一双引号、值重新转义），所以两边处理同一段 HTML 得到的字节并不相同，
// 不要指望能对得上。

// ErrPayloadTooLarge 正文/附件超出上限。这是客户端输入的问题，映射成 400 而不是 500：
// 500 会让前端提示"服务器错误"并鼓励用户重试，而重试只会再吃一遍同样的内存。
var ErrPayloadTooLarge = errors.New("payload too large")

// maxInlineDataURI 单张内联图解码后的大小上限（10 MiB）。
// maxInlineHTML 是走 data: 转换的正文上限：25 MiB 的附件总量 base64 编码后约 33.4 MiB，
// 取 40 MiB 留出正文与标记的余量。它是解码前唯一 O(1) 可判的量，也是这条路上的第一道闸——
// 没有它，"单张 10 MiB × 张数无上限"能让一个请求把堆吃干（实测 40 张 9 MiB 图占 2.4 GiB、
// 跑 209 秒才返回）。声明成 var 只为测试能注入小阈值。
var (
	maxInlineDataURI = 10 << 20
	maxInlineHTML    = 40 << 20
)

// inlineURLAttrs 是会被邮件客户端当资源引用的属性，只有它们里的 data: 图该转。
// 必须按属性名精确匹配：原先用 `src=` 子串正则没有前置边界，data-src 会被改成 cid:
// （属性名没改，收件方看到的是裂图外加一个没人引用的 inline part），
// 正文里贴一段讲 data URI 的代码也会被静默篡改。
var inlineURLAttrs = map[string]bool{
	"src": true, "poster": true, "background": true, "lowsrc": true, "dynsrc": true,
}

var (
	// 只用于已确定是一个 URL 的字符串，所以整体锚定。(?i) 在这里是有效的——
	// 预检也必须大小写不敏感，否则 DATA:IMAGE/PNG 会整条漏过去。
	reDataImage = regexp.MustCompile(`(?is)^data:image/([a-z0-9.+-]+);base64,([a-z0-9+/=\s]*)$`)
	// RE2 没有反向引用：三个分支分别匹配双引号、单引号、裸 URL（与 htmlsan 同款）
	reCSSURL = regexp.MustCompile(`(?is)url\(\s*("[^"]*"|'[^']*'|[^)'"]*)\s*\)`)
)

// InlineDataURIImages 把 HTML 里的 data: 图片替换为 cid: 引用，并返回对应的内联附件。
// 无可转换内容时原样返回，不分配额外内存。
func InlineDataURIImages(input string) (string, []Attachment, error) {
	if !containsDataImage(input) {
		return input, nil, nil
	}
	if len(input) > maxInlineHTML {
		return input, nil, fmt.Errorf("%w: body html exceeds %d bytes", ErrPayloadTooLarge, maxInlineHTML)
	}

	var (
		out     strings.Builder
		conv    inlineConv
		inStyle bool
	)
	out.Grow(len(input))
	z := html.NewTokenizer(strings.NewReader(input))
	for conv.err == nil {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		raw := z.Raw()
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			// <style> 的内容由 tokenizer 当原始文本给出，下一个 token 才是那段 CSS。
			// 自闭合写法 <style/> 也要算：tokenizer 给的是 SelfClosingTagToken，
			// 但它设 rawtag 在判自闭合之前，后续内容照样按原始文本交出——
			// 只认 StartTagToken 会让这种样式块里的背景图整段漏过去。
			inStyle = (tt == html.StartTagToken || tt == html.SelfClosingTagToken) &&
				strings.EqualFold(tok.Data, "style")
			if !conv.rewriteTag(&tok) {
				// 没改动就原样拷回：作者写的引号、转义、属性顺序都保持不变，
				// 不让一次转换顺带重写整封正文
				out.Write(raw)
				continue
			}
			writeTag(&out, tok, tt == html.SelfClosingTagToken)
		case html.TextToken:
			if inStyle {
				inStyle = false
				if css, ok := conv.convertCSS(string(raw)); ok {
					out.WriteString(css)
					continue
				}
			}
			out.Write(raw)
		default:
			inStyle = false
			out.Write(raw)
		}
	}
	if conv.err != nil {
		return input, nil, conv.err
	}
	return out.String(), conv.atts, nil
}

// inlineConv 累积一次转换的产物与已用预算。
type inlineConv struct {
	atts  []Attachment
	total int64
	err   error
}

// rewriteTag 就地改写一个开始标签里的资源引用；返回是否改动过。
func (c *inlineConv) rewriteTag(tok *html.Token) bool {
	changed := false
	for i := range tok.Attr {
		var (
			val = tok.Attr[i].Val
			ok  bool
		)
		switch key := strings.ToLower(tok.Attr[i].Key); {
		case inlineURLAttrs[key]:
			val, ok = c.convertURL(val)
		case key == "srcset":
			val, ok = c.convertSrcset(val)
		case key == "style":
			val, ok = c.convertCSS(val)
		}
		if ok {
			tok.Attr[i].Val = val
			changed = true
		}
	}
	return changed
}

// convertURL 把一个 data:image URI 换成 cid: 引用并登记附件；返回是否换过。
func (c *inlineConv) convertURL(raw string) (string, bool) {
	if c.err != nil {
		return raw, false
	}
	m := reDataImage.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return raw, false
	}
	subtype := m[1]

	// 先从编码长度算出解码后的大小，判在解码之前——先解码再判，
	// 等于把攻击者的输入完整放大一遍才拒绝。数有效字符不分配内存。
	est := int64(base64.StdEncoding.DecodedLen(base64Len(m[2])))
	if est > int64(maxInlineDataURI) {
		c.err = fmt.Errorf("%w: inline image exceeds %d bytes", ErrPayloadTooLarge, maxInlineDataURI)
		return raw, false
	}
	if c.total+est > maxAttachmentTotal {
		c.err = fmt.Errorf("%w: inline images exceed %d bytes", ErrPayloadTooLarge, maxAttachmentTotal)
		return raw, false
	}

	// base64 里允许出现换行（HTML 属性被格式化过），解码前必须先剔除空白
	decoded, err := base64.StdEncoding.DecodeString(stripWS(m[2]))
	if err != nil {
		// 解不开就原样留着：宁可这张图裂，也不要整封发不出去
		return raw, false
	}
	if len(decoded) == 0 {
		// 空 data URI 是个占位符（撰写器图片加载失败时很常见）。凭它挂一个 0 字节的
		// image/png inline part，部分客户端会显示成"损坏的附件"。当作没写。
		// 全空白的 payload 去空白后也是空串，一并挡在这里。
		return raw, false
	}
	cid, err := newContentID()
	if err != nil {
		c.err = err
		return raw, false
	}
	c.total += int64(len(decoded))
	c.atts = append(c.atts, Attachment{
		Filename:    fmt.Sprintf("image-%s.%s", cid[3:11], normalizeImageExt(subtype)),
		ContentType: "image/" + strings.ToLower(subtype),
		Content:     decoded,
		ContentID:   cid,
	})
	return "cid:" + cid, true
}

// isASCIISpace 是 base64 payload 里算作空白的字符：SPACE 与 TAB/LF/VT/FF/CR。
// base64Len 与 stripWS 必须共用同一判定——估大小与实际去空白对不上，容量预估就是错的。
func isASCIISpace(c byte) bool { return c == ' ' || (c >= 9 && c <= 13) }

// base64Len 数出 payload 里的有效 base64 字符（跳过空白），不分配内存。
func base64Len(payload string) int {
	n := 0
	for i := 0; i < len(payload); i++ {
		if !isASCIISpace(payload[i]) {
			n++
		}
	}
	return n
}

// stripWS 剔除 base64 payload 里的空白，分配量恰为有效字符数。
//
// 不能用 strings.Fields + Join：Fields 会为每段非空白 run 各留一个 16 字节的
// string header，代价随 payload 的空白密度增长，与预算闸门判的"解码后字节数"完全脱钩。
// 一个有效字符 13 M、其余全填空白的 40 MiB 正文能过闸门，却在这一步分配几百 MiB——
// 就是"40 张 9 MiB 图吃 2.4 GiB"换了个入口回来。
func stripWS(payload string) string {
	b := make([]byte, 0, base64Len(payload))
	for i := 0; i < len(payload); i++ {
		if c := payload[i]; !isASCIISpace(c) {
			b = append(b, c)
		}
	}
	return string(b)
}

// convertSrcset 逐个候选处理 srcset（"a.png 1x, data:image/png;base64,… 2x"），
// 保留每个候选的描述符（丢了描述符高清屏上尺寸就算错）。
func (c *inlineConv) convertSrcset(v string) (string, bool) {
	cands := splitSrcset(v)
	changed := false
	for i, cand := range cands {
		url, desc := splitCandidate(cand)
		converted, ok := c.convertURL(url)
		if !ok {
			continue
		}
		if desc != "" {
			converted += " " + desc
		}
		cands[i] = converted
		changed = true
	}
	if !changed {
		return v, false
	}
	return strings.Join(cands, ", "), true
}

// splitCandidate 从一个 srcset 候选里切出 URL 与描述符（"a.png 2x"）。
// 按下标切子串，不走 strings.Fields：data: URI 的 payload 里允许有空白，
// Fields 会为每段非空白 run 各留一个 string header，一个空白密集的 payload
// 就能让这一步的分配量涨到十几倍——而这发生在预算闸门判大小之前。
func splitCandidate(cand string) (url, desc string) {
	i := 0
	for i < len(cand) && isASCIISpace(cand[i]) {
		i++
	}
	j := i
	for j < len(cand) && !isASCIISpace(cand[j]) {
		j++
	}
	return cand[i:j], strings.TrimSpace(cand[j:])
}

// splitSrcset 切分 srcset 候选。不能直接按 "," 切：data: URI 自己就带一个逗号
// （data:image/png;base64,AAA），切开会把一张图拆成两半，连带把同一个 srcset 里
// 别的候选也弄坏。
func splitSrcset(v string) []string {
	var (
		out   []string
		start int
	)
	for i := 0; i < len(v); i++ {
		if v[i] == ',' && !endsWithBase64Marker(v[start:i]) {
			out = append(out, v[start:i])
			start = i + 1
		}
	}
	return append(out, v[start:])
}

// endsWithBase64Marker 判断已累积的部分是否正停在 data: URI 的 ";base64" 之后
// （那个逗号是 URI 的一部分，不是候选分隔符）。比较定长后缀，不随正文长度变慢。
func endsWithBase64Marker(cur string) bool {
	const marker = ";base64"
	if len(cur) < len(marker) {
		return false
	}
	return strings.EqualFold(cur[len(cur)-len(marker):], marker)
}

// convertCSS 处理 CSS 里的 url()——style 属性与 <style> 块都走它。
// background:url(data:image/…) 是撰写器粘贴背景图后很常见的写法，漏掉一样是裂图。
func (c *inlineConv) convertCSS(css string) (string, bool) {
	if !containsDataImage(css) {
		return css, false
	}
	changed := false
	out := reCSSURL.ReplaceAllStringFunc(css, func(s string) string {
		m := reCSSURL.FindStringSubmatch(s)
		url, ok := c.convertURL(strings.Trim(strings.TrimSpace(m[1]), `"'`))
		if !ok {
			return s
		}
		changed = true
		return `url("` + url + `")`
	})
	if !changed {
		return css, false
	}
	return out, true
}

// containsDataImage 大小写不敏感地找 data:image/ 前缀。
// strings.Contains 是大小写敏感的——原先靠它做预检，DATA:IMAGE/PNG 整条漏过去，
// 正则里的 (?i) 也就白加了。这里不做 ToLower：正文可能有几十 MB，不该为一次预检复制一遍。
func containsDataImage(s string) bool {
	const needle = "data:image/"
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i] != 'd' && s[i] != 'D' {
			continue
		}
		if strings.EqualFold(s[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

// writeTag 重新序列化一个改动过的开始标签。
func writeTag(out *strings.Builder, tok html.Token, selfClosing bool) {
	out.WriteByte('<')
	out.WriteString(tok.Data)
	for _, a := range tok.Attr {
		out.WriteByte(' ')
		out.WriteString(a.Key)
		out.WriteString(`="`)
		out.WriteString(html.EscapeString(a.Val))
		out.WriteByte('"')
	}
	if selfClosing {
		out.WriteString(" /")
	}
	out.WriteByte('>')
}

// newContentID 生成符合 ValidContentID 的随机 cid。
func newContentID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ii_" + hex.EncodeToString(b), nil
}

// normalizeImageExt 把 MIME 子类型映射为文件扩展名（仅影响附件显示名）。
func normalizeImageExt(subtype string) string {
	switch strings.ToLower(subtype) {
	case "jpeg":
		return "jpg"
	case "svg+xml":
		return "svg"
	default:
		return strings.ToLower(subtype)
	}
}
