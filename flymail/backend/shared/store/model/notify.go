package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// NotifyChannel represents a notification channel configuration
type NotifyChannel struct {
	ID            string        `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name          string        `gorm:"type:varchar(100);not null" json:"name"`
	Type          string        `gorm:"type:varchar(50);not null" json:"type"` // feishu, wecom, telegram, email, webhook
	Enabled       bool          `gorm:"default:true" json:"enabled"`
	Config        ChannelConfig `gorm:"type:json;not null" json:"config"`
	MaxRetries    int           `gorm:"default:3" json:"max_retries"`
	RetryInterval int           `gorm:"default:30" json:"retry_interval"` // seconds
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`

	// Associations
	TimeRanges []NotifyChannelTimeRange `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE" json:"time_ranges,omitempty"`
	Events     []NotifyChannelEvent     `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE" json:"events,omitempty"`
}

// TableName specifies the table name for NotifyChannel
func (NotifyChannel) TableName() string {
	return "notify_channels"
}

// ChannelConfig represents channel-specific configuration stored as JSON
type ChannelConfig map[string]interface{}

// Value implements the driver.Valuer interface
func (c ChannelConfig) Value() (driver.Value, error) {
	if c == nil {
		return "{}", nil
	}
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface
func (c *ChannelConfig) Scan(value interface{}) error {
	if value == nil {
		*c = make(ChannelConfig)
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		if str, ok := value.(string); ok {
			bytes = []byte(str)
		} else {
			return nil
		}
	}

	var result ChannelConfig
	if err := json.Unmarshal(bytes, &result); err != nil {
		return err
	}
	*c = result
	return nil
}

// NotifyChannelTimeRange represents time ranges when a channel is active
type NotifyChannelTimeRange struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ChannelID string    `gorm:"type:varchar(36);not null" json:"channel_id"`
	StartTime string    `gorm:"type:time;not null" json:"start_time"` // HH:MM format
	EndTime   string    `gorm:"type:time;not null" json:"end_time"`   // HH:MM format
	Weekdays  Weekdays  `gorm:"type:text;not null" json:"weekdays"`   // JSON array of weekdays [0-6]
	CreatedAt time.Time `json:"created_at"`
}

// Weekdays represents a list of weekdays
type Weekdays []int

// Value implements the driver.Valuer interface
func (w Weekdays) Value() (driver.Value, error) {
	if w == nil {
		return "[]", nil
	}
	return json.Marshal(w)
}

// Scan implements the sql.Scanner interface
func (w *Weekdays) Scan(value interface{}) error {
	if value == nil {
		*w = []int{}
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		if str, ok := value.(string); ok {
			bytes = []byte(str)
		} else {
			return nil
		}
	}

	var result []int
	if err := json.Unmarshal(bytes, &result); err != nil {
		return err
	}
	*w = result
	return nil
}

// NotifyChannelEvent represents event types a channel is subscribed to
type NotifyChannelEvent struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ChannelID   string    `gorm:"type:varchar(36);not null" json:"channel_id"`
	EventType   string    `gorm:"type:varchar(100);not null" json:"event_type"`
	MinSeverity string    `gorm:"type:varchar(20);default:'low'" json:"min_severity"` // low, medium, high, critical
	CreatedAt   time.Time `json:"created_at"`
}

// TableName specifies the table name for NotifyChannelEvent
func (NotifyChannelEvent) TableName() string {
	return "notify_channel_events"
}

// NotifyLog represents a notification log entry
type NotifyLog struct {
	ID           uint          `gorm:"primaryKey;autoIncrement" json:"id"`
	ChannelID    string        `gorm:"type:varchar(36);not null" json:"channel_id"`
	EventType    string        `gorm:"type:varchar(100);not null" json:"event_type"`
	EventData    EventDataJSON `gorm:"type:json;not null" json:"event_data"`
	Status       string        `gorm:"type:varchar(20);not null" json:"status"` // pending, success, failed
	RetryCount   int           `gorm:"default:0" json:"retry_count"`
	ErrorMessage string        `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	SentAt       *time.Time    `json:"sent_at,omitempty"`

	// Association
	Channel *NotifyChannel `gorm:"foreignKey:ChannelID;references:ID" json:"channel,omitempty"`
}

// TableName specifies the table name for NotifyLog
func (NotifyLog) TableName() string {
	return "notify_logs"
}

// EventDataJSON represents event data stored as JSON
type EventDataJSON map[string]interface{}

// Value implements the driver.Valuer interface
func (e EventDataJSON) Value() (driver.Value, error) {
	if e == nil {
		return "{}", nil
	}
	return json.Marshal(e)
}

// Scan implements the sql.Scanner interface
func (e *EventDataJSON) Scan(value interface{}) error {
	if value == nil {
		*e = make(EventDataJSON)
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		if str, ok := value.(string); ok {
			bytes = []byte(str)
		} else {
			return nil
		}
	}

	var result EventDataJSON
	if err := json.Unmarshal(bytes, &result); err != nil {
		return err
	}
	*e = result
	return nil
}

// Channel type constants
const (
	ChannelTypeFeishu   = "feishu"
	ChannelTypeWecom    = "wecom"
	ChannelTypeTelegram = "telegram"
	ChannelTypeEmail    = "email"
	ChannelTypeWebhook  = "webhook"
	ChannelTypeSSE      = "sse"
)

// Status constants
const (
	NotifyStatusPending = "pending"
	NotifyStatusSuccess = "success"
	NotifyStatusFailed  = "failed"
)
