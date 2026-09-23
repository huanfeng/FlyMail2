package translate

import (
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// ── 为什么把 HTML 拆成文本节点送去翻译，而不是整段 HTML 丢给模型 ───────────
//
// 整段丢过去有两个躲不开的问题：标签会被模型改写（丢属性、改嵌套、把
// <img src="cid:..."> 顺手"优化"掉），以及**模型的输出会当成 HTML 渲染**——
// 那等于让一个外部服务往邮件正文里注入任意标记。
//
// 拆成文本节点后两个问题一起消失：
//   - 结构一个字节都不动，译文只落在文本节点里，排版、图片、链接全部原样；
//   - 回填走的是 node.Data，html.Render 出去时 < > & 一律转义。模型就算
//     返回 "<script>alert(1)</script>"，渲染出来也只是这几个可见字符。
//
// 代价是得自己处理编号、分块与错位，就是下面这些代码。

// maxPartRunes 是单个待译片段的字符上限。
//
// 超过就拆开分别翻译：一个 <pre> 或者一个塞了整篇文章的 <p> 是常见的，
// 而把一万字压进一次请求，换来的是模型在输出中途撞上 max_tokens——
// 那时用户看到的是"译文只有前半篇"，而前半篇看起来完全正常。
const maxPartRunes = 1200

// maxChunkRunes 是一次请求里所有片段的字符上限。
//
// 比 maxPartRunes 大一截，好让零碎的片段（表格单元格、按钮文案）能凑在
// 一次请求里走完——一封营销邮件能拆出几百个三五个字的节点，逐个发请求的话
// 光往返延迟就要几分钟。
const maxChunkRunes = 3000

// skipContainers 是内容不该翻译的元素：里面的文本是给机器读的，翻了等于弄坏。
//
// htmlsan 已经把 script / style 整块删了，这里仍然列上——分段逻辑不该依赖
// "上游一定净化过"这个前提，它同样跑在未净化的原始正文上。
//
// <pre> 刻意不在名单里：邮件里的 <pre> 更多是纯文本正文转过来的（很多客户端
// 就这么发纯文本邮件），整块跳过会让一封纯文本邮件按下翻译毫无反应。
var skipContainers = map[string]bool{
	"script": true, "style": true, "title": true, "textarea": true,
	"noscript": true, "code": true,
}

// segment 是一个文本节点的待译内容。
type segment struct {
	node *html.Node
	// lead / trail 是原文本节点首尾的空白。
	//
	// 必须原样留着：HTML 里 "word <b>bold</b>" 的那个空格就在文本节点末尾，
	// 连同文本一起交给模型，模型会顺手把它吃掉，于是译文变成 "词粗体"。
	lead, trail string
	// parts 是拆开后的待译片段（通常只有一个）。
	parts []string
	// out 与 parts 一一对应；未翻译的位置为空串，回填时退回原文。
	out []string
	// result 是 apply 拼出来的最终文本。主题与纯文本正文没有节点可写回，
	// 走的就是这个字段——让它们与 HTML 节点共用同一套分段、编号、回填逻辑，
	// 而不是各写一份。
	result string
}

// parseDocument 解析 HTML 并收集所有值得翻译的文本片段。
//
// 返回的 doc 与 segs 是绑定的：segs 里握着 doc 中节点的指针，
// 回填改的就是 doc 本身，随后 renderDocument 输出。
func parseDocument(htmlBody string) (*html.Node, []*segment, error) {
	doc, err := html.Parse(strings.NewReader(htmlBody))
	if err != nil {
		return nil, nil, err
	}
	var segs []*segment
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.TextNode:
				if s := newSegment(c); s != nil {
					segs = append(segs, s)
				}
			case html.ElementNode:
				if skipContainers[c.Data] {
					continue
				}
				walk(c)
			}
			// 注释节点（html.CommentNode）不进队列：它不显示，翻译它只是在烧 token。
		}
	}
	walk(doc)
	return doc, segs, nil
}

