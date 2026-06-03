package notify

import "time"

// EventType 通知事件类型。
type EventType string

const (
	EventMailNew       EventType = "mail_new"       // 新邮件到达
	EventSyncFailed    EventType = "sync_failed"    // 同步失败
	EventAccountStatus EventType = "account_status" // 账户状态变化
)

// ValidEvent 校验事件类型是否受支持。
func ValidEvent(t string) bool {
	switch EventType(t) {
	case EventMailNew, EventSyncFailed, EventAccountStatus:
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

// Notification 是站内通知中心的一条事件记录。
type Notification struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Type      string    `gorm:"index;not null" json:"type"`
	AccountID uint      `json:"account_id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Read      bool      `gorm:"not null;default:false;index" json:"read"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (Notification) TableName() string { return "notifications" }

// Channel 是一个外发推送渠道配置。Events 以逗号分隔存储订阅的事件类型。
type Channel struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"not null" json:"name"`
	Kind      string    `gorm:"not null" json:"kind"`
	URL       string    `gorm:"not null" json:"url"`
	Secret    string    `json:"-"` // 密文/密钥不外发
	Events    string    `gorm:"not null;default:''" json:"-"`
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
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
