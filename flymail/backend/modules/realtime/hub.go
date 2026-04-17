package realtime

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// hub implements the Hub interface
type hub struct {
	clients       map[string]Client
	clientsByUser map[uint]map[string]Client
	clientsMu     sync.RWMutex

	register   chan Client
	unregister chan Client
	broadcast  chan broadcastMessage

	subscriptionManager SubscriptionManager

	running   bool
	runningMu sync.RWMutex
	stopCh    chan struct{}
	wg        sync.WaitGroup

	totalMessages int64
	startTime     time.Time

	logger *zap.Logger
}

// broadcastMessage represents a message to broadcast
type broadcastMessage struct {
	topic string
	event *Event
}

// NewHub creates a new SSE hub
func NewHub(logger *zap.Logger) Hub {
	return &hub{
		clients:             make(map[string]Client),
		clientsByUser:       make(map[uint]map[string]Client),
		register:            make(chan Client, 100),
		unregister:          make(chan Client, 100),
		broadcast:           make(chan broadcastMessage, 1000),
		stopCh:              make(chan struct{}),
		logger:              logger,
		subscriptionManager: NewSubscriptionManager(),
	}
}

// Start starts the hub
func (h *hub) Start() error {
	h.runningMu.Lock()
	if h.running {
		h.runningMu.Unlock()
		return fmt.Errorf("hub already running")
	}
	h.running = true
	h.startTime = time.Now()
	h.runningMu.Unlock()

	h.wg.Add(1)
	go h.run()

	h.logger.Info("SSE hub started")
	return nil
}

// Stop stops the hub
func (h *hub) Stop() error {
	h.runningMu.Lock()
	if !h.running {
		h.runningMu.Unlock()
		return fmt.Errorf("hub not running")
	}
	h.running = false
	h.runningMu.Unlock()

	close(h.stopCh)
	h.wg.Wait()

	// Close all client connections
	h.clientsMu.Lock()
	for _, client := range h.clients {
		client.Close()
	}
	h.clientsMu.Unlock()

	h.logger.Info("SSE hub stopped")
	return nil
}

// RegisterClient registers a new client
func (h *hub) RegisterClient(client Client) {
	h.runningMu.RLock()
	if !h.running {
		h.runningMu.RUnlock()
		h.logger.Warn("Cannot register client, hub not running")
		return
	}
	h.runningMu.RUnlock()

	select {
	case h.register <- client:
	default:
		h.logger.Warn("Register channel full, dropping client")
	}
}

// UnregisterClient unregisters a client
func (h *hub) UnregisterClient(client Client) {
	select {
	case h.unregister <- client:
	default:
		// If channel is full, directly remove the client
		h.removeClient(client)
	}
}

// Broadcast sends an event to all clients subscribed to the topic
func (h *hub) Broadcast(topic string, event *Event) {
	h.runningMu.RLock()
	if !h.running {
		h.runningMu.RUnlock()
		return
	}
	h.runningMu.RUnlock()

	// Set topic in event
	if event.Topic == "" {
		event.Topic = topic
	}

	select {
	case h.broadcast <- broadcastMessage{topic: topic, event: event}:
		atomic.AddInt64(&h.totalMessages, 1)
	default:
		h.logger.Warn("Broadcast channel full, dropping message",
			zap.String("topic", topic))
	}
}

// BroadcastToUser sends an event to all clients of a specific user
func (h *hub) BroadcastToUser(userID uint, event *Event) {
	topic := fmt.Sprintf("%s%d", TopicUserPrefix, userID)
	h.Broadcast(topic, event)
}

// GetClient returns a client by ID
func (h *hub) GetClient(clientID string) Client {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()
	return h.clients[clientID]
}

// GetClients returns all active clients
func (h *hub) GetClients() []Client {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	clients := make([]Client, 0, len(h.clients))
	for _, client := range h.clients {
		if client.IsActive() {
			clients = append(clients, client)
		}
	}
	return clients
}

// GetClientsByUser returns all clients for a specific user
func (h *hub) GetClientsByUser(userID uint) []Client {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	userClients := h.clientsByUser[userID]
	if userClients == nil {
		return nil
	}

	clients := make([]Client, 0, len(userClients))
	for _, client := range userClients {
		if client.IsActive() {
			clients = append(clients, client)
		}
	}
	return clients
}

// GetConnectionCount returns the number of active connections
func (h *hub) GetConnectionCount() int {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	count := 0
	for _, client := range h.clients {
		if client.IsActive() {
			count++
		}
	}
	return count
}

