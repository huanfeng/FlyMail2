package task

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"flymail/pkg/i18n"
	"flymail/pkg/response"
)

// HTTPHandler handles HTTP requests for task management
type HTTPHandler struct {
	manager Manager
}

// NewHTTPHandler creates a new task HTTP handler
func NewHTTPHandler(manager Manager) *HTTPHandler {
	return &HTTPHandler{
		manager: manager,
	}
}

// CreateTask creates a new task
func (h *HTTPHandler) CreateTask(c *gin.Context) {
	var config Config
	if err := c.ShouldBindJSON(&config); err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	// Set user ID from context
	config.UserID = c.GetUint("userID")

	if err := h.manager.CreateTask(c.Request.Context(), &config); err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgSuccess, config)
}

// UpdateTask updates an existing task
func (h *HTTPHandler) UpdateTask(c *gin.Context) {
	taskID := c.Param("id")

	var config Config
	if err := c.ShouldBindJSON(&config); err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	config.TaskID = taskID
	config.UserID = c.GetUint("userID")

	if err := h.manager.UpdateTask(c.Request.Context(), &config); err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgSuccess, nil)
}

// DeleteTask deletes a task
func (h *HTTPHandler) DeleteTask(c *gin.Context) {
	taskID := c.Param("id")

	if err := h.manager.DeleteTask(c.Request.Context(), taskID); err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgSuccess, nil)
}

// GetTask gets a task by ID
func (h *HTTPHandler) GetTask(c *gin.Context) {
	taskID := c.Param("id")

	task, err := h.manager.GetTask(c.Request.Context(), taskID)
	if err != nil {
		response.NotFound(c, i18n.MsgNotFound, err)
		return
	}

	response.Success(c, i18n.MsgSuccess, task)
}

// ListTasks lists all tasks
func (h *HTTPHandler) ListTasks(c *gin.Context) {
	tasks, err := h.manager.ListTasks(c.Request.Context())
	if err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgSuccess, tasks)
}

// ExecuteTask manually executes a task
func (h *HTTPHandler) ExecuteTask(c *gin.Context) {
	taskID := c.Param("id")

	if err := h.manager.ExecuteTask(c.Request.Context(), taskID); err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgSuccess, gin.H{
		"message": "Task execution started",
	})
}

// GetTaskLogs gets task execution logs
func (h *HTTPHandler) GetTaskLogs(c *gin.Context) {
	taskID := c.Param("id")
	limit := 20

	// Parse limit from query
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	logs, err := h.manager.GetTaskLogs(c.Request.Context(), taskID, limit)
	if err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgSuccess, logs)
}

// GetStats gets task manager statistics
func (h *HTTPHandler) GetStats(c *gin.Context) {
	stats := h.manager.GetStats()
	response.Success(c, i18n.MsgSuccess, stats)
}

// SubscribeEvents subscribes to task events via SSE
func (h *HTTPHandler) SubscribeEvents(c *gin.Context) {
	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	// Subscribe to events
	eventCh := h.manager.Subscribe()
	defer h.manager.Unsubscribe(eventCh)

	// Create client close notifier
	clientGone := c.Request.Context().Done()

	// Send events
	for {
		select {
		case event := <-eventCh:
			c.SSEvent("task-event", event)
			c.Writer.Flush()
		case <-clientGone:
			return
		}
	}
}
