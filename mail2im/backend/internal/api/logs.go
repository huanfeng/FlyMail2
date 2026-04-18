package api

import (
	"errors"
	"fmt"
	"flymail-core/httputil"
	"mail2im/internal/core"
	"mail2im/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetLogs(c *gin.Context) {
	var logs []models.ForwardLog
	// Get last 50 logs, ordered by time desc
	result := core.DB.Order("created_at desc").Limit(50).Find(&logs)
	if result.Error != nil {
		httputil.InternalError(c, result.Error.Error(), result.Error)
		return
	}
	httputil.Success(c, logs)
}

func DeleteLog(c *gin.Context) {
	id := c.Param("id")
	var logEntry models.ForwardLog
	if err := core.DB.First(&logEntry, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httputil.NotFound(c, "Log not found", nil)
			return
		}
		httputil.InternalError(c, err.Error(), err)
		return
	}

	if err := core.DB.Delete(&logEntry).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}

	core.RecordSystemLog("log_delete", "success", fmt.Sprintf("Deleted log #%d", logEntry.ID), fmt.Sprintf("channel=%s status=%s", logEntry.ChannelName, logEntry.Status))
	httputil.NoContent(c, "deleted")
}

func DeleteAllLogs(c *gin.Context) {
	result := core.DB.Where("1 = 1").Delete(&models.ForwardLog{})
	if result.Error != nil {
		httputil.InternalError(c, result.Error.Error(), result.Error)
		return
	}

	core.RecordSystemLog("log_delete_all", "success", "Cleared logs", fmt.Sprintf("deleted %d logs", result.RowsAffected))
	httputil.NoContent(c, "cleared")
}
