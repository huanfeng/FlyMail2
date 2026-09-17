package notify

import "time"

// EventType 通知事件类型。
type EventType string

const (
	EventMailNew       EventType = "mail_new"       // 新邮件到达
	EventSyncFailed    EventType = "sync_failed"    // 同步失败
	EventAccountStatus EventType = "account_status" // 账户状态变化
	EventMailRule      EventType = "mail_rule"      // 规则命中（规则动作「触发通知」）
)

// ValidEvent 校验事件类型是否受支持。
func ValidEvent(t string) bool {
	switch EventType(t) {
	case EventMailNew, EventSyncFailed, EventAccountStatus, EventMailRule:
		return true
	}
	return false
}

// ChannelKind 外发渠道类型。
type ChannelKind string

const (
	KindWebhook ChannelKind = "webhook" // 通用 webhook（POST JSON）
	KindFeishu  ChannelKind = "feishu"  // 飞书自定义机器人
)

// ValidKind 校验渠道类型。
func ValidKind(k string) bool {
	switch ChannelKind(k) {
	case KindWebhook, KindFeishu:
		return true
	}
	return false
}

// ContentLevel 决定外发通知里带多少邮件内容。
//
// ── 为什么要分级 ─────────────────────────────────────────────────────────────
//
// 外发通知会进到第三方（飞书群、任意 webhook），而「主题不敏感、正文敏感」的邮件
// 很常见：验证码、密码重置、财务金额、医疗信息。一条推到多人群里的摘要是撤不回来的。
// 反过来，接到私人 webhook 做自动化处理的人又希望拿到全文。
// 同一个口径满足不了这两边，所以按渠道各配一档。
const (
	// LevelBasic 只有发件人与主题，正文一个字都不带。
	LevelBasic ContentLevel = "basic"
	// LevelSnippet 带正文开头的摘要（默认，也是历史行为）。
	LevelSnippet ContentLevel = "snippet"
	// LevelFull 带正文全文，按各渠道自己的上限截断。
	LevelFull ContentLevel = "full"
)

type ContentLevel string

// DefaultContentLevel 是没配时的取值。
//
// ⚠ 必须是 snippet：这个字段是后加的，老渠道在库里是空串。回落到别的档会在
// 用户毫不知情的情况下改变他已有渠道的推送内容——往上是突然外泄全文，
// 往下是突然收不到摘要。
const DefaultContentLevel = LevelSnippet

// ValidContentLevel 校验内容级别。
func ValidContentLevel(s string) bool {
	switch ContentLevel(s) {
	case LevelBasic, LevelSnippet, LevelFull:
		return true
	}
	return false
}

// wantsBody 表示这一档需要正文（摘要或全文）。
func (l ContentLevel) wantsBody() bool { return l == LevelSnippet || l == LevelFull }

// contentLevel 返回该渠道生效的内容级别（空值回落到默认）。
func (c *Channel) contentLevel() ContentLevel {
	if ValidContentLevel(c.ContentLevel) {
		return ContentLevel(c.ContentLevel)
	}
	return DefaultContentLevel
}

// Notification 是站内通知中心的一条事件记录。
// MessageID 仅在单封新邮件事件时非 0，供前端精准跳转到该邮件。
type Notification struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Type      string    `gorm:"index;not null" json:"type"`
	AccountID uint      `json:"account_id"`
	MessageID uint      `gorm:"not null;default:0" json:"message_id,omitempty"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Read      bool      `gorm:"not null;default:false;index" json:"read"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (Notification) TableName() string { return "notifications" }

// Channel 是一个外发推送渠道配置。Events 以逗号分隔存储订阅的事件类型。
type Channel struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	Name    string `gorm:"not null" json:"name"`
	Kind    string `gorm:"not null" json:"kind"`
	URL     string `gorm:"not null" json:"url"`
	Secret  string `json:"-"` // 密文/密钥不外发
	Events  string `gorm:"not null;default:''" json:"-"`
	Enabled bool   `gorm:"not null;default:true" json:"enabled"`
	// ContentLevel 决定这个渠道收到多少邮件内容（basic / snippet / full）。
	// 空串表示没配，按 DefaultContentLevel 处理——老渠道都是这种状态。
	ContentLevel string `gorm:"not null;default:''" json:"content_level"`
	// Template 预留给自定义排版：内置排版够用之前它恒为空，接口上也还没开放编辑。
	// 先占住位置是为了将来加模板时不必再动一次表结构和 DTO。
	Template  string    `gorm:"not null;default:''" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Channel) TableName() string { return "notify_channels" }

// Log 是一条外发投递日志。
type Log struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ChannelID   uint      `gorm:"index" json:"channel_id"`
	ChannelName string    `json:"channel_name"`
	Type        string    `json:"type"`
	Status      string    `json:"status"` // ok | failed
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

func (Log) TableName() string { return "notify_logs" }
