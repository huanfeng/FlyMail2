package i18n

// 英文翻译
var enUSTranslations = map[string]string{
	// 通用消息
	MsgSuccess:            "Operation successful",
	MsgOperationFailed:    "Operation failed",
	MsgInvalidRequest:     "Invalid request",
	MsgUnauthorized:       "Unauthorized",
	MsgForbidden:          "Forbidden",
	MsgNotFound:           "Resource not found",
	MsgInternalError:      "Internal server error",
	MsgServiceUnavailable: "Service unavailable",

	// 认证相关
	MsgLoginSuccess:       "Login successful",
	MsgLoginFailed:        "Login failed",
	MsgLogoutSuccess:      "Logout successful",
	MsgInvalidCredentials: "Invalid credentials",
	MsgTokenExpired:       "Token expired",
	MsgTokenInvalid:       "Invalid token",
	MsgTokenRefreshed:     "Token refreshed successfully",
	MsgPasswordChanged:    "Password changed successfully",
	MsgPasswordResetSent:  "Password reset link sent",

	// 用户相关
	MsgUserCreated:        "User created successfully",
	MsgUserUpdated:        "User updated successfully",
	MsgUserDeleted:        "User deleted successfully",
	MsgUserNotFound:       "User not found",
	MsgUserAlreadyExists:  "User already exists",
	MsgUserProfileUpdated: "Profile updated successfully",

	// 邮箱账户相关
	MsgAccountCreated:       "Account created successfully",
	MsgAccountUpdated:       "Account updated successfully",
	MsgAccountDeleted:       "Account deleted successfully",
	MsgAccountNotFound:      "Account not found",
	MsgAccountAlreadyExists: "Account already exists",
	MsgAccountTestSuccess:   "Connection test successful",
	MsgAccountTestFailed:    "Connection test failed",
	MsgAccountInactive:      "Account is inactive",

	// 邮件相关
	MsgEmailSent:         "Email sent successfully",
	MsgEmailSendFailed:   "Failed to send email",
	MsgEmailSynced:       "Emails synced successfully",
	MsgEmailSyncFailed:   "Failed to sync emails",
	MsgEmailSyncSuccess:  "Email sync successful",
	MsgEmailMarkedRead:   "Email marked as read",
	MsgEmailMarkedUnread: "Email marked as unread",
	MsgEmailStarred:      "Email starred",
	MsgEmailUnstarred:    "Email unstarred",
	MsgEmailDeleted:      "Email deleted successfully",
	MsgEmailNotFound:     "Email not found",
	MsgEmailMoved:        "Email moved successfully",
	MsgEmailUpdated:      "Email updated successfully",
	MsgEmailsUpdated:     "Emails updated successfully",
	MsgEmailsDeleted:     "Emails deleted successfully",
	MsgPartialSuccess:    "Operation partially successful",

	// 任务相关
	MsgTaskCreated:   "Task created successfully",
	MsgTaskUpdated:   "Task updated successfully",
	MsgTaskDeleted:   "Task deleted successfully",
	MsgTaskNotFound:  "Task not found",
	MsgTaskStarted:   "Task started",
	MsgTaskCompleted: "Task completed successfully",
	MsgTaskFailed:    "Task failed",
	MsgTaskExists:    "Task already exists for this account",
	MsgTaskCancelled: "Task cancelled successfully",

	// 验证相关
	MsgValidationFailed: "Validation failed",
	MsgInvalidEmail:     "Invalid email address",
	MsgInvalidPassword:  "Invalid password",
	MsgRequiredField:    "Required field missing",
	MsgInvalidFormat:    "Invalid format",
	MsgValueTooLong:     "Value too long",
	MsgValueTooShort:    "Value too short",

	// 数据库相关
	MsgDatabaseError:     "Database error",
	MsgRecordNotFound:    "Record not found",
	MsgDuplicateRecord:   "Duplicate record",
	MsgTransactionFailed: "Transaction failed",

	// 通用操作
	MsgUpdateSuccess: "Update successful",
	MsgDeleteSuccess: "Delete successful",
	MsgCreateSuccess: "Create successful",

	// 通知相关
	NotifyNewEmail:      "New Email",
	NotifyTaskCompleted: "Task Completed",
	NotifyTaskFailed:    "Task Failed",
	NotifyAccountError:  "Account Error",
	NotifySystemAlert:   "System Alert",
	NotifyMaintenance:   "Maintenance",
	NotifySecurityAlert: "Security Alert",

	// 通知级别
	NotifySeverityLow:      "Low",
	NotifySeverityMedium:   "Medium",
	NotifySeverityHigh:     "High",
	NotifySeverityCritical: "Critical",
	NotifySeverityUnknown:  "Unknown",

	// 通知字段
	NotifyFieldLevel:   "Level",
	NotifyFieldTime:    "Time",
	NotifyFieldMessage: "Message",
	NotifyFieldAccount: "Account",
	NotifyFieldSubject: "Subject",
	NotifyFieldFrom:    "From",

	// 通知模板
	NotifyTemplateSystem:        "System Notification",
	NotifyTemplateTestMessage:   "This is a test message",
	NotifyTemplateNewEmailTitle: "{{count}} new email(s) received",
	NotifyTemplateNewEmailBody:  "{{count}} new email(s) in {{account}}",
}
