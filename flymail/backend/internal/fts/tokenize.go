// Package fts 为 SQLite FTS5 全文索引提供中文可用的切分策略。
//
// 背景：FTS5 自带的 unicode61 分词器把一整段连续汉字当成一个 token，
// trigram 分词器又要求查询词至少 3 个字符——「发票」「张三」这类两字词一个都命中不了，
// 而两字词恰恰是中文检索的主流。驱动层也不提供注册自定义分词器的入口。
//
// 因此切分放在应用层：把每一段连续的 CJK 文字展开成相邻二元组（bigram），
// 用空格隔开后交给 unicode61 做常规索引；查询时对用户输入做同样的展开，
// 并把展开结果包成短语（phrase），要求二元组顺序相邻，这样精度与整词匹配一致。
//
//	索引：  "张三给李四发了发票"  →  "张三 三给 给李 李四 四发 发了 了发 发票"
//	查询：  "李四发"            →  "\"李四 四发\""     （短语，命中连续的「李四发」）
//
// 非 CJK 文本原样透传：unicode61 会照常按标点切词、大小写折叠，
// 英文/邮箱/数字部分不需要我们插手。
//
// 索引与查询必须用同一份切分逻辑——两边只要有一处不一致，搜索就会「看着能用但漏结果」。
// 这个包同时被数据库层（注册为 SQL 函数供触发器调用）与仓储层（构造 MATCH 表达式）引用。
package fts

import (
	"strings"
	"unicode"
)

// isCJK 判断一个字符是否需要按二元组切分。
// 覆盖汉字（含扩展区与兼容区）、日文假名、韩文音节——这些文字系统词间都没有空格。
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// Tokenize 把待索引文本中的每一段连续 CJK 文字展开成二元组序列，其余内容原样保留。
// 空串与纯非 CJK 文本返回原值（不做无谓分配）。
func Tokenize(text string) string {
	// 快速路径：没有 CJK 字符就不动
	hasCJK := false
	for _, r := range text {
		if isCJK(r) {
			hasCJK = true
			break
		}
	}
	if !hasCJK {
		return text
	}

	var b strings.Builder
	b.Grow(len(text) * 2)
	var run []rune
	flush := func() {
		if len(run) == 0 {
			return
		}
		// 前后补空格，让二元组与相邻的非 CJK 文本隔开
		b.WriteByte(' ')
		writeBigrams(&b, run)
		b.WriteByte(' ')
		run = run[:0]
	}
	for _, r := range text {
		if isCJK(r) {
			run = append(run, r)
			continue
		}
		flush()
		b.WriteRune(r)
	}
	flush()
	return b.String()
}

// writeBigrams 把一段 CJK 字符写成空格分隔的相邻二元组；只有一个字时写它本身。
func writeBigrams(b *strings.Builder, run []rune) {
	if len(run) == 1 {
		b.WriteRune(run[0])
		return
	}
	for i := 0; i+1 < len(run); i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(run[i])
		b.WriteRune(run[i+1])
	}
}

// Segment 是用户查询词切出的一段：要么是一段连续 CJK 文字，要么是一段非 CJK 文字。
type Segment struct {
	Text string
	CJK  bool
}

// Segments 把一个查询词按 CJK / 非 CJK 交界切段。
// 非 CJK 段内部的标点交给调用方处理（构造 MATCH 时会再做一次清洗）。
func Segments(term string) []Segment {
	var out []Segment
	var cur []rune
	curCJK := false
	flush := func() {
		if len(cur) > 0 {
			out = append(out, Segment{Text: string(cur), CJK: curCJK})
			cur = cur[:0]
		}
	}
	for _, r := range term {
		c := isCJK(r)
		if len(cur) > 0 && c != curCJK {
			flush()
		}
		curCJK = c
		cur = append(cur, r)
	}
	flush()
	return out
}

// MatchExpr 把一个用户输入的自由词转换成 FTS5 MATCH 片段（不含列过滤前缀）。
//
//   - CJK 段 → 二元组短语 "c1c2 c2c3 …"，要求顺序相邻，与整词匹配等价；
//     单字段退化为该字本身（只能命中索引里孤立的单字，属已知限制）
//   - 非 CJK 段 → 去掉 FTS5 语法字符后按 token 拆开，每个 token 作前缀匹配 "tok"*，
//     这样输入 inv 能命中 invoice，输入邮箱前缀能命中整个地址
//   - 各段之间以 AND（空格）连接
//
// phrase=true 时非 CJK 段按整体短语匹配（"hello world" 要求两词相邻），不做前缀展开；
// 这是用户用引号明确要求精确匹配时的语义。CJK 段本来就是短语，不受影响。
//
// 返回空串表示这个词里没有任何可检索内容（如只有标点）。
// 所有 token 都用双引号包裹，因此用户输入里的 AND/OR/NOT/括号都不会被当成语法。
func MatchExpr(term string, phrase bool) string {
	var parts []string
	for _, seg := range Segments(term) {
		if seg.CJK {
			var b strings.Builder
			writeBigrams(&b, []rune(seg.Text))
			parts = append(parts, `"`+b.String()+`"`)
			continue
		}
		toks := latinTokens(seg.Text)
		if len(toks) == 0 {
			continue
		}
		if phrase {
			parts = append(parts, `"`+strings.Join(toks, " ")+`"`)
			continue
		}
		for _, tok := range toks {
			parts = append(parts, `"`+tok+`"*`)
		}
	}
	return strings.Join(parts, " ")
}

// latinTokens 按 unicode61 的口径粗略切非 CJK 文本：字母/数字连续段为一个 token，
// 其余字符（标点、空白、引号、星号等 FTS5 语法字符）全部当分隔符丢弃。
// 这里只需与 unicode61 的「什么算 token 字符」一致即可，大小写折叠由 FTS5 自己做。
func latinTokens(s string) []string {
	var out []string
	var cur []rune
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur = append(cur, r)
			continue
		}
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}
