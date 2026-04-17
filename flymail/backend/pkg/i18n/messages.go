package i18n

// 消息ID常量定义
const (
	// 通用消息
	MsgSuccess            = "msg.success"
	MsgOperationFailed    = "msg.operation_failed"
	MsgInvalidRequest     = "msg.invalid_request"
	MsgUnauthorized       = "msg.unauthorized"
	MsgForbidden          = "msg.forbidden"
	MsgNotFound           = "msg.not_found"
	MsgInternalError      = "msg.internal_error"
	MsgServiceUnavailable = "msg.service_unavailable"

	// 认证相关
	MsgLoginSuccess       = "msg.auth.login_success"
	MsgLoginFailed        = "msg.auth.login_failed"
	MsgLogoutSuccess      = "msg.auth.logout_success"
	MsgInvalidCredentials = "msg.auth.invalid_credentials"
	MsgTokenExpired       = "msg.auth.token_expired"
	MsgTokenInvalid       = "msg.auth.token_invalid"
	MsgTokenRefreshed     = "msg.auth.token_refreshed"
	MsgPasswordChanged    = "msg.auth.password_changed"
	MsgPasswordResetSent  = "msg.auth.password_reset_sent"

	// 用户相关
	MsgUserCreated        = "msg.user.created"
	MsgUserUpdated        = "msg.user.updated"
	MsgUserDeleted        = "msg.user.deleted"
	MsgUserNotFound       = "msg.user.not_found"
	MsgUserAlreadyExists  = "msg.user.already_exists"
	MsgUserProfileUpdated = "msg.user.profile_updated"

	// 邮箱账户相关
	MsgAccountCreated       = "msg.account.created"
	MsgAccountUpdated       = "msg.account.updated"
	MsgAccountDeleted       = "msg.account.deleted"
	MsgAccountNotFound      = "msg.account.not_found"
	MsgAccountAlreadyExists = "msg.account.already_exists"
	MsgAccountTestSuccess   = "msg.account.test_success"
	MsgAccountTestFailed    = "msg.account.test_failed"
	MsgAccountInactive      = "msg.account.inactive"

	// 邮件相关
	MsgEmailSent         = "msg.email.sent"
	MsgEmailSendFailed   = "msg.email.send_failed"
	MsgEmailSynced       = "msg.email.synced"
	MsgEmailSyncFailed   = "msg.email.sync_failed"
	MsgEmailSyncSuccess  = "msg.email.sync_success"
	MsgEmailMarkedRead   = "msg.email.marked_read"
	MsgEmailMarkedUnread = "msg.email.marked_unread"
	MsgEmailStarred      = "msg.email.starred"
	MsgEmailUnstarred    = "msg.email.unstarred"
	MsgEmailDeleted      = "msg.email.deleted"
	MsgEmailNotFound     = "msg.email.not_found"
	MsgEmailMoved        = "msg.email.moved"
	MsgEmailUpdated      = "msg.email.updated"
	MsgEmailsUpdated     = "msg.email.batch_updated"
	MsgEmailsDeleted     = "msg.email.batch_deleted"
	MsgPartialSuccess    = "msg.email.partial_success"

	// 任务相关
	MsgTaskCreated   = "msg.task.created"
	MsgTaskUpdated   = "msg.task.updated"
	MsgTaskDeleted   = "msg.task.deleted"
	MsgTaskNotFound  = "msg.task.not_found"
	MsgTaskStarted   = "msg.task.started"
	MsgTaskCompleted = "msg.task.completed"
	MsgTaskFailed    = "msg.task.failed"
	MsgTaskExists    = "msg.task.exists"
	MsgTaskCancelled = "msg.task.cancelled"

	// 验证相关
	MsgValidationFailed = "msg.validation.failed"
	MsgInvalidEmail     = "msg.validation.invalid_email"
	MsgInvalidPassword  = "msg.validation.invalid_password"
	MsgRequiredField    = "msg.validation.required_field"
	MsgInvalidFormat    = "msg.validation.invalid_format"
	MsgValueTooLong     = "msg.validation.value_too_long"
	MsgValueTooShort    = "msg.validation.value_too_short"

	// 数据库相关
	MsgDatabaseError     = "msg.database.error"
	MsgRecordNotFound    = "msg.database.record_not_found"
	MsgDuplicateRecord   = "msg.database.duplicate_record"
	MsgTransactionFailed = "msg.database.transaction_failed"

	// 通用操作
	MsgUpdateSuccess = "msg.update_success"
	MsgDeleteSuccess = "msg.delete_success"
	MsgCreateSuccess = "msg.create_success"

	// 通知相关消息ID
	NotifyNewEmail      = "notify.new_email"
	NotifyTaskCompleted = "notify.task_completed"
	NotifyTaskFailed    = "notify.task_failed"
	NotifyAccountError  = "notify.account_error"
	NotifySystemAlert   = "notify.system_alert"
	NotifyMaintenance   = "notify.maintenance"
	NotifySecurityAlert = "notify.security_alert"

	// 通知级别
	NotifySeverityLow      = "notify.severity.low"
	NotifySeverityMedium   = "notify.severity.medium"
	NotifySeverityHigh     = "notify.severity.high"
	NotifySeverityCritical = "notify.severity.critical"
	NotifySeverityUnknown  = "notify.severity.unknown"

	// 通知字段
	NotifyFieldLevel   = "notify.field.level"
	NotifyFieldTime    = "notify.field.time"
	NotifyFieldMessage = "notify.field.message"
	NotifyFieldAccount = "notify.field.account"
	NotifyFieldSubject = "notify.field.subject"
	NotifyFieldFrom    = "notify.field.from"

	// 通知模板
	NotifyTemplateSystem        = "notify.template.system"
	NotifyTemplateTestMessage   = "notify.template.test_message"
	NotifyTemplateNewEmailTitle = "notify.template.new_email_title"
	NotifyTemplateNewEmailBody  = "notify.template.new_email_body"
)
