package setting

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"flymail/pkg/i18n"
	"flymail/pkg/logger"
	"flymail/pkg/response"
)

// Handler handles setting-related HTTP requests
type Handler struct {
	service      Service
	emailMonitor interface{} // Will be set to *monitor.EmailMonitor if available
}

// NewHandler creates a new setting handler
func NewHandler(service Service) *Handler {
	return &Handler{
		service: service,
	}
}

// SetEmailMonitor sets the email monitor instance for dynamic updates
func (h *Handler) SetEmailMonitor(monitor interface{}) {
	h.emailMonitor = monitor
}

// GetSetting retrieves a single setting by key
func (h *Handler) GetSetting(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		response.BadRequest(c, i18n.MsgInvalidRequest, nil)
		return
	}

	value, err := h.service.GetSetting(c.Request.Context(), key)
	if err != nil {
		logger.Error("Failed to get setting", zap.String("key", key), zap.Error(err))
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgSuccess, gin.H{
		"key":   key,
		"value": value,
	})
}

// GetAllSettings retrieves all settings
func (h *Handler) GetAllSettings(c *gin.Context) {
	settings, err := h.service.GetAllSettings(c.Request.Context())
	if err != nil {
		logger.Error("Failed to get all settings", zap.Error(err))
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgSuccess, gin.H{
		"settings": settings,
		"count":    len(settings),
	})
}

// UpdateSetting updates a single setting
func (h *Handler) UpdateSetting(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		response.BadRequest(c, i18n.MsgInvalidRequest, nil)
		return
	}

	var req struct {
		Value string `json:"value" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "value", err.Error())
		return
	}

	err := h.service.UpdateSetting(c.Request.Context(), key, req.Value)
	if err != nil {
		logger.Error("Failed to update setting", zap.String("key", key), zap.Error(err))
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgUpdateSuccess, gin.H{
		"key":   key,
		"value": req.Value,
	})
}

// DeleteSetting deletes a setting
func (h *Handler) DeleteSetting(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		response.BadRequest(c, i18n.MsgInvalidRequest, nil)
		return
	}

	err := h.service.DeleteSetting(c.Request.Context(), key)
	if err != nil {
		logger.Error("Failed to delete setting", zap.String("key", key), zap.Error(err))
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgDeleteSuccess, nil)
}

// UpdateMultipleSettings updates multiple settings at once
func (h *Handler) UpdateMultipleSettings(c *gin.Context) {
	var req struct {
		Settings map[string]string `json:"settings" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "settings", err.Error())
		return
	}

	if len(req.Settings) == 0 {
		response.BadRequest(c, i18n.MsgInvalidRequest, nil)
		return
	}

	err := h.service.UpdateSettings(c.Request.Context(), req.Settings)
	if err != nil {
		logger.Error("Failed to update settings", zap.Error(err))
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgUpdateSuccess, gin.H{
		"updated": len(req.Settings),
	})
}

// GetAppSettings retrieves common application settings
func (h *Handler) GetAppSettings(c *gin.Context) {
	settings, err := h.service.GetAppSettings(c.Request.Context())
	if err != nil {
		logger.Error("Failed to get app settings", zap.Error(err))
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgSuccess, settings)
}

// UpdateAppSettings updates common application settings
func (h *Handler) UpdateAppSettings(c *gin.Context) {
	var settings AppSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		response.ValidationError(c, "settings", err.Error())
		return
	}

	// Validate settings
	if err := validateAppSettings(&settings); err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	err := h.service.UpdateAppSettings(c.Request.Context(), &settings)
	if err != nil {
		logger.Error("Failed to update app settings", zap.Error(err))
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgUpdateSuccess, settings)
}

// validateAppSettings validates application settings
func validateAppSettings(settings *AppSettings) error {
	// Validate email settings
	if settings.EmailSettings.MaxEmailSize < 1024*1024 {
		settings.EmailSettings.MaxEmailSize = 1024 * 1024 // Minimum 1MB
	}
	if settings.EmailSettings.DefaultSyncLimit < 1 {
		settings.EmailSettings.DefaultSyncLimit = 1
	}

	// Validate security settings
	if settings.SecuritySettings.PasswordMinLength < 6 {
		settings.SecuritySettings.PasswordMinLength = 6
	}

	return nil
}

// GetEmailMonitorSettings retrieves email monitor settings
func (h *Handler) GetEmailMonitorSettings(c *gin.Context) {
	settings, err := h.service.GetEmailMonitorSettings(c.Request.Context())
	if err != nil {
		logger.Error("Failed to get email monitor settings", zap.Error(err))
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgSuccess, settings)
}

// UpdateEmailMonitorSettings updates email monitor settings
func (h *Handler) UpdateEmailMonitorSettings(c *gin.Context) {
	var settings EmailMonitorSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		response.ValidationError(c, "settings", err.Error())
		return
	}

	// Validate settings
	if err := validateEmailMonitorSettings(&settings); err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	err := h.service.UpdateEmailMonitorSettings(c.Request.Context(), &settings)
	if err != nil {
		logger.Error("Failed to update email monitor settings", zap.Error(err))
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgUpdateSuccess, settings)
}

// validateEmailMonitorSettings validates email monitor settings
func validateEmailMonitorSettings(settings *EmailMonitorSettings) error {
	// Validate time range
	if settings.DayTimeStart < 0 || settings.DayTimeStart > 23 {
		settings.DayTimeStart = 8
	}
	if settings.DayTimeEnd < 0 || settings.DayTimeEnd > 23 {
		settings.DayTimeEnd = 22
	}
	if settings.DayTimeStart >= settings.DayTimeEnd {
		settings.DayTimeStart = 8
		settings.DayTimeEnd = 22
	}

	// Validate intervals
	if settings.DayTimePollInterval == "" {
		settings.DayTimePollInterval = "1m"
	}
	if settings.NightTimePollInterval == "" {
		settings.NightTimePollInterval = "10m"
	}
	if settings.RetryInterval == "" {
		settings.RetryInterval = "30s"
	}

	// Validate max retries
	if settings.MaxRetries < 1 {
		settings.MaxRetries = 3
	}

	// Validate check interval
	if settings.CheckInterval < 60 {
		settings.CheckInterval = 300 // Default 5 minutes
	}

	return nil
}