// GetStats returns hub statistics
func (h *hub) GetStats() Stats {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	stats := Stats{
		TotalConnections:   len(h.clients),
		ActiveConnections:  0,
		TotalMessages:      atomic.LoadInt64(&h.totalMessages),
		ConnectionsByUser:  make(map[uint]int),
		ConnectionsByTopic: make(map[string]int),
		StartTime:          h.startTime,
		Uptime:             time.Since(h.startTime),
	}

	// Count active connections and group by user/topic
	for _, client := range h.clients {
		if client.IsActive() {
			stats.ActiveConnections++
			stats.ConnectionsByUser[client.GetUserID()]++

			for _, topic := range client.GetTopics() {
				stats.ConnectionsByTopic[topic]++
			}
		}
	}

	return stats
}

// run is the main hub loop
func (h *hub) run() {
	defer h.wg.Done()

	// Start cleanup ticker
	cleanupTicker := time.NewTicker(1 * time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-h.stopCh:
			return

		case client := <-h.register:
			h.addClient(client)

		case client := <-h.unregister:
			h.removeClient(client)

		case msg := <-h.broadcast:
			h.broadcastMessage(msg)

		case <-cleanupTicker.C:
			h.cleanupInactiveClients()
		}
	}
}

// addClient adds a client to the hub
func (h *hub) addClient(client Client) {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()

	clientID := client.GetID()
	userID := client.GetUserID()

	// Add to main map
	h.clients[clientID] = client

	// Add to user map
	if h.clientsByUser[userID] == nil {
		h.clientsByUser[userID] = make(map[string]Client)
	}
	h.clientsByUser[userID][clientID] = client

	h.logger.Info("Client connected",
		zap.String("client_id", clientID),
		zap.Uint("user_id", userID))
}

// removeClient removes a client from the hub
func (h *hub) removeClient(client Client) {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()

	clientID := client.GetID()
	userID := client.GetUserID()

	// Remove from main map
	delete(h.clients, clientID)

	// Remove from user map
	if userClients := h.clientsByUser[userID]; userClients != nil {
		delete(userClients, clientID)
		if len(userClients) == 0 {
			delete(h.clientsByUser, userID)
		}
	}

	// Clear subscriptions
	h.subscriptionManager.Clear(clientID)

	// Close client connection
	client.Close()

	h.logger.Info("Client disconnected",
		zap.String("client_id", clientID),
		zap.Uint("user_id", userID))
}

// broadcastMessage broadcasts a message to subscribed clients
func (h *hub) broadcastMessage(msg broadcastMessage) {
	h.clientsMu.RLock()
	defer h.clientsMu.RUnlock()

	sentCount := 0
	for _, client := range h.clients {
		// Check if client is subscribed to this topic
		subscribed := false
		for _, topic := range client.GetTopics() {
			if topic == msg.topic || topic == TopicAll {
				subscribed = true
				break
			}
		}

		if !subscribed {
			continue
		}

		// Check with subscription manager for additional filters
		if !h.subscriptionManager.ShouldReceive(client.GetID(), msg.topic, msg.event) {
			continue
		}

		// Send event to client
		if err := client.Send(msg.event); err != nil {
			h.logger.Debug("Failed to send event to client",
				zap.String("client_id", client.GetID()),
				zap.Error(err))
			// Mark client for removal
			go h.UnregisterClient(client)
		} else {
			sentCount++
		}
	}

	if sentCount > 0 {
		h.logger.Debug("Broadcast event",
			zap.String("topic", msg.topic),
			zap.String("event_type", string(msg.event.Type)),
			zap.Int("recipients", sentCount))
	}
}

// cleanupInactiveClients removes inactive clients
func (h *hub) cleanupInactiveClients() {
	h.clientsMu.RLock()
	inactiveClients := make([]Client, 0)

	for _, client := range h.clients {
		if !client.IsActive() {
			inactiveClients = append(inactiveClients, client)
		} else if sseClient, ok := client.(*sseClient); ok {
			// Check last ping time for SSE clients
			if time.Since(sseClient.GetLastPing()) > 2*time.Minute {
				inactiveClients = append(inactiveClients, client)
			}
		}
	}
	h.clientsMu.RUnlock()

	// Remove inactive clients
	for _, client := range inactiveClients {
		h.UnregisterClient(client)
	}

	if len(inactiveClients) > 0 {
		h.logger.Info("Cleaned up inactive clients",
			zap.Int("count", len(inactiveClients)))
	}
}
