// Package mailtext 把邮件正文降成**给人读**的纯文本，并提取其中的链接。
//
// ── 为什么不复用 fts.StripHTML ─────────────────────────────────────────────
//
// 那个是给全文检索用的，目标是「把词切出来」：它把 [\s\xA0]+ 一律压成一个空格，
// 于是换行、段落、表格行全没了。索引不在乎，人在乎——两千字挤成一坨没有任何
// 段落的墙，比不推正文还难看。而且它也不该改：fts.StripHTML 被注册成 SQLite
// 函数供 FTS 触发器调用，改了会让已建好的索引与新写入的行对不上。
//
// 所以这里另起一套：块级标签转换行、只压水平空白。HTML 造出来的换行一律收敛
// 成一个（那是标签的副产物），纯文本里的空行留一个（那是作者敲的）。
//
// ── 链接为什么要单独提出来 ─────────────────────────────────────────────────
//
// 转成文本后 <a href="http://x">点这里</a> 只剩「点这里」，地址没了。而推送卡片
// 要让链接可点，就必须拿到地址本身。纯文本邮件则没有标签，只能从正文里按
// URL 形态扫。两条路径都归到 Extract 里，调用方不必关心原文是哪种形态。
package mailtext

import (
	"html"
	"regexp"
	"strings"
)

// Result 是一封邮件正文降级后的产物。
type Result struct {
	// Text 是可读纯文本（保留段落换行）。
	Text string
	// Links 是正文中出现的 http/https 链接，按出现顺序去重。
	Links []string
}

// maxLinks 限制提取的链接条数。
//
// 营销邮件动辄几十上百个链接（每张图、每个图标都是一个），全列出来就是刷屏，
// 而真正有用的几乎总在最前面。
const maxLinks = 8

// Extract 把邮件正文降成可读文本并提取链接。
//
// textBody 非空时按纯文本处理（只扫裸 URL）；否则把 htmlBody 转成文本，
// 链接取自 <a href> 与正文里的裸 URL。
//
// 两者都为空返回零值。
func Extract(textBody, htmlBody string) Result {
	if strings.TrimSpace(textBody) != "" {
		text := normalizeText(textBody)
		return Result{Text: text, Links: dedupeLinks(bareURLs(text))}
	}
	if strings.TrimSpace(htmlBody) == "" {
		return Result{}
	}
	text := htmlToText(htmlBody)
	// href 排在前面：<a href> 是作者明确标成链接的，比正文里顺手写的裸地址更该优先展示。
	return Result{Text: text, Links: dedupeLinks(append(hrefs(htmlBody), bareURLs(text)...))}
}

// htmlToText 把 HTML 降成保留段落的纯文本。
func htmlToText(s string) string {
	s = reComment.ReplaceAllString(s, " ")
	// script/style/head 整块丢掉：它们的内容一个字都不该出现在正文里。
	// 不丢的话仅剥标签会把整坨 CSS 和 JS 源码留在文本里。
	s = reDropBlock.ReplaceAllString(s, " ")
	// 块级边界转换行、单元格边界转空格。都要放在剥标签之前，否则边界信息就没了。
	s = reBlockBreak.ReplaceAllString(s, "\n")
	// td/th 之间是空格而不是换行：它们是同一行里的几个格子，换行会把一行表格
	// 拆成好几行。但也不能直接删——<td>发票</td><td>报销</td> 会粘成「发票报销」。
	s = reCellBreak.ReplaceAllString(s, " ")
	// 剩下的都是 b/span/a/font 这类行内标签，直接去掉；换成空格会把
	// <b>发</b><b>票</b> 拆成「发 票」。
	s = reTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	// HTML 里的换行是**我们按标签造出来的**，相邻的 </p><p> 会造出两个，
	// 嵌套 div 每层再贡献一个。这些空行不承载作者意图，一律收敛成一个换行。
	// 纯文本邮件不走这条路径——那里的空行是作者敲的，normalizeText 会留一个。
	s = reNewlines.ReplaceAllString(s, "\n")
	return normalizeText(s)
}

