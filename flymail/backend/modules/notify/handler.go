package notify

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler handles HTTP requests for notification operations
type Handler struct {
	service Service
	logger  *zap.Logger
}

// NewHandler creates a new notification handler
func NewHandler(service Service, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// CreateChannelRequest represents a request to create a notification channel
type CreateChannelRequest struct {
	Name        string        `json:"name" binding:"required"`
	Type        ChannelType   `json:"type" binding:"required"`
	Config      ChannelConfig `json:"config" binding:"required"`
	Priority    int           `json:"priority"`
	Description string        `json:"description"`
	TimeRanges  []TimeRange   `json:"time_ranges,omitempty"`
	Events      []EventFilter `json:"events,omitempty"`
}

// UpdateChannelRequest represents a request to update a notification channel
type UpdateChannelRequest struct {
	Name        string        `json:"name"`
	Config      ChannelConfig `json:"config"`
	Status      ChannelStatus `json:"status"`
	Priority    int           `json:"priority"`
	Description string        `json:"description"`
}

// TimeRange represents a time range configuration
type TimeRange struct {
	Type      TimeRangeType `json:"type" binding:"required"`
	StartTime string        `json:"start_time" binding:"required"` // Format: "HH:MM"
	EndTime   string        `json:"end_time" binding:"required"`   // Format: "HH:MM"
	Timezone  string        `json:"timezone"`
}

// EventFilter represents an event subscription filter
type EventFilter struct {
	EventType EventType `json:"event_type" binding:"required"`
	Severity  Severity  `json:"severity,omitempty"`
}

// TestNotificationRequest represents a request to send a test notification
type TestNotificationRequest struct {
	Type     EventType              `json:"type"`
	Severity Severity               `json:"severity"`
	Title    string                 `json:"title"`
	Message  string                 `json:"message"`
	Data     map[string]interface{} `json:"data,omitempty"`
}

// CreateChannel creates a new notification channel
func (h *Handler) CreateChannel(c *gin.Context) {
	userID := c.GetUint("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	var req CreateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create channel model
	channel := &NotifyChannel{
		UserID:      userID,
		Name:        req.Name,
		Type:        req.Type,
		Config:      req.Config,
		Priority:    req.Priority,
		Description: req.Description,
	}

	// Convert time ranges
	for _, tr := range req.TimeRanges {
		channel.TimeRanges = append(channel.TimeRanges, NotifyChannelTimeRange{
			Type:      tr.Type,
			StartTime: tr.StartTime,
			EndTime:   tr.EndTime,
			Timezone:  tr.Timezone,
		})
	}

	// Convert event filters
	for _, ef := range req.Events {
		channel.Events = append(channel.Events, NotifyChannelEvent{
			EventType: ef.EventType,
			Severity:  ef.Severity,
		})
	}

	// Create channel
	if err := h.service.CreateChannel(c.Request.Context(), userID, channel); err != nil {
		h.logger.Error("Failed to create channel",
			zap.Uint("user_id", userID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建通知渠道失败"})
		return
	}

	c.JSON(http.StatusOK, channel)
}

// UpdateChannel updates a notification channel
func (h *Handler) UpdateChannel(c *gin.Context) {
	userID := c.GetUint("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	channelID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道ID"})
		return
	}

	var req UpdateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create update model
	channel := &NotifyChannel{
		Name:        req.Name,
		Config:      req.Config,
		Status:      req.Status,
		Priority:    req.Priority,
		Description: req.Description,
	}

	// Update channel
	if err := h.service.UpdateChannel(c.Request.Context(), userID, uint(channelID), channel); err != nil {
		h.logger.Error("Failed to update channel",
			zap.Uint("user_id", userID),
			zap.Uint64("channel_id", channelID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新通知渠道失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "通知渠道已更新"})
}

// DeleteChannel deletes a notification channel
func (h *Handler) DeleteChannel(c *gin.Context) {
	userID := c.GetUint("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	channelID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道ID"})
		return
	}

	// Delete channel
	if err := h.service.DeleteChannel(c.Request.Context(), userID, uint(channelID)); err != nil {
		h.logger.Error("Failed to delete channel",
			zap.Uint("user_id", userID),
			zap.Uint64("channel_id", channelID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除通知渠道失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "通知渠道已删除"})
}

// GetChannel gets a notification channel
func (h *Handler) GetChannel(c *gin.Context) {
	userID := c.GetUint("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	channelID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道ID"})
		return
	}

	// Get channel
	channel, err := h.service.GetChannel(c.Request.Context(), userID, uint(channelID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "渠道不存在"})
		return
	}

	c.JSON(http.StatusOK, channel)
}

