package api

import (
	"encoding/json"
	"flymail-core/httputil"
	"mail2im/internal/core"
	"mail2im/internal/dispatcher"
	"mail2im/internal/dispatcher/channels"
	"mail2im/internal/models"

	"github.com/gin-gonic/gin"
)

func GetChannels(c *gin.Context) {
	var channels []models.Channel
	if err := core.DB.Find(&channels).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	c.JSON(200, channels)
}

func CreateChannel(c *gin.Context) {
	var channel models.Channel
	if err := c.ShouldBindJSON(&channel); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}

	if err := core.DB.Create(&channel).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}

	dispatcher.Instance.ReloadChannels()
	c.JSON(200, channel)
}

func UpdateChannel(c *gin.Context) {
	id := c.Param("id")
	var channel models.Channel
	if err := core.DB.First(&channel, id).Error; err != nil {
		httputil.NotFound(c, "Channel not found", nil)
		return
	}

	if err := c.ShouldBindJSON(&channel); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}

	if err := core.DB.Save(&channel).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}

	dispatcher.Instance.ReloadChannels()
	c.JSON(200, channel)
}

func DeleteChannel(c *gin.Context) {
	id := c.Param("id")
	if err := core.DB.Delete(&models.Channel{}, id).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}

	dispatcher.Instance.ReloadChannels()
	httputil.NoContent(c, "deleted")
}

func TestChannel(c *gin.Context) {
	var input struct {
		Type      string `json:"type"`
		Config    string `json:"config"`
		EventType string `json:"event_type"` // "system", "email"
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		httputil.BadRequest(c, err.Error(), err)
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
			httputil.BadRequest(c, "Invalid config format", nil)
			return
		}
		channel = channels.NewTelegramChannelWithConfig(config.Token, config.ChatID, core.PriorityNormal, "")
	default:
		httputil.BadRequest(c, "Unsupported channel type", nil)
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
		httputil.BadRequest(c, err.Error(), err)
		return
	}

	httputil.NoContent(c, "ok")
}
