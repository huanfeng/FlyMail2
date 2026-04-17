package notify

import (
	"fmt"
	"time"
)

// BaseChannel provides common functionality for all channels
type BaseChannel struct {
	ID         uint
	UserID     uint
	Name       string
	Type       ChannelType
	Config     ChannelConfig
	Status     ChannelStatus
	TimeRanges []NotifyChannelTimeRange
	Events     []NotifyChannelEvent
}

// GetType returns the channel type
func (b *BaseChannel) GetType() ChannelType {
	return b.Type
}

// IsActive checks if the channel is currently active based on time ranges
func (b *BaseChannel) IsActive() bool {
	if b.Status != ChannelStatusActive {
		return false
	}

	// If no time ranges specified, always active
	if len(b.TimeRanges) == 0 {
		return true
	}

	now := time.Now()
	for _, tr := range b.TimeRanges {
		if isInTimeRange(now, tr) {
			return true
		}
	}

	return false
}

// ShouldNotify checks if this channel should notify for the given event
func (b *BaseChannel) ShouldNotify(event *Event) bool {
	if !b.IsActive() {
		return false
	}

	// If no event subscriptions specified, notify for all events
	if len(b.Events) == 0 {
		return true
	}

	// Check if this event type is subscribed
	for _, e := range b.Events {
		if e.EventType == event.Type {
			// If severity is specified, check it matches
			if e.Severity != "" && e.Severity != event.Severity {
				continue
			}
			return true
		}
	}

	return false
}

// Helper function to check if current time is in the time range
func isInTimeRange(now time.Time, tr NotifyChannelTimeRange) bool {
	// Parse timezone
	loc, err := time.LoadLocation(tr.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now = now.In(loc)

	// Check day type
	isWeekend := now.Weekday() == time.Saturday || now.Weekday() == time.Sunday
	switch tr.Type {
	case TimeRangeTypeWeekday:
		if isWeekend {
			return false
		}
	case TimeRangeTypeWeekend:
		if !isWeekend {
			return false
		}
	}

	// Parse start and end times
	startTime, err := time.Parse("15:04", tr.StartTime)
	if err != nil {
		return false
	}
	endTime, err := time.Parse("15:04", tr.EndTime)
	if err != nil {
		return false
	}

	// Create today's start and end times
	todayStart := time.Date(now.Year(), now.Month(), now.Day(),
		startTime.Hour(), startTime.Minute(), 0, 0, loc)
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(),
		endTime.Hour(), endTime.Minute(), 0, 0, loc)

	// Handle case where end time is before start time (spans midnight)
	if todayEnd.Before(todayStart) {
		todayEnd = todayEnd.Add(24 * time.Hour)
		if now.Before(todayStart) {
			now = now.Add(24 * time.Hour)
		}
	}

	return now.After(todayStart) && now.Before(todayEnd)
}

// GetConfigString gets a string value from config
func (b *BaseChannel) GetConfigString(key string) string {
	if val, ok := b.Config[key].(string); ok {
		return val
	}
	return ""
}

// GetConfigInt gets an int value from config
func (b *BaseChannel) GetConfigInt(key string) int {
	if val, ok := b.Config[key].(float64); ok {
		return int(val)
	}
	if val, ok := b.Config[key].(int); ok {
		return val
	}
	return 0
}

// GetConfigBool gets a bool value from config
func (b *BaseChannel) GetConfigBool(key string) bool {
	if val, ok := b.Config[key].(bool); ok {
		return val
	}
	return false
}

// ValidateRequiredConfigs validates that all required config keys are present
func (b *BaseChannel) ValidateRequiredConfigs(required []string) error {
	for _, key := range required {
		if b.GetConfigString(key) == "" {
			return fmt.Errorf("missing required config: %s", key)
		}
	}
	return nil
}
