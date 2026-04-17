package notify

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// NotifyChannel represents a notification channel configuration
type NotifyChannel struct {
	ID          uint          `gorm:"primaryKey" json:"id"`
	UserID      uint          `gorm:"not null;index" json:"user_id"`
	Name        string        `gorm:"not null" json:"name"`
	Type        ChannelType   `gorm:"not null" json:"type"`
	Config      ChannelConfig `gorm:"type:json" json:"config"`
	Status      ChannelStatus `gorm:"default:'active'" json:"status"`
	Priority    int           `gorm:"default:0" json:"priority"`
	Description string        `json:"description"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`

	// Associations
	TimeRanges []NotifyChannelTimeRange `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE" json:"time_ranges,omitempty"`
	Events     []NotifyChannelEvent     `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE" json:"events,omitempty"`
}

// NotifyChannelTimeRange represents when a channel is active
type NotifyChannelTimeRange struct {
	ID        uint          `gorm:"primaryKey" json:"id"`
	ChannelID uint          `gorm:"not null;index" json:"channel_id"`
	Type      TimeRangeType `gorm:"not null" json:"type"`
	StartTime string        `gorm:"not null" json:"start_time"` // Format: "HH:MM"
	EndTime   string        `gorm:"not null" json:"end_time"`   // Format: "HH:MM"
	Timezone  string        `gorm:"default:'UTC'" json:"timezone"`
}

// NotifyChannelEvent represents which events a channel subscribes to
type NotifyChannelEvent struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ChannelID uint      `gorm:"not null;index" json:"channel_id"`
	EventType EventType `gorm:"not null" json:"event_type"`
	Severity  Severity  `json:"severity,omitempty"`
}

// NotifyLog represents a notification send log
type NotifyLog struct {
	ID         uint                   `gorm:"primaryKey" json:"id"`
	UserID     uint                   `gorm:"index" json:"user_id"`
	ChannelID  uint                   `gorm:"index" json:"channel_id"`
	EventType  EventType              `json:"event_type"`
	Severity   Severity               `json:"severity"`
	Title      string                 `json:"title"`
	Message    string                 `json:"message"`
	Status     LogStatus              `json:"status"`
	Error      string                 `json:"error,omitempty"`
	RetryCount int                    `json:"retry_count"`
	SentAt     *time.Time             `json:"sent_at,omitempty"`
	Data       map[string]interface{} `gorm:"type:json" json:"data,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

// ChannelConfig represents channel-specific configuration
type ChannelConfig map[string]interface{}

// Value implements driver.Valuer interface
func (c ChannelConfig) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil
	}
	return json.Marshal(c)
}

// Scan implements sql.Scanner interface
func (c *ChannelConfig) Scan(value interface{}) error {
	if value == nil {
		*c = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}

	return json.Unmarshal(bytes, c)
}

// TableName specifies the table name for NotifyChannel
func (NotifyChannel) TableName() string {
	return "notify_channels"
}

// TableName specifies the table name for NotifyChannelTimeRange
func (NotifyChannelTimeRange) TableName() string {
	return "notify_channel_time_ranges"
}

// TableName specifies the table name for NotifyChannelEvent
func (NotifyChannelEvent) TableName() string {
	return "notify_channel_events"
}

// TableName specifies the table name for NotifyLog
func (NotifyLog) TableName() string {
	return "notify_logs"
}

// BeforeCreate sets default values before creating
func (n *NotifyChannel) BeforeCreate(tx *gorm.DB) error {
	if n.Status == "" {
		n.Status = ChannelStatusActive
	}
	return nil
}
