package core

import (
	"time"

	"mail2im/internal/models"
)

// RecordForwardLog writes an event log for email receive/push actions.
func RecordForwardLog(accountID uint, action, status string, channelID *uint, channelName, messageID, subject, from string, receivedAt time.Time, priority int, request, response, errMsg string) {
	entry := models.ForwardLog{
		AccountID:   accountID,
		MessageID:   messageID,
		ChannelID:   channelID,
		ChannelName: channelName,
		Subject:     subject,
		From:        from,
		ReceivedAt:  receivedAt,
		ForwardedAt: time.Now(),
		Priority:    priority,
		Status:      status,
		Action:      action,
		Channel:     channelName,
		Request:     request,
		Response:    response,
		Error:       errMsg,
	}
	_ = DB.Create(&entry).Error
}

// RecordSystemLog writes an audit/system log entry for local operations (e.g., deletions).
func RecordSystemLog(action, status, subject, detail string) {
	now := time.Now()
	entry := models.ForwardLog{
		AccountID:   0,
		MessageID:   "",
		ChannelID:   nil,
		ChannelName: "system",
		From:        "system",
		Subject:     subject,
		ReceivedAt:  now,
		ForwardedAt: now,
		Priority:    int(PriorityLow),
		Status:      status,
		Action:      action,
		Channel:     "system",
		Response:    detail,
	}
	_ = DB.Create(&entry).Error
}