// newSegment 从文本节点构造待译片段；不值得翻译时返回 nil。
func newSegment(n *html.Node) *segment {
	s := newTextSegment(n.Data)
	if s == nil {
		return nil
	}
	s.node = n
	return s
}

// newTextSegment 从一段纯文本构造待译片段；不值得翻译时返回 nil。
//
// 主题、纯文本正文，以及 HTML 的文本节点都从这里出，三者于是共用同一套
// 首尾空白处理与超长拆分——否则"主题不该被吃掉前导空格"这种事要修三遍。
func newTextSegment(text string) *segment {
	core := strings.TrimFunc(text, unicode.IsSpace)
	if !worthTranslating(core) {
		return nil
	}
	lead := text[:strings.Index(text, core)]
	trail := text[len(lead)+len(core):]
	parts := splitText(core, maxPartRunes)
	return &segment{lead: lead, trail: trail, parts: parts, out: make([]string, len(parts))}
}

// worthTranslating 报告一段文本是否值得送去翻译。
//
// 判据是"含不含字母"（汉字、假名、西里尔字母都算 unicode.IsLetter）：
// 纯空白、纯数字、"---"、"|"、"© 2026" 这类节点在一封营销邮件里能占到
// 一半以上的节点数，送过去既费 token 又给模型制造改写它们的机会。
func worthTranslating(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// splitText 把超长文本拆成不超过 max 字符的片段。
//
// 优先在换行处断，其次在句末标点后断，都找不到才硬切——硬切会把一个词
// 劈成两半，但那只发生在"一整段没有任何标点的超长文本"上。
func splitText(s string, max int) []string {
	runes := []rune(s)
	if len(runes) <= max {
		return []string{s}
	}
	var out []string
	for len(runes) > max {
		cut := breakPoint(runes, max)
		out = append(out, string(runes[:cut]))
		runes = runes[cut:]
	}
	if len(runes) > 0 {
		out = append(out, string(runes))
	}
	return out
}

// sentenceEnd 是可以断句的位置（断在它**之后**）。
var sentenceEnd = map[rune]bool{
	'\n': true, '。': true, '！': true, '？': true, '；': true,
	'.': true, '!': true, '?': true, ';': true,
}

// breakPoint 在 [max/2, max] 里找一个断点，找不到就返回 max。
//
// 下界 max/2 是为了不让片段碎得太厉害：宁可在一个不太理想的位置断开，
// 也不要切出一串十几个字的碎片——每个碎片都要带上编号往返一次。
func breakPoint(runes []rune, max int) int {
	lo := max / 2
	for i := max - 1; i >= lo; i-- {
		if sentenceEnd[runes[i]] {
			return i + 1
		}
	}
	return max
}

// renderDocument 把（已回填的）文档渲染回 HTML 字符串。
func renderDocument(doc *html.Node) (string, error) {
	var sb strings.Builder
	if err := html.Render(&sb, doc); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// apply 把译文写回文本节点。
//
// 任何一个片段没翻出来（模型漏了编号、返回了空串），那个位置就退回原文：
// 半句原文半句译文仍然读得懂，而空白会让用户以为邮件内容丢了。
func (s *segment) apply() {
	var sb strings.Builder
	sb.WriteString(s.lead)
	for i, p := range s.parts {
		if s.out[i] != "" {
			sb.WriteString(s.out[i])
		} else {
			sb.WriteString(p)
		}
	}
	sb.WriteString(s.trail)
	s.result = sb.String()
	if s.node != nil {
		// ⚠ 赋的是 Data（文本节点的内容），不是 HTML 源码：
		// html.Render 会把其中的 < > & 转义掉，模型返回什么都不会变成标记。
		s.node.Data = s.result
	}
}

// translated 报告这个片段是否至少有一部分被翻译了。
func (s *segment) translated() bool {
	for _, o := range s.out {
		if o != "" {
			return true
		}
	}
	return false
}
