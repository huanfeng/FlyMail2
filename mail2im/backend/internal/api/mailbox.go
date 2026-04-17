package api

import (
	"mail2im/internal/core"
	"mail2im/internal/models"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// GetMailboxes list all mailboxes for an account
func GetMailboxes(c *gin.Context) {
	accountIDStr := c.Param("id")
	accountID, err := strconv.ParseUint(accountIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	var mailboxes []models.Mailbox
	if err := core.DB.Where("account_id = ?", accountID).Find(&mailboxes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch mailboxes"})
		return
	}

	c.JSON(http.StatusOK, mailboxes)
}

type UpdateMailboxRequest struct {
	WatchMode string `json:"watch_mode"`
	Type      string `json:"type"`
}

// UpdateMailbox update watch mode and type
func UpdateMailbox(c *gin.Context) {
	mailboxIDStr := c.Param("mailbox_id")
	mailboxID, err := strconv.ParseUint(mailboxIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mailbox ID"})
		return
	}

	var req UpdateMailboxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var mailbox models.Mailbox
	if err := core.DB.First(&mailbox, mailboxID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Mailbox not found"})
		return
	}

	// Validate inputs
	if req.WatchMode != "" {
		if req.WatchMode != "idle" && req.WatchMode != "poll" && req.WatchMode != "none" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid watch_mode (idle, poll, none)"})
			return
		}
		mailbox.WatchMode = req.WatchMode
	}

	if req.Type != "" {
		mailbox.Type = req.Type
	}

	if err := core.DB.Save(&mailbox).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update mailbox"})
		return
	}

	// Restart worker to apply changes
	core.Watcher.RestartWorker(mailbox.AccountID)

	c.JSON(http.StatusOK, mailbox)
}

// SyncMailboxes force fetch folders from remote
func SyncMailboxes(c *gin.Context) {
	accountIDStr := c.Param("id")
	accountID, err := strconv.ParseUint(accountIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	// We can trigger sync by restarting the worker, or just let the worker do it on startup.
	// For now, we trigger a restart which does a sync on connect.
	core.Watcher.RestartWorker(uint(accountID))

	// Wait a bit for sync to happen (naive)
	time.Sleep(2 * time.Second)

	var mailboxes []models.Mailbox
	if err := core.DB.Where("account_id = ?", accountID).Find(&mailboxes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch mailboxes"})
		return
	}

	c.JSON(http.StatusOK, mailboxes)
}
