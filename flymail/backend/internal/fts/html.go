package fts

import (
	"html"
	"regexp"
	"strings"
)

// StripHTML 把 HTML 正文降成可索引的纯文本：去掉 script/style 整块、剥标签、解实体、压空白。
//
// 只用于纯 HTML 邮件（text_body 为空）的兜底索引——新闻邮件与不少企业邮件只有 HTML 部分，
// 不处理它们正文就完全搜不到。不追求排版还原，只要词能被切出来。
// 块级标签换成空格而不是直接删掉：<td>发票</td><td>报销</td> 若删标签会粘成「发票报销」，
// 反而多出一个不存在的二元组。
func StripHTML(s string) string {
	if s == "" {
		return ""
	}
	s = reScriptStyle.ReplaceAllString(s, " ")
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = reSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

var (
	reScriptStyle = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(script|style)>`)
	reTag         = regexp.MustCompile(`(?s)<[^>]*>`)
	reSpace       = regexp.MustCompile(`[\s\x{00A0}]+`)
)
