package sync

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"flymail/pkg/i18n"
	"flymail/pkg/response"
)

// Handler handles HTTP requests for email sync operations
type Handler struct {
	service Service
}

// NewHandler creates a new email sync handler
func NewHandler(service Service) *Handler {
	return &Handler{
		service: service,
	}
}

// GetStatus returns the monitoring status of all accounts
func (h *Handler) GetStatus(c *gin.Context) {
	status := h.service.GetStatus()

	// Convert to response format
	var statusList []AccountStatus
	for accountID, monitorStatus := range status {
		statusList = append(statusList, AccountStatus{
			AccountID: accountID,
			Status:    monitorStatus,
		})
	}

	response.Success(c, i18n.MsgSuccess, gin.H{
		"monitors": statusList,
		"total":    len(statusList),
	})
}

// GetAccountStatus returns the monitoring status of a specific account
func (h *Handler) GetAccountStatus(c *gin.Context) {
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	status, err := h.service.GetAccountStatus(uint(accountID))
	if err != nil {
		response.NotFound(c, "Monitor not found", err)
		return
	}

	response.Success(c, i18n.MsgSuccess, status)
}

// StartAccountMonitor starts monitoring for a specific account
func (h *Handler) StartAccountMonitor(c *gin.Context) {
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	// Restart monitoring (this will fetch the latest account info)
	if err := h.service.RestartMonitoringAccount(uint(accountID)); err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, "Monitor started successfully", nil)
}

// StopAccountMonitor stops monitoring for a specific account
func (h *Handler) StopAccountMonitor(c *gin.Context) {
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	if err := h.service.StopMonitoringAccount(uint(accountID)); err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, "Monitor stopped successfully", nil)
}

// SyncAccount manually triggers sync for a specific account
func (h *Handler) SyncAccount(c *gin.Context) {
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	result, err := h.service.SyncAccount(c.Request.Context(), uint(accountID))
	if err != nil {
		response.InternalError(c, i18n.MsgEmailSyncFailed, err)
		return
	}

	response.Success(c, i18n.MsgEmailSyncSuccess, result)
}

// SyncAllAccounts manually triggers sync for all accounts
func (h *Handler) SyncAllAccounts(c *gin.Context) {
	userID := c.GetUint("userID")

	results, err := h.service.SyncAllAccounts(c.Request.Context())
	if err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgEmailSyncSuccess, gin.H{
		"user_id": userID,
		"results": results,
	})
}

// GetConfig returns the current sync configuration
func (h *Handler) GetConfig(c *gin.Context) {
	config := h.service.GetConfig()
	response.Success(c, i18n.MsgSuccess, config)
}

// UpdateConfig updates the sync configuration
func (h *Handler) UpdateConfig(c *gin.Context) {
	var config Config
	if err := c.ShouldBindJSON(&config); err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	// Validate configuration
	if config.DayTimeStart < 0 || config.DayTimeStart > 23 {
		response.ValidationError(c, "day_time_start", "must be between 0 and 23")
		return
	}
	if config.DayTimeEnd < 0 || config.DayTimeEnd > 23 {
		response.ValidationError(c, "day_time_end", "must be between 0 and 23")
		return
	}
	if config.MaxRetries < 1 {
		response.ValidationError(c, "max_retries", "must be at least 1")
		return
	}

	h.service.SetConfig(&config)
	response.Success(c, "Configuration updated successfully", nil)
}
