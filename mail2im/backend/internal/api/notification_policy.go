package api

import (
	"encoding/json"
	"mail2im/internal/core"
	"mail2im/internal/dispatcher"
	"mail2im/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// PolicyItem represents one row in the notification policy view.
type PolicyItem struct {
	models.MailType
	Channels []PolicyChannel `json:"channels"` // available channels with selection state
}

// PolicyChannel is a channel with its selection state for a given mail type.
type PolicyChannel struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Selected bool   `json:"selected"`
}

// GetNotificationPolicy returns all mail types with their channel bindings.
func GetNotificationPolicy(c *gin.Context) {
	var mailTypes []models.MailType
	if err := core.DB.Order("priority desc, key asc").Find(&mailTypes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var dbChannels []models.Channel
	core.DB.Where("status = ?", "enabled").Find(&dbChannels)

	items := make([]PolicyItem, 0, len(mailTypes))
	for _, mt := range mailTypes {
		channelIDs := parseChannelIDs(mt.ChannelIDs)

		policyChannels := make([]PolicyChannel, 0, len(dbChannels))
		for _, ch := range dbChannels {
			policyChannels = append(policyChannels, PolicyChannel{
				ID:       ch.ID,
				Name:     ch.Name,
				Type:     ch.Type,
				Selected: channelIDs[ch.ID],
			})
		}

		items = append(items, PolicyItem{
			MailType: mt,
			Channels: policyChannels,
		})
	}

	c.JSON(http.StatusOK, items)
}

// UpdateNotificationPolicy updates a single mail type's channel_ids and action.
func UpdateNotificationPolicy(c *gin.Context) {
	key := c.Param("key")

	var mt models.MailType
	if err := core.DB.Where("key = ?", key).First(&mt).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "mail type not found"})
		return
	}

	var req struct {
		ChannelIDs string `json:"channel_ids"` // JSON array e.g. "[1,2]"
		Action     string `json:"action"`      // "notify" / "silent" / "ignore"
		Priority   *int   `json:"priority"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]any{}
	if req.ChannelIDs != "" {
		updates["channel_ids"] = req.ChannelIDs
	}
	if req.Action != "" {
		switch req.Action {
		case "notify", "silent", "ignore":
			updates["action"] = req.Action
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "action must be notify, silent, or ignore"})
			return
		}
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no updates provided"})
		return
	}

	if err := core.DB.Model(&mt).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Reload dispatcher channels so the new routing takes effect
	dispatcher.Instance.ReloadChannels()

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func parseChannelIDs(raw string) map[uint]bool {
	result := make(map[uint]bool)
	if raw == "" || raw == "[]" {
		return result
	}
	var ids []uint
	if err := json.Unmarshal([]byte(raw), &ids); err == nil {
		for _, id := range ids {
			result[id] = true
		}
	}
	return result
}
