package realtime

import (
	"time"
)

// EventType represents the type of SSE event
type EventType string

const (
	EventTypeMessage   EventType = "message"
	EventTypePing      EventType = "ping"
	EventTypeHeartbeat EventType = "heartbeat"
)

// Event represents an SSE event
type Event struct {
	ID        string                 `json:"id,omitempty"`
	Type      EventType              `json:"type"`
	Topic     string                 `json:"topic,omitempty"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// Topic represents a subscription topic
type Topic string

const (
	// User-specific topics (format: "user:{user_id}")
	TopicUserPrefix = "user:"

	// Account-specific topics (format: "account:{account_id}")
	TopicAccountPrefix = "account:"

	// Global topics
	TopicSystem = "system"
	TopicAll    = "all"
)

// ClientStatus represents the status of a client connection
type ClientStatus string

const (
	ClientStatusActive       ClientStatus = "active"
	ClientStatusDisconnected ClientStatus = "disconnected"
)

// Client represents an SSE client connection
type Client interface {
	// GetID returns the client's unique ID
	GetID() string

	// GetUserID returns the user ID associated with this client
	GetUserID() uint

	// GetTopics returns the topics this client is subscribed to
	GetTopics() []string

	// Subscribe adds a topic subscription
	Subscribe(topic string)

	// Unsubscribe removes a topic subscription
	Unsubscribe(topic string)

	// Send sends an event to this client
	Send(event *Event) error

	// Close closes the client connection
	Close()

	// IsActive returns whether the client is still active
	IsActive() bool
}

// Hub manages SSE client connections and event broadcasting
type Hub interface {
	// Start starts the hub
	Start() error

	// Stop stops the hub
	Stop() error

	// RegisterClient registers a new client
	RegisterClient(client Client)

	// UnregisterClient unregisters a client
	UnregisterClient(client Client)

	// Broadcast sends an event to all clients subscribed to the topic
	Broadcast(topic string, event *Event)

	// BroadcastToUser sends an event to all clients of a specific user
	BroadcastToUser(userID uint, event *Event)

	// GetClient returns a client by ID
	GetClient(clientID string) Client

	// GetClients returns all active clients
	GetClients() []Client

	// GetClientsByUser returns all clients for a specific user
	GetClientsByUser(userID uint) []Client

	// GetConnectionCount returns the number of active connections
	GetConnectionCount() int

	// GetStats returns hub statistics
	GetStats() Stats
}

// Stats represents hub statistics
type Stats struct {
	TotalConnections   int            `json:"total_connections"`
	ActiveConnections  int            `json:"active_connections"`
	TotalMessages      int64          `json:"total_messages"`
	ConnectionsByUser  map[uint]int   `json:"connections_by_user"`
	ConnectionsByTopic map[string]int `json:"connections_by_topic"`
	StartTime          time.Time      `json:"start_time"`
	Uptime             time.Duration  `json:"uptime"`
}

// Subscription represents a client's subscription preferences
type Subscription struct {
	Topics     []string          `json:"topics"`
	EventTypes []string          `json:"event_types,omitempty"`
	Filters    map[string]string `json:"filters,omitempty"`
}

// SubscriptionManager manages client subscriptions
type SubscriptionManager interface {
	// Subscribe adds a subscription for a client
	Subscribe(clientID string, subscription *Subscription) error

	// Unsubscribe removes a subscription for a client
	Unsubscribe(clientID string, topic string) error

	// GetSubscriptions returns all subscriptions for a client
	GetSubscriptions(clientID string) (*Subscription, error)

	// ShouldReceive checks if a client should receive an event
	ShouldReceive(clientID string, topic string, event *Event) bool

	// Clear removes all subscriptions for a client
	Clear(clientID string)
}
