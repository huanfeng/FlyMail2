package api

import (
	"flymail-core/httputil"
	"mail2im/internal/core"
	"mail2im/internal/models"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// GetMailboxes list all mailboxes for an account
func GetMailboxes(c *gin.Context) {
	accountIDStr := c.Param("id")
	accountID, err := strconv.ParseUint(accountIDStr, 10, 64)
	if err != nil {
		httputil.BadRequest(c, "Invalid account ID", nil)
		return
	}

	var mailboxes []models.Mailbox
	if err := core.DB.Where("account_id = ?", accountID).Find(&mailboxes).Error; err != nil {
		httputil.InternalError(c, "Failed to fetch mailboxes", nil)
		return
	}

	httputil.Success(c, mailboxes)
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
		httputil.BadRequest(c, "Invalid mailbox ID", nil)
		return
	}

	var req UpdateMailboxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.BadRequest(c, "Invalid request body", nil)
		return
	}

	var mailbox models.Mailbox
	if err := core.DB.First(&mailbox, mailboxID).Error; err != nil {
		httputil.NotFound(c, "Mailbox not found", nil)
		return
	}

	// Validate inputs
	if req.WatchMode != "" {
		if req.WatchMode != "idle" && req.WatchMode != "poll" && req.WatchMode != "none" {
			httputil.BadRequest(c, "Invalid watch_mode (idle, poll, none)", nil)
			return
		}
		mailbox.WatchMode = req.WatchMode
	}

	if req.Type != "" {
		mailbox.Type = req.Type
	}

	if err := core.DB.Save(&mailbox).Error; err != nil {
		httputil.InternalError(c, "Failed to update mailbox", nil)
		return
	}

	// Restart worker to apply changes
	core.Watcher.RestartWorker(mailbox.AccountID)

	httputil.Success(c, mailbox)
}

// SyncMailboxes force fetch folders from remote
func SyncMailboxes(c *gin.Context) {
	accountIDStr := c.Param("id")
	accountID, err := strconv.ParseUint(accountIDStr, 10, 64)
	if err != nil {
		httputil.BadRequest(c, "Invalid account ID", nil)
		return
	}

	// We can trigger sync by restarting the worker, or just let the worker do it on startup.
	// For now, we trigger a restart which does a sync on connect.
	core.Watcher.RestartWorker(uint(accountID))

	// Wait a bit for sync to happen (naive)
	time.Sleep(2 * time.Second)

	var mailboxes []models.Mailbox
	if err := core.DB.Where("account_id = ?", accountID).Find(&mailboxes).Error; err != nil {
		httputil.InternalError(c, "Failed to fetch mailboxes", nil)
		return
	}

	httputil.Success(c, mailboxes)
}

