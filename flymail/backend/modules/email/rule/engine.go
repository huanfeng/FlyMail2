package rule

import (
	"strings"
)

// MessageView 是规则求值看到的一封邮件：只含匹配需要的字段，与存储模型解耦，
// 试运行与实跑都从 message.Message 转成它。
type MessageView struct {
	From string // "显示名 <地址>" 形式，显示名与地址都能匹配
	To   string // 各收件人同样拼接后用 ", " 连接
	Cc   string
	// 纯地址版本：equals / ends_with 这类运算对「名 <地址>」整串几乎永远不成立（串尾是 >），
	// 地址型字段同时按纯地址求值，任一命中即算命中
	FromAddr      string
	ToAddrs       string
	CcAddrs       string
	Subject       string
	Body          string
	BodyKnown     bool // 正文尚未落库时为 false：正文 / 附件名条件按「未知 = 不命中」处理
	Attachments   []string
	HasAttachment bool
}

// Evaluate 对一封邮件求值。返回是否命中，以及求值过程中是否碰到「正文未知」的条件
// （试运行据此统计「N 封无正文未参与正文条件」）。
func (c *Compiled) Evaluate(v *MessageView) (matched bool, unknownBody bool) {
	if len(c.Conditions) == 0 {
		return false, false
	}
	for i, cond := range c.Conditions {
		hit, unknown := c.evalCondition(i, cond, v)
		if unknown {
			unknownBody = true
		}
		if c.Rule.Match == MatchAny {
			if hit {
				return true, unknownBody
			}
			continue
		}
		if !hit {
			return false, unknownBody
		}
	}
	return c.Rule.Match == MatchAll, unknownBody
}

// evalCondition 求单个条件。unknown 表示该条件依赖尚不存在的正文数据。
func (c *Compiled) evalCondition(i int, cond Condition, v *MessageView) (hit bool, unknown bool) {
	switch cond.Field {
	case FieldHasAttachment:
		// has_attachment 列由正文落库时回填，正文未到之前恒为 false——此时既不能说「有」也不能说「没有」
		if !v.BodyKnown {
			return false, true
		}
		return v.HasAttachment == (cond.Value == "true"), false
	case FieldAttachmentName:
		if !v.BodyKnown {
			return false, true
		}
		if cond.Op == OpNotContains {
			// 「没有任何附件名包含 X」：任一附件含 X 即为假；没有附件时 vacuously 为真
			for _, name := range v.Attachments {
				if strings.Contains(strings.ToLower(name), strings.ToLower(strings.TrimSpace(cond.Value))) {
					return false, false
				}
			}
			return true, false
		}
		for _, name := range v.Attachments {
			if c.match(i, cond, name) {
				return true, false
			}
		}
		return false, false
	case FieldBody:
		if !v.BodyKnown {
			return false, true
		}
		return c.match(i, cond, v.Body), false
	case FieldFrom:
		return c.matchAddr(i, cond, v.From, v.FromAddr), false
	case FieldTo:
		return c.matchAddr(i, cond, v.To, v.ToAddrs), false
	case FieldCc:
		return c.matchAddr(i, cond, v.Cc, v.CcAddrs), false
	case FieldSubject:
		return c.match(i, cond, v.Subject), false
	}
	return false, false
}

// matchAddr 对地址型字段求值：「名 <地址>」整串与纯地址任一命中即命中。
// not_contains 例外——它要求两种形态都不含（纯地址是整串的子串，看整串即可）。
func (c *Compiled) matchAddr(i int, cond Condition, full, addrs string) bool {
	if cond.Op == OpNotContains {
		return c.match(i, cond, full)
	}
	return c.match(i, cond, full) || c.match(i, cond, addrs)
}

// match 按运算比较。除 regex 外都大小写不敏感（用户想区分大小写时写 (?i) 的反面几乎不存在）。
func (c *Compiled) match(i int, cond Condition, text string) bool {
	if cond.Op == OpRegex {
		re := c.regexps[i]
		return re != nil && re.MatchString(text)
	}
	t := strings.ToLower(text)
	val := strings.ToLower(strings.TrimSpace(cond.Value))
	switch cond.Op {
	case OpContains:
		return strings.Contains(t, val)
	case OpNotContains:
		return !strings.Contains(t, val)
	case OpEquals:
		return strings.TrimSpace(t) == val
	case OpStartsWith:
		return strings.HasPrefix(strings.TrimSpace(t), val)
	case OpEndsWith:
		return strings.HasSuffix(strings.TrimSpace(t), val)
	}
	return false
}

// Blocked 判断发件地址是否命中黑名单：精确地址，或以 "@域名" 结尾。
// patterns 已经过 NormalizePattern。
func Blocked(fromAddr string, patterns []string) (string, bool) {
	addr := strings.ToLower(strings.TrimSpace(fromAddr))
	if addr == "" {
		return "", false
	}
	for _, p := range patterns {
		if strings.Contains(p, "@") {
			if addr == p {
				return p, true
			}
			continue
		}
		// 域名：@domain 精确，或 .domain 子域（本地部分在 @ 之前，HasSuffix 够不到，不会误匹配）
		if strings.HasSuffix(addr, "@"+p) || (strings.HasSuffix(addr, "."+p) && strings.Contains(addr, "@")) {
			return p, true
		}
	}
	return "", false
}
