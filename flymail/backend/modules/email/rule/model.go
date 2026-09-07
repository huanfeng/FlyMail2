package rule

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Rule 是一条用户定义的收件规则：按 Priority 升序执行，命中后按 Actions 逐项执行。
// Conditions / Actions 以 JSON 存列：规则数量小、结构会演进，不值得拆成子表。
type Rule struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Name           string    `gorm:"not null" json:"name"`
	Enabled        bool      `gorm:"not null;default:true" json:"enabled"`
	Priority       int       `gorm:"not null;default:0;index" json:"priority"`
	AccountID      uint      `gorm:"not null;default:0" json:"account_id"` // 0 = 全部账户
	Match          string    `gorm:"not null;default:all" json:"match"`    // all | any
	Conditions     string    `gorm:"type:text" json:"-"`
	Actions        string    `gorm:"type:text" json:"-"`
	StopProcessing bool      `gorm:"not null;default:false" json:"stop_processing"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Rule) TableName() string { return "inbox_rules" }

// BlockEntry 是黑名单里的一个发件地址或域名（小写，域名不带 @）。
type BlockEntry struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Pattern   string    `gorm:"uniqueIndex;not null" json:"pattern"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

func (BlockEntry) TableName() string { return "block_list" }

// RuleRun 记录「某封邮件已被某条规则处理过」，是幂等的依据：
// (account_id, message_key, rule_id) 唯一，message_key 取 RFC Message-ID（缺则 u{folder}-{uid}），
// UIDVALIDITY 重建后主键全变而 Message-ID 不变，所以不用主键。黑名单命中的 rule_id = 0。
type RuleRun struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	AccountID  uint      `gorm:"not null;uniqueIndex:idx_rule_run_key,priority:1" json:"account_id"`
	MessageKey string    `gorm:"not null;uniqueIndex:idx_rule_run_key,priority:2" json:"message_key"`
	RuleID     uint      `gorm:"not null;uniqueIndex:idx_rule_run_key,priority:3" json:"rule_id"`
	RuleName   string    `json:"rule_name"`
	Action     string    `json:"action"` // 执行了的动作摘要，如 "move:归档,mark_read"
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}

func (RuleRun) TableName() string { return "rule_runs" }

// ── 条件与动作 ────────────────────────────────────────────────────────────────

const (
	MatchAll = "all"
	MatchAny = "any"

	FieldFrom           = "from"
	FieldTo             = "to"
	FieldCc             = "cc"
	FieldSubject        = "subject"
	FieldBody           = "body"
	FieldAttachmentName = "attachment_name"
	FieldHasAttachment  = "has_attachment"

	OpContains    = "contains"
	OpNotContains = "not_contains"
	OpEquals      = "equals"
	OpRegex       = "regex"
	OpStartsWith  = "starts_with"
	OpEndsWith    = "ends_with"

	ActionMove     = "move"
	ActionMarkRead = "mark_read"
	ActionStar     = "star"
	ActionDelete   = "delete"
	ActionNotify   = "notify"
)

var (
	validFields  = map[string]bool{FieldFrom: true, FieldTo: true, FieldCc: true, FieldSubject: true, FieldBody: true, FieldAttachmentName: true, FieldHasAttachment: true}
	validOps     = map[string]bool{OpContains: true, OpNotContains: true, OpEquals: true, OpRegex: true, OpStartsWith: true, OpEndsWith: true}
	validActions = map[string]bool{ActionMove: true, ActionMarkRead: true, ActionStar: true, ActionDelete: true, ActionNotify: true}
)

// Condition 是一个匹配条件。has_attachment 只认 equals，Value 为 true/false。
type Condition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// Action 是一个动作。move 的 Value 是目标文件夹的 display_name 或 path（按账户解析），其余为空。
type Action struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// ErrInvalid 表示规则输入不合法，附带具体原因。
var ErrInvalid = errors.New("invalid rule")

// Compiled 是解析并校验过的规则：正则已编译，可直接求值。试运行与实跑共用。
type Compiled struct {
	Rule       Rule
	Conditions []Condition
	Actions    []Action
	regexps    map[int]*regexp.Regexp // 条件下标 → 编译好的正则
}

