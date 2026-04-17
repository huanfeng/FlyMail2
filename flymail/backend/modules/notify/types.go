package notify

import (
	"time"
)

// EventType represents the type of notification event
type EventType string

const (
	// Email related events
	EventNewEmail       EventType = "new_email"
	EventEmailSyncStart EventType = "email_sync_start"
	EventEmailSyncDone  EventType = "email_sync_done"
	EventEmailSyncFail  EventType = "email_sync_fail"

	// Account related events
	EventAccountAdded   EventType = "account_added"
	EventAccountUpdated EventType = "account_updated"
	EventAccountDeleted EventType = "account_deleted"
	EventAccountError   EventType = "account_error"

	// Task related events
	EventTaskCreated   EventType = "task_created"
	EventTaskStarted   EventType = "task_started"
	EventTaskCompleted EventType = "task_completed"
	EventTaskFailed    EventType = "task_failed"

	// System related events
	EventSystemStartup  EventType = "system_startup"
	EventSystemShutdown EventType = "system_shutdown"
	EventSystemError    EventType = "system_error"
	EventSystemWarning  EventType = "system_warning"
)

// Severity represents the severity level of an event
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Event represents a notification event
type Event struct {
	Type      EventType              `json:"type"`
	Severity  Severity               `json:"severity"`
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Timestamp time.Time              `json:"timestamp"`
	UserID    uint                   `json:"user_id,omitempty"`
	AccountID uint                   `json:"account_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// ChannelType represents the type of notification channel
type ChannelType string

const (
	ChannelTypeFeishu   ChannelType = "feishu"
	ChannelTypeWecom    ChannelType = "wecom"
	ChannelTypeTelegram ChannelType = "telegram"
	ChannelTypeEmail    ChannelType = "email"
	ChannelTypeWebhook  ChannelType = "webhook"
	ChannelTypeSSE      ChannelType = "sse"
)

// ChannelStatus represents the status of a notification channel
type ChannelStatus string

const (
	ChannelStatusActive   ChannelStatus = "active"
	ChannelStatusInactive ChannelStatus = "inactive"
	ChannelStatusError    ChannelStatus = "error"
)

// TimeRangeType represents the type of time range
type TimeRangeType string

const (
	TimeRangeTypeWeekday TimeRangeType = "weekday"
	TimeRangeTypeWeekend TimeRangeType = "weekend"
	TimeRangeTypeAll     TimeRangeType = "all"
)

// LogStatus represents the status of a notification log
type LogStatus string

const (
	LogStatusPending LogStatus = "pending"
	LogStatusSent    LogStatus = "sent"
	LogStatusFailed  LogStatus = "failed"
)

// Channel is the interface that all notification channels must implement
type Channel interface {
	// GetType returns the type of this channel
	GetType() ChannelType

	// Send sends a notification through this channel
	Send(event *Event) error

	// ValidateConfig validates the channel configuration
	ValidateConfig() error

	// IsActive checks if the channel is currently active (based on time range etc.)
	IsActive() bool
}

// Manager is the interface for the notification manager
type Manager interface {
	// Start starts the notification manager
	Start() error

	// Stop stops the notification manager
	Stop() error

	// RegisterChannel registers a notification channel
	RegisterChannel(channel Channel) error

	// UnregisterChannel unregisters a notification channel
	UnregisterChannel(channelType ChannelType, channelID uint) error

	// Send sends a notification event
	Send(event *Event) error

	// SendAsync sends a notification event asynchronously
	SendAsync(event *Event)

	// GetChannels returns all registered channels
	GetChannels() []Channel
}
