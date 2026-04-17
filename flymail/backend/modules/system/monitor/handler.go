package monitor

import (
	"github.com/gin-gonic/gin"

	"flymail/pkg/i18n"
	"flymail/pkg/response"
)

// Handler handles HTTP requests for system monitoring
type Handler struct {
	service Service
}

// NewHandler creates a new monitor handler
func NewHandler(service Service) *Handler {
	return &Handler{
		service: service,
	}
}

// GetSystemStatus returns system-level status
func (h *Handler) GetSystemStatus(c *gin.Context) {
	status, err := h.service.GetSystemStatus(c.Request.Context())
	if err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}
	response.Success(c, i18n.MsgSuccess, status)
}

// GetStatus returns combined monitoring status
func (h *Handler) GetStatus(c *gin.Context) {
	summary, err := h.service.GetMonitorSummary(c.Request.Context())
	if err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}
	response.Success(c, i18n.MsgSuccess, summary)
}

// GetHealth returns health check status
func (h *Handler) GetHealth(c *gin.Context) {
	healthStatus, err := h.service.GetHealthStatus(c.Request.Context())
	if err != nil {
		response.Error(c, response.CodeServiceUnavailable, i18n.MsgServiceUnavailable, err)
		return
	}

	// 如果服务不健康，返回503状态码
	if !healthStatus.Healthy {
		c.JSON(503, gin.H{
			"code":    response.CodeServiceUnavailable,
			"message": "服务不健康",
			"data":    healthStatus,
		})
		return
	}

	response.Success(c, i18n.MsgSuccess, healthStatus)
}

// GetRealtimeStatus returns realtime monitoring status
func (h *Handler) GetRealtimeStatus(c *gin.Context) {
	status, err := h.service.GetRealtimeStatus(c.Request.Context())
	if err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}
	response.Success(c, i18n.MsgSuccess, status)
}