// Compile 解析 JSON 列并校验；正则编译失败、字段/运算/动作非法都返回 ErrInvalid。
func Compile(r Rule) (*Compiled, error) {
	c := &Compiled{Rule: r, regexps: map[int]*regexp.Regexp{}}
	if strings.TrimSpace(r.Name) == "" {
		return nil, fmt.Errorf("%w: 名称不能为空", ErrInvalid)
	}
	if r.Match != MatchAll && r.Match != MatchAny {
		return nil, fmt.Errorf("%w: match 取值只能是 all / any", ErrInvalid)
	}
	if r.Conditions != "" {
		if err := json.Unmarshal([]byte(r.Conditions), &c.Conditions); err != nil {
			return nil, fmt.Errorf("%w: 条件不是合法 JSON", ErrInvalid)
		}
	}
	if r.Actions != "" {
		if err := json.Unmarshal([]byte(r.Actions), &c.Actions); err != nil {
			return nil, fmt.Errorf("%w: 动作不是合法 JSON", ErrInvalid)
		}
	}
	if len(c.Conditions) == 0 {
		return nil, fmt.Errorf("%w: 至少需要一个条件", ErrInvalid)
	}
	if len(c.Actions) == 0 {
		return nil, fmt.Errorf("%w: 至少需要一个动作", ErrInvalid)
	}
	for i, cond := range c.Conditions {
		if !validFields[cond.Field] {
			return nil, fmt.Errorf("%w: 未知字段 %q", ErrInvalid, cond.Field)
		}
		if !validOps[cond.Op] {
			return nil, fmt.Errorf("%w: 未知运算 %q", ErrInvalid, cond.Op)
		}
		if cond.Field == FieldHasAttachment {
			if cond.Op != OpEquals || (cond.Value != "true" && cond.Value != "false") {
				return nil, fmt.Errorf("%w: has_attachment 只支持 equals true/false", ErrInvalid)
			}
			continue
		}
		if strings.TrimSpace(cond.Value) == "" {
			return nil, fmt.Errorf("%w: 条件 %d 的取值不能为空", ErrInvalid, i+1)
		}
		if cond.Op == OpRegex {
			// Go regexp 是 RE2：线性时间，没有回溯爆炸，不需要额外的超时保护
			re, err := regexp.Compile(cond.Value)
			if err != nil {
				return nil, fmt.Errorf("%w: 正则 %q 无法编译: %v", ErrInvalid, cond.Value, err)
			}
			c.regexps[i] = re
		}
	}
	moves := 0
	for _, a := range c.Actions {
		if !validActions[a.Type] {
			return nil, fmt.Errorf("%w: 未知动作 %q", ErrInvalid, a.Type)
		}
		if a.Type == ActionMove {
			if strings.TrimSpace(a.Value) == "" {
				return nil, fmt.Errorf("%w: 移动动作必须指定目标文件夹", ErrInvalid)
			}
			moves++
		}
	}
	// 一封邮件只能在一个地方：同一条规则里两个 move 没有意义，且执行顺序无法定义
	if moves > 1 {
		return nil, fmt.Errorf("%w: 一条规则只能有一个移动动作", ErrInvalid)
	}
	return c, nil
}

// NeedsBody 表示规则含正文条件；NeedsAttachments 表示含附件名条件。两者都要正文落库后才有数据。
func (c *Compiled) NeedsBody() bool {
	for _, cond := range c.Conditions {
		if cond.Field == FieldBody {
			return true
		}
	}
	return false
}

func (c *Compiled) NeedsAttachments() bool {
	for _, cond := range c.Conditions {
		if cond.Field == FieldAttachmentName {
			return true
		}
	}
	return false
}

// NormalizePattern 把黑名单输入归一化：小写、去空白、域名去掉前导 @。
// 返回空串表示非法（含空格、或既不是地址也不像域名）。
func NormalizePattern(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "@")
	if s == "" || strings.ContainsAny(s, " \t\r\n,;<>\"") {
		return ""
	}
	// 地址：恰好一个 @ 且两侧非空；域名：不含 @ 且含 .
	if i := strings.Count(s, "@"); i == 1 {
		parts := strings.SplitN(s, "@", 2)
		if parts[0] == "" || parts[1] == "" {
			return ""
		}
		return s
	} else if i > 1 {
		return ""
	}
	if !strings.Contains(s, ".") {
		return ""
	}
	return s
}
