package realtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// sseClient implements the Client interface
type sseClient struct {
	id        string
	userID    uint
	writer    http.ResponseWriter
	flusher   http.Flusher
	topics    map[string]bool
	topicsMu  sync.RWMutex
	sendCh    chan *Event
	closeCh   chan struct{}
	closeOnce sync.Once
	active    bool
	activeMu  sync.RWMutex
	lastPing  time.Time
	pingMu    sync.RWMutex
}

// NewClient creates a new SSE client
func NewClient(userID uint, w http.ResponseWriter) (Client, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported")
	}

	client := &sseClient{
		id:       uuid.New().String(),
		userID:   userID,
		writer:   w,
		flusher:  flusher,
		topics:   make(map[string]bool),
		sendCh:   make(chan *Event, 100),
		closeCh:  make(chan struct{}),
		active:   true,
		lastPing: time.Now(),
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable Nginx buffering

	// Subscribe to user's personal topic by default
	client.Subscribe(fmt.Sprintf("%s%d", TopicUserPrefix, userID))

	// Start the client's event loop
	go client.eventLoop()

	return client, nil
}

// GetID returns the client's unique ID
func (c *sseClient) GetID() string {
	return c.id
}

// GetUserID returns the user ID associated with this client
func (c *sseClient) GetUserID() uint {
	return c.userID
}

// GetTopics returns the topics this client is subscribed to
func (c *sseClient) GetTopics() []string {
	c.topicsMu.RLock()
	defer c.topicsMu.RUnlock()

	topics := make([]string, 0, len(c.topics))
	for topic := range c.topics {
		topics = append(topics, topic)
	}
	return topics
}

// Subscribe adds a topic subscription
func (c *sseClient) Subscribe(topic string) {
	c.topicsMu.Lock()
	defer c.topicsMu.Unlock()
	c.topics[topic] = true
}

// Unsubscribe removes a topic subscription
func (c *sseClient) Unsubscribe(topic string) {
	c.topicsMu.Lock()
	defer c.topicsMu.Unlock()
	delete(c.topics, topic)
}

// Send sends an event to this client
func (c *sseClient) Send(event *Event) error {
	c.activeMu.RLock()
	if !c.active {
		c.activeMu.RUnlock()
		return fmt.Errorf("client is not active")
	}
	c.activeMu.RUnlock()

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Try to send without blocking
	select {
	case c.sendCh <- event:
		return nil
	default:
		// Channel is full, client is too slow
		return fmt.Errorf("client send channel is full")
	}
}

// Close closes the client connection
func (c *sseClient) Close() {
	c.closeOnce.Do(func() {
		c.activeMu.Lock()
		c.active = false
		c.activeMu.Unlock()

		close(c.closeCh)
	})
}

// IsActive returns whether the client is still active
func (c *sseClient) IsActive() bool {
	c.activeMu.RLock()
	defer c.activeMu.RUnlock()
	return c.active
}

// UpdatePing updates the last ping time
func (c *sseClient) UpdatePing() {
	c.pingMu.Lock()
	defer c.pingMu.Unlock()
	c.lastPing = time.Now()
}

// GetLastPing returns the last ping time
func (c *sseClient) GetLastPing() time.Time {
	c.pingMu.RLock()
	defer c.pingMu.RUnlock()
	return c.lastPing
}

// eventLoop handles sending events to the client
func (c *sseClient) eventLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Send initial connection event
	c.writeEvent(&Event{
		Type: EventTypeMessage,
		Data: map[string]interface{}{
			"message":   "Connected to SSE",
			"client_id": c.id,
		},
		Timestamp: time.Now(),
	})

	for {
		select {
		case <-c.closeCh:
			return

		case event := <-c.sendCh:
			if err := c.writeEvent(event); err != nil {
				c.Close()
				return
			}

		case <-ticker.C:
			// Send heartbeat
			if err := c.writeHeartbeat(); err != nil {
				c.Close()
				return
			}
		}
	}
}

// writeEvent writes an event to the SSE stream
func (c *sseClient) writeEvent(event *Event) error {
	// Check if still active
	if !c.IsActive() {
		return fmt.Errorf("client is not active")
	}

	// Prepare event data
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// Write SSE format
	if event.ID != "" {
		fmt.Fprintf(c.writer, "id: %s\n", event.ID)
	}
	if event.Type != "" {
		fmt.Fprintf(c.writer, "event: %s\n", event.Type)
	}
	fmt.Fprintf(c.writer, "data: %s\n\n", data)

	// Flush the data
	c.flusher.Flush()

	return nil
}

// writeHeartbeat writes a heartbeat event
func (c *sseClient) writeHeartbeat() error {
	return c.writeEvent(&Event{
		Type:      EventTypeHeartbeat,
		Data:      map[string]interface{}{"ping": "pong"},
		Timestamp: time.Now(),
	})
}
