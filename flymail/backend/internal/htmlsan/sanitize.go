// Package htmlsan 在服务端净化邮件 HTML：剥脚本与危险标签、剥事件属性、按需把远程资源引用换成占位符。
//
// 两遍：先用 x/net/html 的 tokenizer 做预处理（bluemonday 管不到的部分：<style> 块、style 属性里的 CSS、
// 远程资源计数与占位替换），再交给 bluemonday 按白名单收尾。前端 iframe 的 CSP 与 sandbox 只是纵深防御，
// 不再承担净化职责。
package htmlsan

import (
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

// Result 是净化结果。RemoteCount 是正文里远程资源引用的个数（无论是否放行都统计，前端据此显示横幅）。
type Result struct {
	HTML        string
	RemoteCount int
}

// placeholderImage 是 1×1 透明 gif：不放行远程图时替换 <img src>，保住 width/height 撑起的布局，又不发请求。
const placeholderImage = "data:image/gif;base64,R0lGODlhAQABAAAAACH5BAEKAAEALAAAAAABAAEAAAICTAEAOw=="

// dropWithContent 整块丢弃（连内容一起）的元素。
var dropWithContent = map[string]bool{
	"script": true, "iframe": true, "object": true, "embed": true, "applet": true, "noscript": true,
	"form": true, "input": true, "button": true, "textarea": true, "select": true, "option": true,
	"title": true, "template": true, "svg": true, "math": true, "frameset": true, "frame": true,
	// 原始文本 / 遗留元素：内容不该显示（否则被二次转义成乱码）
	"plaintext": true, "xmp": true, "listing": true, "noembed": true, "noframes": true,
}

// rawTextElements 是 tokenizer 进入原始文本模式的元素：即使写成 `<script/>` 自闭合，
// 后面的内容也已被当作原始文本，必须按开始标签处理（否则脚本源码会当正文显示）。
var rawTextElements = map[string]bool{
	"script": true, "style": true, "iframe": true, "noscript": true, "xmp": true, "textarea": true,
	"title": true, "noembed": true, "noframes": true, "plaintext": true,
}

// maxInput 是净化输入上限：营销邮件几 MB 的 HTML 每次点开都要完整跑一遍 tokenizer + 正则，
// 超限截断（截断处之后的内容本就不该有人看）。
const maxInput = 2 << 20

// dropTagOnly 丢弃标签本身（都是空元素，没有内容）。
var dropTagOnly = map[string]bool{"link": true, "meta": true, "base": true}

// urlAttrs 是可能携带资源引用的属性。
var urlAttrs = map[string]bool{
	"src": true, "srcset": true, "poster": true, "background": true, "lowsrc": true, "dynsrc": true,
	"data": true, "xlink:href": true, "action": true, "formaction": true,
}

var (
	// 都只在一条声明内匹配（不越过 ; 或 }），否则一个不带分号的 @import 会把后面所有规则吃光
	reImport = regexp.MustCompile(`(?is)@import\b[^;{}]*;?`)
	reDanger = regexp.MustCompile(`(?is)[^;{}]*(expression\s*\(|behavior\s*:|-moz-binding\s*:)[^;}]*;?`)
	// RE2 没有反向引用：三个分支分别匹配双引号、单引号、裸 URL
	reCSSURL = regexp.MustCompile(`(?is)url\(\s*("[^"]*"|'[^']*'|[^)'"]*)\s*\)`)
	// image-set() 不用 url() 也能引资源
	reImageSet = regexp.MustCompile(`(?is)-?(?:webkit-)?image-set\(([^)]*)\)`)
	reQuoted   = regexp.MustCompile(`"[^"]*"|'[^']*'`)
	// CSS 转义：\75 rl( 会被浏览器读成 url(；匹配前先还原
	reCSSEscape = regexp.MustCompile(`\\([0-9a-fA-F]{1,6})\s?|\\(.)`)
)

// normalizeURL 按浏览器口径归一化：去掉控制字符与空白（浏览器会丢掉 URL 里的 TAB/CR/LF），
// 反斜杠当正斜杠（特殊 scheme 的行为），小写。isRemote 与 isDangerousURL 共用，否则会出现
// 一个认得、另一个认不得的缝隙。
func normalizeURL(u string) string {
	u = strings.Map(func(r rune) rune {
		if r <= ' ' || r == 0x7f {
			return -1
		}
		if r == '\\' {
			return '/'
		}
		return r
	}, u)
	return strings.ToLower(u)
}

// isRemote 判断 URL 是否指向外部：http(s) 与协议相对地址算远程；cid: / data: / 锚点 / 相对路径不算。
func isRemote(u string) bool {
	u = normalizeURL(u)
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "//")
}

// isDangerousURL 判断 URL 是否是脚本载体。
func isDangerousURL(u string) bool {
	u = normalizeURL(u)
	return strings.HasPrefix(u, "javascript:") || strings.HasPrefix(u, "vbscript:") ||
		strings.HasPrefix(u, "data:text/html") || strings.HasPrefix(u, "data:application")
}