// GetChannels gets all notification channels for a user
func (h *Handler) GetChannels(c *gin.Context) {
	userID := c.GetUint("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	// Get channels
	channels, err := h.service.GetUserChannels(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get channels",
			zap.Uint("user_id", userID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取通知渠道失败"})
		return
	}

	c.JSON(http.StatusOK, channels)
}

// TestChannel sends a test notification to a channel
func (h *Handler) TestChannel(c *gin.Context) {
	userID := c.GetUint("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	channelID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道ID"})
		return
	}

	// Test channel
	if err := h.service.TestChannel(c.Request.Context(), userID, uint(channelID)); err != nil {
		h.logger.Error("Failed to test channel",
			zap.Uint("user_id", userID),
			zap.Uint64("channel_id", channelID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "测试通知发送失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "测试通知已发送"})
}

// UpdateChannelEvents updates event subscriptions for a channel
func (h *Handler) UpdateChannelEvents(c *gin.Context) {
	userID := c.GetUint("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	channelID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的渠道ID"})
		return
	}

	var events []EventFilter
	if err := c.ShouldBindJSON(&events); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert to channel events
	channelEvents := make([]NotifyChannelEvent, len(events))
	for i, e := range events {
		channelEvents[i] = NotifyChannelEvent{
			EventType: e.EventType,
			Severity:  e.Severity,
		}
	}

	// Update events
	if err := h.service.UpdateChannelEvents(c.Request.Context(), userID, uint(channelID), channelEvents); err != nil {
		h.logger.Error("Failed to update channel events",
			zap.Uint("user_id", userID),
			zap.Uint64("channel_id", channelID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新事件订阅失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "事件订阅已更新"})
}

// GetLogs gets notification logs
func (h *Handler) GetLogs(c *gin.Context) {
	userID := c.GetUint("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	// Parse pagination
	limit := 20
	offset := 0
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Check if channel specific
	if channelID := c.Query("channel_id"); channelID != "" {
		if id, err := strconv.ParseUint(channelID, 10, 32); err == nil {
			logs, err := h.service.GetChannelLogs(c.Request.Context(), userID, uint(id), limit, offset)
			if err != nil {
				h.logger.Error("Failed to get channel logs",
					zap.Uint("user_id", userID),
					zap.Uint64("channel_id", id),
					zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "获取通知日志失败"})
				return
			}
			c.JSON(http.StatusOK, logs)
			return
		}
	}

	// Get all logs
	logs, err := h.service.GetLogs(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to get logs",
			zap.Uint("user_id", userID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取通知日志失败"})
		return
	}

	// Get total count
	total, _ := h.service.CountLogs(c.Request.Context(), userID)

	c.JSON(http.StatusOK, gin.H{
		"logs":   logs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// SendTestNotification sends a test notification
func (h *Handler) SendTestNotification(c *gin.Context) {
	userID := c.GetUint("user_id")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	var req TestNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set defaults
	if req.Type == "" {
		req.Type = EventSystemWarning
	}
	if req.Severity == "" {
		req.Severity = SeverityInfo
	}
	if req.Title == "" {
		req.Title = "测试通知"
	}
	if req.Message == "" {
		req.Message = "这是一条测试通知消息"
	}

	// Create event
	event := &Event{
		Type:     req.Type,
		Severity: req.Severity,
		Title:    req.Title,
		Message:  req.Message,
		UserID:   userID,
		Data:     req.Data,
	}

	// Send notification
	if err := h.service.Send(event); err != nil {
		h.logger.Error("Failed to send test notification",
			zap.Uint("user_id", userID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "发送测试通知失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "测试通知已发送"})
}
