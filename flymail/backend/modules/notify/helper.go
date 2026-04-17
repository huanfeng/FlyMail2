package notify

import (
	"fmt"
	"time"

	"flymail/modules/email/account"
	"flymail/modules/email/message"
)

// Global notify manager instance
var globalManager Manager

// SetGlobalManager sets the global notify manager
func SetGlobalManager(manager Manager) {
	globalManager = manager
}

// GetGlobalManager returns the global notify manager
func GetGlobalManager() Manager {
	return globalManager
}

// Helper functions for common notifications

// NotifyNewEmails sends a notification for new emails
func NotifyNewEmails(account *account.EmailAccount, emails []*message.Email) {
	if globalManager == nil || len(emails) == 0 {
		return
	}

	var title string
	var message string

	if len(emails) == 1 {
		email := emails[0]
		title = fmt.Sprintf("新邮件：%s", email.Subject)
		message = fmt.Sprintf("来自：%s", email.From)
	} else {
		title = fmt.Sprintf("收到 %d 封新邮件", len(emails))
		message = fmt.Sprintf("账户：%s", account.Email)
	}

	event := &Event{
		Type:      EventNewEmail,
		Severity:  SeverityInfo,
		Title:     title,
		Message:   message,
		Timestamp: time.Now(),
		UserID:    account.UserID,
		AccountID: account.AccountID,
		Data: map[string]interface{}{
			"account_email": account.Email,
			"email_count":   len(emails),
			"emails":        emails,
		},
	}

	globalManager.SendAsync(event)
}

// NotifyEmailSyncStart sends a notification when email sync starts
func NotifyEmailSyncStart(account *account.EmailAccount) {
	if globalManager == nil {
		return
	}

	event := &Event{
		Type:      EventEmailSyncStart,
		Severity:  SeverityInfo,
		Title:     "邮件同步开始",
		Message:   fmt.Sprintf("正在同步账户：%s", account.Email),
		Timestamp: time.Now(),
		UserID:    account.UserID,
		AccountID: account.AccountID,
		Data: map[string]interface{}{
			"account_email": account.Email,
		},
	}

	globalManager.SendAsync(event)
}

// NotifyEmailSyncDone sends a notification when email sync completes
func NotifyEmailSyncDone(account *account.EmailAccount, newCount, totalCount int) {
	if globalManager == nil {
		return
	}

	var message string
	if newCount > 0 {
		message = fmt.Sprintf("账户 %s 同步完成，新邮件：%d 封", account.Email, newCount)
	} else {
		message = fmt.Sprintf("账户 %s 同步完成，无新邮件", account.Email)
	}

	event := &Event{
		Type:      EventEmailSyncDone,
		Severity:  SeverityInfo,
		Title:     "邮件同步完成",
		Message:   message,
		Timestamp: time.Now(),
		UserID:    account.UserID,
		AccountID: account.AccountID,
		Data: map[string]interface{}{
			"account_email": account.Email,
			"new_count":     newCount,
			"total_count":   totalCount,
		},
	}

	globalManager.SendAsync(event)
}

// NotifyEmailSyncFailed sends a notification when email sync fails
func NotifyEmailSyncFailed(account *account.EmailAccount, err error) {
	if globalManager == nil {
		return
	}

	event := &Event{
		Type:      EventEmailSyncFail,
		Severity:  SeverityError,
		Title:     "邮件同步失败",
		Message:   fmt.Sprintf("账户 %s 同步失败：%s", account.Email, err.Error()),
		Timestamp: time.Now(),
		UserID:    account.UserID,
		AccountID: account.AccountID,
		Data: map[string]interface{}{
			"account_email": account.Email,
			"error":         err.Error(),
		},
	}

	globalManager.SendAsync(event)
}

// NotifyAccountError sends a notification for account errors
func NotifyAccountError(account *account.EmailAccount, err error) {
	if globalManager == nil {
		return
	}

	event := &Event{
		Type:      EventAccountError,
		Severity:  SeverityError,
		Title:     "账户错误",
		Message:   fmt.Sprintf("账户 %s 出现错误：%s", account.Email, err.Error()),
		Timestamp: time.Now(),
		UserID:    account.UserID,
		AccountID: account.AccountID,
		Data: map[string]interface{}{
			"account_email": account.Email,
			"error":         err.Error(),
		},
	}

	globalManager.SendAsync(event)
}

// NotifySystemError sends a system error notification
func NotifySystemError(title, message string, err error) {
	if globalManager == nil {
		return
	}

	data := map[string]interface{}{
		"message": message,
	}
	if err != nil {
		data["error"] = err.Error()
	}

	event := &Event{
		Type:      EventSystemError,
		Severity:  SeverityError,
		Title:     title,
		Message:   message,
		Timestamp: time.Now(),
		Data:      data,
	}

	globalManager.SendAsync(event)
}

// NotifySystemWarning sends a system warning notification
func NotifySystemWarning(title, message string) {
	if globalManager == nil {
		return
	}

	event := &Event{
		Type:      EventSystemWarning,
		Severity:  SeverityWarning,
		Title:     title,
		Message:   message,
		Timestamp: time.Now(),
	}

	globalManager.SendAsync(event)
}

// NotifyTaskEvent sends a task-related notification
func NotifyTaskEvent(taskType string, taskID string, eventType EventType, title, message string, userID uint) {
	if globalManager == nil {
		return
	}

	severity := SeverityInfo
	if eventType == EventTaskFailed {
		severity = SeverityError
	}

	event := &Event{
		Type:      eventType,
		Severity:  severity,
		Title:     title,
		Message:   message,
		Timestamp: time.Now(),
		UserID:    userID,
		Data: map[string]interface{}{
			"task_type": taskType,
			"task_id":   taskID,
		},
	}

	globalManager.SendAsync(event)
}
