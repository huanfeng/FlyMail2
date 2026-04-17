package notify

import (
	"fmt"

	"flymail/modules/realtime"
)

// SSEChannel implements the Channel interface for SSE notifications
type SSEChannel struct {
	hub realtime.Hub
}

// NewSSEChannel creates a new SSE notification channel
func NewSSEChannel(hub realtime.Hub) Channel {
	return &SSEChannel{
		hub: hub,
	}
}

// GetType returns the channel type
func (c *SSEChannel) GetType() ChannelType {
	return ChannelTypeSSE
}

// Send sends a notification through SSE
func (c *SSEChannel) Send(event *Event) error {
	if c.hub == nil {
		return fmt.Errorf("SSE hub not initialized")
	}

	// Convert notify event to SSE event
	sseEvent := &realtime.Event{
		Type: realtime.EventTypeMessage,
		Data: map[string]interface{}{
			"notify_type": string(event.Type),
			"severity":    string(event.Severity),
			"title":       event.Title,
			"message":     event.Message,
			"timestamp":   event.Timestamp,
			"data":        event.Data,
		},
		Timestamp: event.Timestamp,
	}

	// Add account_id if present
	if event.AccountID > 0 {
		sseEvent.Data["account_id"] = event.AccountID
	}

	// Determine broadcast topic based on event
	var topic string
	if event.UserID > 0 {
		// User-specific event
		topic = fmt.Sprintf("%s%d", realtime.TopicUserPrefix, event.UserID)
	} else {
		// System-wide event
		topic = realtime.TopicSystem
	}

	// Broadcast the event
	c.hub.Broadcast(topic, sseEvent)

	// Also broadcast to account-specific topic if applicable
	if event.AccountID > 0 {
		accountTopic := fmt.Sprintf("%s%d", realtime.TopicAccountPrefix, event.AccountID)
		c.hub.Broadcast(accountTopic, sseEvent)
	}

	return nil
}

// ValidateConfig validates the channel configuration
func (c *SSEChannel) ValidateConfig() error {
	if c.hub == nil {
		return fmt.Errorf("SSE hub not initialized")
	}
	return nil
}

// IsActive checks if the channel is currently active
func (c *SSEChannel) IsActive() bool {
	return c.hub != nil
}

// IntegrateSSEWithNotifyManager integrates SSE with the notification manager
func IntegrateSSEWithNotifyManager(notifyManager Manager, sseHub realtime.Hub) error {
	if notifyManager == nil {
		return fmt.Errorf("notify manager is nil")
	}
	if sseHub == nil {
		return fmt.Errorf("SSE hub is nil")
	}

	// Create SSE channel
	sseChannel := NewSSEChannel(sseHub)

	// Register with notify manager
	return notifyManager.RegisterChannel(sseChannel)
}