// cssUnescape 还原 CSS 转义序列，让后面的正则看到的与浏览器解析到的一致。
// 输出也用还原后的文本：对浏览器语义相同，且不给「转义写法」留第二次机会。
func cssUnescape(css string) string {
	if !strings.Contains(css, `\`) {
		return css
	}
	return reCSSEscape.ReplaceAllStringFunc(css, func(s string) string {
		m := reCSSEscape.FindStringSubmatch(s)
		if m[1] != "" {
			var r rune
			for _, c := range m[1] {
				r = r*16 + rune(hexVal(c))
			}
			if r == 0 || r > 0x10FFFF {
				return "�"
			}
			return string(r)
		}
		return m[2]
	})
}

func hexVal(c rune) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}

// sanitizeCSS 剥掉 @import / expression / behavior / -moz-binding，并处理 url() 与 image-set()：
// 远程的计数，不放行时换成 none；javascript: 一律换成 none。
func sanitizeCSS(css string, allowRemote bool, remote *int) string {
	css = cssUnescape(css)
	css = reImport.ReplaceAllStringFunc(css, func(s string) string {
		if isRemote(extractURLArg(s)) {
			*remote++
		}
		return ""
	})
	css = reDanger.ReplaceAllString(css, "")
	css = reImageSet.ReplaceAllStringFunc(css, func(s string) string {
		hit := false
		for _, q := range reQuoted.FindAllString(s, -1) {
			if u := strings.Trim(q, `"'`); isDangerousURL(u) || isRemote(u) {
				hit = true
				if isRemote(u) {
					*remote++
				}
			}
		}
		if hit && !allowRemote {
			return "none"
		}
		return s
	})
	return reCSSURL.ReplaceAllStringFunc(css, func(s string) string {
		m := reCSSURL.FindStringSubmatch(s)
		u := strings.Trim(strings.TrimSpace(m[1]), `"'`)
		if isDangerousURL(u) {
			return "none"
		}
		if isRemote(u) {
			*remote++
			if allowRemote {
				return s
			}
			return "none"
		}
		return s
	})
}

// srcsetHasRemote 逐个候选检查 srcset（"a.png 1x, //t.example/b.png 2x"）。
func srcsetHasRemote(v string) bool {
	for _, cand := range strings.Split(v, ",") {
		fields := strings.Fields(cand)
		if len(fields) > 0 && (isRemote(fields[0]) || isDangerousURL(fields[0])) {
			return true
		}
	}
	return false
}

// extractURLArg 从 @import "x" / @import url(x) 里取 URL。
func extractURLArg(s string) string {
	if m := reCSSURL.FindStringSubmatch(s); m != nil {
		return strings.Trim(strings.TrimSpace(m[1]), `"'`)
	}
	s = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(s), "@import"))
	return strings.Trim(strings.TrimSuffix(s, ";"), ` "'`)
}

// policy 是 bluemonday 白名单：邮件排版常用的标签与属性。style 属性放行（预处理已净化 CSS）。
var policy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements(
		"a", "abbr", "acronym", "address", "article", "aside", "b", "bdi", "bdo", "big", "blockquote", "br",
		"caption", "center", "cite", "code", "col", "colgroup", "dd", "del", "details", "dfn", "div", "dl", "dt",
		"em", "figcaption", "figure", "font", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "i",
		"img", "ins", "kbd", "label", "li", "main", "mark", "nav", "ol", "p", "pre", "q", "s", "samp", "section",
		"small", "span", "strike", "strong", "sub", "summary", "sup", "table", "tbody", "td", "tfoot", "th",
		"thead", "time", "tr", "tt", "u", "ul", "var", "wbr",
	)
	p.AllowAttrs(
		"style", "class", "id", "dir", "lang", "title", "align", "valign", "width", "height", "bgcolor", "color",
		"border", "cellpadding", "cellspacing", "colspan", "rowspan", "face", "size", "nowrap", "role", "background",
	).Globally()
	p.AllowAttrs("href", "name", "target", "rel").OnElements("a")
	p.AllowAttrs("src", "srcset", "alt", "usemap", "loading").OnElements("img")
	p.AllowAttrs("start", "type", "reversed").OnElements("ol")
	p.AllowAttrs("type").OnElements("ul", "li")
	p.AllowAttrs("datetime").OnElements("time", "del", "ins")
	p.AllowAttrs("cite").OnElements("blockquote", "q", "del", "ins")
	p.AllowAttrs("span").OnElements("col", "colgroup")
	p.AllowAttrs("headers", "scope", "abbr").OnElements("td", "th")
	p.AllowAttrs("open").OnElements("details")
	p.AllowURLSchemes("http", "https", "mailto", "tel", "cid")
	p.AllowDataURIImages()
	p.AllowRelativeURLs(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.RequireNoReferrerOnLinks(true)
	return p
}()

