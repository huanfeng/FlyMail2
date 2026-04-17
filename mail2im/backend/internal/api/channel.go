package api

import (
	"encoding/json"
	"mail2im/internal/core"
	"mail2im/internal/dispatcher"
	"mail2im/internal/dispatcher/channels"
	"mail2im/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetChannels(c *gin.Context) {
	var channels []models.Channel
	if err := core.DB.Find(&channels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, channels)
}

func CreateChannel(c *gin.Context) {
	var channel models.Channel
	if err := c.ShouldBindJSON(&channel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := core.DB.Create(&channel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	dispatcher.Instance.ReloadChannels()
	c.JSON(http.StatusOK, channel)
}

func UpdateChannel(c *gin.Context) {
	id := c.Param("id")
	var channel models.Channel
	if err := core.DB.First(&channel, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Channel not found"})
		return
	}

	if err := c.ShouldBindJSON(&channel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := core.DB.Save(&channel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	dispatcher.Instance.ReloadChannels()
	c.JSON(http.StatusOK, channel)
}

func DeleteChannel(c *gin.Context) {
	id := c.Param("id")
	if err := core.DB.Delete(&models.Channel{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	dispatcher.Instance.ReloadChannels()
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func TestChannel(c *gin.Context) {
	var input struct {
		Type      string `json:"type"`
		Config    string `json:"config"`
		EventType string `json:"event_type"` // "system", "email"
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var channel core.NotificationChannel
	var err error

	// Create a temporary channel instance for testing
	switch input.Type {
	case "telegram":
		var config struct {
			Token  string `json:"token"`
			ChatID string `json:"chat_id"`
		}
		if err := json.Unmarshal([]byte(input.Config), &config); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid config format"})
			return
		}
		channel = channels.NewTelegramChannelWithConfig(config.Token, config.ChatID, core.PriorityNormal, "")
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported channel type"})
		return
	}

	var event core.Event
	switch input.EventType {
	case "email":
		event = core.Event{
			Type:     core.EventEmailReceived,
			Priority: core.PriorityNormal,
			Source:   "Test",
			Payload: map[string]interface{}{
				"uid":         123,
				"email_id":    1,
				"message_id":  "test-msg-id",
				"subject":     "Test Email Subject",
				"from":        "Test Sender <test@example.com>",
				"mailbox":     "INBOX",
				"mailboxRaw":  "INBOX",
				"mail_type":   "primary",
				"account_id":  0,
				"received_at": core.NowInSystemTZ(),
			},
		}
	default: // system
		event = core.Event{
			Type:     core.EventSystemError,
			Priority: core.PriorityNormal,
			Source:   "Test",
			Payload:  "This is a test message from Mail2IM.",
		}
	}

	err = channel.Send(event)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
