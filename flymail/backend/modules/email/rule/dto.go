package rule

import (
	"encoding/json"
	"time"

	"flymail/modules/email/message"
)

// RuleInput 是创建 / 更新规则的入参。
type RuleInput struct {
	Name           string      `json:"name"`
	Enabled        *bool       `json:"enabled"`
	AccountID      uint        `json:"account_id"`
	Match          string      `json:"match"`
	Conditions     []Condition `json:"conditions"`
	Actions        []Action    `json:"actions"`
	StopProcessing bool        `json:"stop_processing"`
}

// toRule 把入参装成模型（Conditions / Actions 序列化为 JSON 列）。
func (in RuleInput) toRule() Rule {
	conds, _ := json.Marshal(in.Conditions)
	acts, _ := json.Marshal(in.Actions)
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	m := in.Match
	if m == "" {
		m = MatchAll
	}
	return Rule{
		Name: in.Name, Enabled: enabled, AccountID: in.AccountID, Match: m,
		Conditions: string(conds), Actions: string(acts), StopProcessing: in.StopProcessing,
	}
}

// RuleDTO 是对外表示：Conditions / Actions 展开成数组。
type RuleDTO struct {
	ID             uint        `json:"id"`
	Name           string      `json:"name"`
	Enabled        bool        `json:"enabled"`
	Priority       int         `json:"priority"`
	AccountID      uint        `json:"account_id"`
	Match          string      `json:"match"`
	Conditions     []Condition `json:"conditions"`
	Actions        []Action    `json:"actions"`
	StopProcessing bool        `json:"stop_processing"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

func toDTO(r Rule) RuleDTO {
	d := RuleDTO{
		ID: r.ID, Name: r.Name, Enabled: r.Enabled, Priority: r.Priority, AccountID: r.AccountID,
		Match: r.Match, StopProcessing: r.StopProcessing, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		Conditions: []Condition{}, Actions: []Action{},
	}
	if r.Conditions != "" {
		_ = json.Unmarshal([]byte(r.Conditions), &d.Conditions)
	}
	if r.Actions != "" {
		_ = json.Unmarshal([]byte(r.Actions), &d.Actions)
	}
	return d
}

// TestResult 是试运行的结果：命中列表 + 扫描规模 + 无正文而未参与正文条件的封数。
type TestResult struct {
	Matched     []message.MessageListItem `json:"matched"`
	Scanned     int                       `json:"scanned"`
	WithoutBody int                       `json:"without_body"`
	// Truncated 表示命中数超过回传上限，Matched 只是前一部分
	Truncated bool `json:"truncated"`
}