// Sanitize 净化邮件 HTML。allowRemote 为真时保留远程资源引用（仍计数），否则换成占位符。
func Sanitize(input string, allowRemote bool) Result {
	if strings.TrimSpace(input) == "" {
		return Result{}
	}
	if len(input) > maxInput {
		input = input[:maxInput]
	}
	var (
		out     strings.Builder
		styles  []string
		remote  int
		z       = html.NewTokenizer(strings.NewReader(input))
		skip    string // 正在整块丢弃的元素名
		skipDep int    // 同名嵌套深度
	)
	out.Grow(len(input))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		tok := z.Token()
		name := strings.ToLower(tok.Data)
		if skip != "" {
			switch tt {
			case html.StartTagToken:
				if name == skip {
					skipDep++
				}
			case html.EndTagToken:
				if name == skip {
					skipDep--
					if skipDep == 0 {
						skip = ""
					}
				}
			}
			continue
		}
		switch tt {
		case html.TextToken:
			out.WriteString(html.EscapeString(tok.Data))
		case html.StartTagToken, html.SelfClosingTagToken:
			if name == "style" {
				// tokenizer 把 <style> 内容当原始文本给出：取走净化后放到输出头部。
				// 不变量：这段 CSS 来自 tokenizer 的原始文本，所以不可能含 "</style"，直接拼回输出是安全的；
				// 若以后从别处拼 CSS，必须先转义 "</"。
				// 空样式块 <style></style> 下一个 token 是结束标签而不是文本：此时闭合标签已被消费，
				// 绝不能再置 skip，否则之后整封正文都会被当作样式内容丢掉。
				switch z.Next() {
				case html.TextToken:
					styles = append(styles, sanitizeCSS(z.Token().Data, allowRemote, &remote))
					skip, skipDep = "style", 1
				case html.ErrorToken:
					return finish(&out, styles, remote)
				}
				continue
			}
			if dropWithContent[name] {
				// 原始文本元素即使自闭合，tokenizer 也已进入原始文本模式，必须照开始标签丢内容
				if (tt == html.StartTagToken || rawTextElements[name]) && !isVoid(name) {
					skip, skipDep = name, 1
				}
				continue
			}
			if dropTagOnly[name] {
				continue
			}
			writeTag(&out, name, tok.Attr, tt == html.SelfClosingTagToken, allowRemote, &remote)
		case html.EndTagToken:
			if dropWithContent[name] || dropTagOnly[name] || name == "style" {
				continue
			}
			out.WriteString("</")
			out.WriteString(name)
			out.WriteString(">")
		case html.CommentToken, html.DoctypeToken:
			// 注释可能藏条件注释里的内容，doctype 无用，都丢
		}
	}
	return finish(&out, styles, remote)
}

// finish 跑 bluemonday 白名单并把净化过的 <style> 块拼回头部。
func finish(out *strings.Builder, styles []string, remote int) Result {
	cleaned := policy.Sanitize(out.String())
	if len(styles) > 0 {
		var b strings.Builder
		for _, css := range styles {
			b.WriteString("<style>")
			b.WriteString(css)
			b.WriteString("</style>\n")
		}
		cleaned = b.String() + cleaned
	}
	return Result{HTML: cleaned, RemoteCount: remote}
}

var voidElements = map[string]bool{
	"area": true, "br": true, "col": true, "embed": true, "hr": true, "img": true, "input": true,
	"link": true, "meta": true, "source": true, "track": true, "wbr": true, "base": true,
}

func isVoid(name string) bool { return voidElements[name] }

// writeTag 重写一个开始标签：丢事件属性与脚本 URL，处理资源引用与 style。
func writeTag(out *strings.Builder, name string, attrs []html.Attribute, selfClosing, allowRemote bool, remote *int) {
	out.WriteByte('<')
	out.WriteString(name)
	for _, a := range attrs {
		key := strings.ToLower(a.Key)
		val := a.Val
		switch {
		case strings.HasPrefix(key, "on"):
			continue
		case key == "rel" || key == "target":
			// 由 bluemonday 统一补 target=_blank rel=noopener noreferrer，不让邮件自带的值参与拼装
			continue
		case key == "style":
			val = sanitizeCSS(val, allowRemote, remote)
		case key == "href":
			if isDangerousURL(val) {
				continue
			}
		case urlAttrs[key]:
			if isDangerousURL(val) {
				continue
			}
			if key == "srcset" {
				if srcsetHasRemote(val) {
					*remote++
					if !allowRemote {
						continue
					}
				}
			} else if isRemote(val) {
				*remote++
				if !allowRemote {
					if key == "src" && name == "img" {
						val = placeholderImage
					} else {
						continue
					}
				}
			}
		}
		out.WriteByte(' ')
		out.WriteString(key)
		out.WriteString(`="`)
		out.WriteString(html.EscapeString(val))
		out.WriteByte('"')
	}
	if selfClosing {
		out.WriteString(" /")
	}
	out.WriteByte('>')
}
