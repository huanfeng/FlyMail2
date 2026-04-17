package api

import (
	"errors"
	"fmt"
	"mail2im/internal/core"
	"mail2im/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetLogs(c *gin.Context) {
	var logs []models.ForwardLog
	// Get last 50 logs, ordered by time desc
	result := core.DB.Order("created_at desc").Limit(50).Find(&logs)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	c.JSON(http.StatusOK, logs)
}

func DeleteLog(c *gin.Context) {
	id := c.Param("id")
	var logEntry models.ForwardLog
	if err := core.DB.First(&logEntry, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Log not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := core.DB.Delete(&logEntry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	core.RecordSystemLog("log_delete", "success", fmt.Sprintf("Deleted log #%d", logEntry.ID), fmt.Sprintf("channel=%s status=%s", logEntry.ChannelName, logEntry.Status))
	c.JSON(http.StatusOK, gin.H{"message": "Deleted"})
}

func DeleteAllLogs(c *gin.Context) {
	result := core.DB.Where("1 = 1").Delete(&models.ForwardLog{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	core.RecordSystemLog("log_delete_all", "success", "Cleared logs", fmt.Sprintf("deleted %d logs", result.RowsAffected))
	c.JSON(http.StatusOK, gin.H{"message": "Cleared"})
}
