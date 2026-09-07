package fts

import (
	"strings"
	"time"
)

// Term 是一个自由检索词。Phrase 表示用户用双引号括起来的整段，
// 非 CJK 部分按整体短语匹配（不做前缀展开）。
type Term struct {
	Text   string
	Phrase bool
}

// Query 是搜索语法解析后的结构化条件。
//
// 语法（Gmail 风格，限定符大小写不敏感，取值可用双引号包裹含空格的内容）：
//
//	from:张三  to:me@x.com  subject:发票      → 落到对应 FTS 列过滤
//	has:attachment                            → HasAttachment=true
//	is:unread / is:read / is:starred / is:unstarred
//	before:2024-06-01  after:2024-01          → 日期区间（before 不含当天，after 含当天）
//	in:inbox / in:已发送                       → 文件夹类型或显示名
//	account:foo@bar.com                       → 账户邮箱或名称
//	其余词（含 "带空格的短语"）                 → 自由词，全列 AND 匹配
//
// 非法/无法识别的限定符取值不报错，退化成普通自由词——搜索是渐进增强，
// 用户拼错一个限定符不该让整个搜索失败。
type Query struct {
	Terms   []Term
	From    []Term
	To      []Term
	Subject []Term

	HasAttachment *bool
	Seen          *bool
	Flagged       *bool
	Before        *time.Time
	After         *time.Time
	Folder        string
	Account       string
}

// Empty 表示解析后没有任何有效条件（如输入全是空白或标点）。
func (q Query) Empty() bool {
	return q.Match() == "" && !q.HasStructured()
}

// HasStructured 表示是否存在需要落到 SQL WHERE（而非 FTS MATCH）的条件。
func (q Query) HasStructured() bool {
	return q.HasAttachment != nil || q.Seen != nil || q.Flagged != nil ||
		q.Before != nil || q.After != nil || q.Folder != "" || q.Account != ""
}

// Match 构造 FTS5 MATCH 表达式；没有任何文本条件时返回空串（调用方跳过 FTS 子查询）。
//
// 列名与 messages_fts 虚表一致：subject / from_name / from_addr / recipients / snippet / body。
// 自由词不限列；from: 落到 {from_name from_addr}，to: 落到 recipients，subject: 落到 subject。
// 各条件间以显式 AND 连接：FTS5 只对相邻的裸短语做隐式 AND，
// 带括号或列过滤的子表达式之间不写 AND 会直接报语法错误。
func (q Query) Match() string {
	var parts []string
	add := func(prefix string, terms []Term) {
		for _, t := range terms {
			expr := MatchExpr(t.Text, t.Phrase)
			if expr == "" {
				continue
			}
			if prefix == "" {
				parts = append(parts, "("+expr+")")
			} else {
				parts = append(parts, prefix+": ("+expr+")")
			}
		}
	}
	add("", q.Terms)
	add("{from_name from_addr}", q.From)
	add("recipients", q.To)
	add("subject", q.Subject)
	return strings.Join(parts, " AND ")
}

// Parse 解析搜索输入。永不返回错误：任何无法理解的片段都退化为自由词。
func Parse(input string) Query {
	var q Query
	for _, tok := range splitTokens(input) {
		key, val, isQual := splitQualifier(tok.text)
		if !isQual {
			q.Terms = append(q.Terms, Term{Text: tok.text, Phrase: tok.quoted})
			continue
		}
		if !q.applyQualifier(key, val, tok.quoted) {
			// 认识这个限定符但取值非法，整段退化为自由词；
			// 空取值（如孤零零的 "from:"）直接丢弃，没有可搜的内容。
			if val != "" {
				q.Terms = append(q.Terms, Term{Text: tok.text, Phrase: tok.quoted})
			}
		}
	}
	return q
}

// applyQualifier 把 key:val 落到结构化字段；返回 false 表示取值非法，交由调用方退化处理。
// quoted 表示取值带了引号（subject:"hello world"），文本类限定符要保留短语语义。
func (q *Query) applyQualifier(key, val string, quoted bool) bool {
	if val == "" {
		return false
	}
	lower := strings.ToLower(val)
	switch key {
	case "from":
		q.From = append(q.From, Term{Text: val, Phrase: quoted})
	case "to":
		q.To = append(q.To, Term{Text: val, Phrase: quoted})
	case "subject":
		q.Subject = append(q.Subject, Term{Text: val, Phrase: quoted})
	case "has":
		switch lower {
		case "attachment", "attachments", "file", "files", "附件":
			q.HasAttachment = boolPtr(true)
		default:
			return false
		}
	case "is":
		switch lower {
		case "unread", "未读":
			q.Seen = boolPtr(false)
		case "read", "已读":
			q.Seen = boolPtr(true)
		case "starred", "flagged", "star", "星标":
			q.Flagged = boolPtr(true)
		case "unstarred", "unflagged":
			q.Flagged = boolPtr(false)
		default:
			return false
		}
	case "before":
		d, ok := parseDate(val)
		if !ok {
			return false
		}
		q.Before = &d
	case "after":
		d, ok := parseDate(val)
		if !ok {
			return false
		}
		q.After = &d
	case "in", "folder":
		q.Folder = val
	case "account":
		q.Account = val
	}
	return true
}

// knownQualifiers 是会被当作限定符处理的键；其它 `x:y` 形式一律视为普通文本
// （比如用户直接搜 "Re:" 或某个 URL）。
var knownQualifiers = map[string]bool{
	"from": true, "to": true, "subject": true, "has": true, "is": true,
	"before": true, "after": true, "in": true, "folder": true, "account": true,
}

// splitQualifier 把 "key:value" 拆开；不是已知限定符时返回 isQual=false。
func splitQualifier(s string) (key, val string, isQual bool) {
	i := strings.IndexByte(s, ':')
	if i <= 0 {
		return "", "", false
	}
	key = strings.ToLower(s[:i])
	if !knownQualifiers[key] {
		return "", "", false
	}
	return key, strings.TrimSpace(s[i+1:]), true
}

type token struct {
	text   string
	quoted bool
}

// splitTokens 按空白切分，双引号内的空白不切；引号可以出现在限定符取值位置
// （subject:"a b"），也可以整段引用（"a b"）。中文弯引号同样识别。未闭合的引号一直延伸到末尾。
func splitTokens(s string) []token {
	var out []token
	var cur []rune
	inQuote := false
	quoted := false
	flush := func() {
		if len(cur) > 0 {
			out = append(out, token{text: string(cur), quoted: quoted})
		}
		cur = cur[:0]
		quoted = false
	}
	for _, r := range s {
		switch {
		case r == '"' || r == '“' || r == '”':
			inQuote = !inQuote
			if inQuote {
				quoted = true
			}
		case !inQuote && (r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '　'):
			flush()
		default:
			cur = append(cur, r)
		}
	}
	flush()
	return out
}

// parseDate 接受 YYYY-MM-DD / YYYY/MM/DD / YYYY-MM / YYYY，按本地时区解释为当天零点。
func parseDate(s string) (time.Time, bool) {
	s = strings.ReplaceAll(s, "/", "-")
	for _, layout := range []string{"2006-01-02", "2006-1-2", "2006-01", "2006-1", "2006"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func boolPtr(b bool) *bool { return &b }
