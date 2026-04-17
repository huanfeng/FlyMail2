package realtime

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler handles SSE connections
type Handler struct {
	hub    Hub
	logger *zap.Logger
}

// NewHandler creates a new SSE handler
func NewHandler(hub Hub, logger *zap.Logger) *Handler {
	return &Handler{
		hub:    hub,
		logger: logger,
	}
}

// SSEMiddleware is a middleware that allows SSE connections with special auth
func SSEMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// For SSE endpoints, check if token is in query parameter
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/events") {
			token := c.Query("token")
			if token != "" {
				// Set the Authorization header from query parameter
				c.Request.Header.Set("Authorization", "Bearer "+token)
			}
		}
		c.Next()
	}
}

// HandleSSE handles SSE connections
func (h *Handler) HandleSSE(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID := c.GetUint("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	// Create new client
	client, err := NewClient(userID, c.Writer)
	if err != nil {
		h.logger.Error("Failed to create SSE client",
			zap.Uint("user_id", userID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to establish SSE connection"})
		return
	}

	// Parse subscription preferences from query parameters
	subscription := h.parseSubscription(c)
	if subscription != nil {
		h.hub.(*hub).subscriptionManager.Subscribe(client.GetID(), subscription)
	}

	// Register client with hub
	h.hub.RegisterClient(client)

	// Keep the connection open
	<-c.Request.Context().Done()

	// Unregister client when connection closes
	h.hub.UnregisterClient(client)
}

// GetStats returns SSE hub statistics
func (h *Handler) GetStats(c *gin.Context) {
	stats := h.hub.GetStats()
	c.JSON(http.StatusOK, stats)
}

// GetConnections returns active SSE connections
func (h *Handler) GetConnections(c *gin.Context) {
	// Check if user is admin
	isAdmin := c.GetBool("is_admin")
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
		return
	}

	clients := h.hub.GetClients()
	connections := make([]map[string]interface{}, 0, len(clients))

	for _, client := range clients {
		connections = append(connections, map[string]interface{}{
			"client_id": client.GetID(),
			"user_id":   client.GetUserID(),
			"topics":    client.GetTopics(),
			"active":    client.IsActive(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"total":       len(connections),
		"connections": connections,
	})
}

// Broadcast sends a broadcast message (admin only)
func (h *Handler) Broadcast(c *gin.Context) {
	// Check if user is admin
	isAdmin := c.GetBool("is_admin")
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
		return
	}

	var req struct {
		Topic   string                 `json:"topic" binding:"required"`
		Message string                 `json:"message" binding:"required"`
		Data    map[string]interface{} `json:"data"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create event
	event := &Event{
		Type:  EventTypeMessage,
		Topic: req.Topic,
		Data: map[string]interface{}{
			"message": req.Message,
		},
	}

	// Merge additional data
	for k, v := range req.Data {
		event.Data[k] = v
	}

	// Broadcast event
	h.hub.Broadcast(req.Topic, event)

	c.JSON(http.StatusOK, gin.H{
		"message": "Broadcast sent",
		"topic":   req.Topic,
	})
}

// parseSubscription parses subscription preferences from query parameters
func (h *Handler) parseSubscription(c *gin.Context) *Subscription {
	sub := &Subscription{
		Topics:     []string{},
		EventTypes: []string{},
		Filters:    make(map[string]string),
	}

	// Parse topics
	if topics := c.Query("topics"); topics != "" {
		sub.Topics = strings.Split(topics, ",")
		for i := range sub.Topics {
			sub.Topics[i] = strings.TrimSpace(sub.Topics[i])
		}
	}

	// Parse event types
	if eventTypes := c.Query("event_types"); eventTypes != "" {
		sub.EventTypes = strings.Split(eventTypes, ",")
		for i := range sub.EventTypes {
			sub.EventTypes[i] = strings.TrimSpace(sub.EventTypes[i])
		}
	}

	// Parse filters
	for key, values := range c.Request.URL.Query() {
		if strings.HasPrefix(key, "filter_") {
			filterKey := strings.TrimPrefix(key, "filter_")
			if len(values) > 0 {
				sub.Filters[filterKey] = values[0]
			}
		}
	}

	// Subscribe to additional account topics if specified
	if accountIDs := c.Query("account_ids"); accountIDs != "" {
		ids := strings.Split(accountIDs, ",")
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id != "" {
				sub.Topics = append(sub.Topics, fmt.Sprintf("%s%s", TopicAccountPrefix, id))
			}
		}
	}

	// If no specific configuration, return nil
	if len(sub.Topics) == 0 && len(sub.EventTypes) == 0 && len(sub.Filters) == 0 {
		return nil
	}

	return sub
}