// normalizeText 压水平空白、整理换行。
//
// 只压水平空白（空格/制表/不换行空格），不动 \n——这正是与 fts.StripHTML 的分野。
func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = reHSpace.ReplaceAllString(s, " ")

	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			blank++
			// 连续空行最多保留一个：HTML 转出来的文本里动辄十几个连续空行
			// （嵌套的 table/div 每层都贡献一个换行），留着就是大段空白。
			if blank > 1 || len(out) == 0 {
				continue
			}
			out = append(out, "")
			continue
		}
		blank = 0
		out = append(out, ln)
	}
	// 去掉末尾空行
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// hrefs 按出现顺序取出 <a href> 里的地址。
func hrefs(htmlBody string) []string {
	ms := reHref.FindAllStringSubmatch(htmlBody, -1)
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		// 三个捕获组对应双引号/单引号/无引号三种写法，取非空的那个
		raw := m[1] + m[2] + m[3]
		out = append(out, html.UnescapeString(raw))
	}
	return out
}

// bareURLs 从纯文本里扫出 http/https 地址。
func bareURLs(text string) []string {
	return reBareURL.FindAllString(text, -1)
}

// dedupeLinks 归一、过滤并按出现顺序去重，最多 maxLinks 条。
func dedupeLinks(raw []string) []string {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, maxLinks)
	for _, u := range raw {
		u = cleanURL(u)
		if u == "" {
			continue
		}
		if _, dup := seen[u]; dup {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
		if len(out) == maxLinks {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cleanURL 校验并修剪一条链接；不合格返回空串。
//
// 只放行 http/https：javascript:、data:、file: 这些要么点不动，要么点了有害，
// 而它们混进来毫不费力——发信人写什么 href 我们就读到什么。
func cleanURL(u string) string {
	u = strings.TrimSpace(u)
	// 结尾的标点多半是句子的而不是地址的：「详见 https://x.com/a。」
	u = strings.TrimRight(u, ".,;:!?、。！？)]}>\"'")
	if u == "" {
		return ""
	}
	low := strings.ToLower(u)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		return ""
	}
	// 带控制字符的一律丢掉：正常地址不会有，而它们能在卡片里制造换行、
	// 把一条链接伪装成两行内容。
	if strings.ContainsAny(u, "\n\r\t") {
		return ""
	}
	// 超长地址（追踪链接、base64 塞进 query 的那种）没人会去点，列出来只是占地方
	if len([]rune(u)) > maxURLRunes {
		return ""
	}
	return u
}

// maxURLRunes 是单条链接的长度上限。
const maxURLRunes = 300

var (
	reComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	// script/style/head 连同内容整块丢弃
	reDropBlock = regexp.MustCompile(`(?is)<(script|style|head)\b[^>]*>.*?</(script|style|head)>`)
	// 块级边界：这些标签的起止都意味着「另起一行」
	reBlockBreak = regexp.MustCompile(`(?i)</?(br|p|div|li|ul|ol|tr|table|h[1-6]|blockquote|section|article|header|footer|hr|pre)\b[^>]*/?>`)
	// 单元格边界：同一行里的几个格子，用空格分隔
	reCellBreak = regexp.MustCompile(`(?i)</?(td|th|caption|dt|dd)\b[^>]*/?>`)
	reTag       = regexp.MustCompile(`(?s)<[^>]*>`)
	reNewlines  = regexp.MustCompile(`\n+`)
	// 只压水平空白，\n 要留着
	reHSpace = regexp.MustCompile(`[ \t\x{00A0}\x{200B}]+`)
	// href 的三种写法：双引号、单引号、无引号
	reHref = regexp.MustCompile(`(?is)<a\b[^>]*\bhref\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	// 裸 URL：到空白或明显不属于地址的字符为止
	reBareURL = regexp.MustCompile(`(?i)https?://[^\s<>"'，。；！？、）】]+`)
)
