package i18n

// 中文翻译
var zhCNTranslations = map[string]string{
	// 通用消息
	MsgSuccess:            "操作成功",
	MsgOperationFailed:    "操作失败",
	MsgInvalidRequest:     "无效请求",
	MsgUnauthorized:       "未授权",
	MsgForbidden:          "禁止访问",
	MsgNotFound:           "资源未找到",
	MsgInternalError:      "服务器内部错误",
	MsgServiceUnavailable: "服务不可用",

	// 认证相关
	MsgLoginSuccess:       "登录成功",
	MsgLoginFailed:        "登录失败",
	MsgLogoutSuccess:      "退出成功",
	MsgInvalidCredentials: "凭据无效",
	MsgTokenExpired:       "令牌已过期",
	MsgTokenInvalid:       "无效令牌",
	MsgTokenRefreshed:     "令牌刷新成功",
	MsgPasswordChanged:    "密码修改成功",
	MsgPasswordResetSent:  "密码重置链接已发送",

	// 用户相关
	MsgUserCreated:        "用户创建成功",
	MsgUserUpdated:        "用户更新成功",
	MsgUserDeleted:        "用户删除成功",
	MsgUserNotFound:       "用户不存在",
	MsgUserAlreadyExists:  "用户已存在",
	MsgUserProfileUpdated: "个人资料更新成功",

	// 邮箱账户相关
	MsgAccountCreated:       "账户创建成功",
	MsgAccountUpdated:       "账户更新成功",
	MsgAccountDeleted:       "账户删除成功",
	MsgAccountNotFound:      "账户不存在",
	MsgAccountAlreadyExists: "账户已存在",
	MsgAccountTestSuccess:   "连接测试成功",
	MsgAccountTestFailed:    "连接测试失败",
	MsgAccountInactive:      "账户未激活",

	// 邮件相关
	MsgEmailSent:         "邮件发送成功",
	MsgEmailSendFailed:   "邮件发送失败",
	MsgEmailSynced:       "邮件同步成功",
	MsgEmailSyncFailed:   "邮件同步失败",
	MsgEmailSyncSuccess:  "邮件同步成功",
	MsgEmailMarkedRead:   "邮件已标记为已读",
	MsgEmailMarkedUnread: "邮件已标记为未读",
	MsgEmailStarred:      "邮件已加星标",
	MsgEmailUnstarred:    "邮件已取消星标",
	MsgEmailDeleted:      "邮件删除成功",
	MsgEmailNotFound:     "邮件不存在",
	MsgEmailMoved:        "邮件移动成功",
	MsgEmailUpdated:      "邮件更新成功",
	MsgEmailsUpdated:     "批量邮件更新成功",
	MsgEmailsDeleted:     "批量邮件删除成功",
	MsgPartialSuccess:    "操作部分成功",

	// 任务相关
	MsgTaskCreated:   "任务创建成功",
	MsgTaskUpdated:   "任务更新成功",
	MsgTaskDeleted:   "任务删除成功",
	MsgTaskNotFound:  "任务不存在",
	MsgTaskStarted:   "任务已开始",
	MsgTaskCompleted: "任务完成",
	MsgTaskFailed:    "任务失败",
	MsgTaskExists:    "该账户的任务已存在",
	MsgTaskCancelled: "任务已取消",

	// 验证相关
	MsgValidationFailed: "验证失败",
	MsgInvalidEmail:     "无效的邮箱地址",
	MsgInvalidPassword:  "无效的密码",
	MsgRequiredField:    "缺少必填字段",
	MsgInvalidFormat:    "格式无效",
	MsgValueTooLong:     "值过长",
	MsgValueTooShort:    "值过短",

	// 数据库相关
	MsgDatabaseError:     "数据库错误",
	MsgRecordNotFound:    "记录未找到",
	MsgDuplicateRecord:   "重复记录",
	MsgTransactionFailed: "事务失败",

	// 通用操作
	MsgUpdateSuccess: "更新成功",
	MsgDeleteSuccess: "删除成功",
	MsgCreateSuccess: "创建成功",

	// 通知相关
	NotifyNewEmail:      "新邮件",
	NotifyTaskCompleted: "任务完成",
	NotifyTaskFailed:    "任务失败",
	NotifyAccountError:  "账户错误",
	NotifySystemAlert:   "系统警报",
	NotifyMaintenance:   "系统维护",
	NotifySecurityAlert: "安全警报",

	// 通知级别
	NotifySeverityLow:      "低",
	NotifySeverityMedium:   "中",
	NotifySeverityHigh:     "高",
	NotifySeverityCritical: "紧急",
	NotifySeverityUnknown:  "未知",

	// 通知字段
	NotifyFieldLevel:   "级别",
	NotifyFieldTime:    "时间",
	NotifyFieldMessage: "消息",
	NotifyFieldAccount: "账户",
	NotifyFieldSubject: "主题",
	NotifyFieldFrom:    "发件人",

	// 通知模板
	NotifyTemplateSystem:        "系统通知",
	NotifyTemplateTestMessage:   "这是一条测试消息",
	NotifyTemplateNewEmailTitle: "收到 {{count}} 封新邮件",
	NotifyTemplateNewEmailBody:  "{{account}} 中有 {{count}} 封新邮件",
}
