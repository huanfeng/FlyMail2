package response

// 响应状态码定义
const (
	// 成功状态码 (0-999)
	CodeSuccess = 0 // 操作成功

	// 客户端错误 (1000-1999)
	CodeBadRequest         = 1000 // 请求参数错误
	CodeUnauthorized       = 1001 // 未授权
	CodeForbidden          = 1002 // 禁止访问
	CodeNotFound           = 1003 // 资源不存在
	CodeMethodNotAllowed   = 1004 // 方法不允许
	CodeConflict           = 1005 // 资源冲突
	CodeTooManyRequests    = 1006 // 请求过多
	CodeValidationFailed   = 1007 // 验证失败
	CodeInvalidCredentials = 1008 // 凭证无效
	CodeTokenExpired       = 1009 // Token过期
	CodeTokenInvalid       = 1010 // Token无效
	CodeAccountNotActive   = 1011 // 账户未激活
	CodePermissionDenied   = 1012 // 权限不足

	// 服务器错误 (2000-2999)
	CodeInternalError      = 2000 // 内部错误
	CodeDatabaseError      = 2001 // 数据库错误
	CodeCacheError         = 2002 // 缓存错误
	CodeEmailSendError     = 2003 // 邮件发送失败
	CodeEmailSyncError     = 2004 // 邮件同步失败
	CodeExternalAPIError   = 2005 // 外部API错误
	CodeFileOperationError = 2006 // 文件操作错误
	CodeServiceUnavailable = 2007 // 服务不可用

	// 业务错误 (3000-3999)
	CodeUserNotFound         = 3000 // 用户不存在
	CodeUserAlreadyExists    = 3001 // 用户已存在
	CodeAccountNotFound      = 3002 // 账户不存在
	CodeAccountAlreadyExists = 3003 // 账户已存在
	CodeEmailNotFound        = 3004 // 邮件不存在
	CodeFolderNotFound       = 3005 // 文件夹不存在
	CodeTaskNotFound         = 3006 // 任务不存在
	CodeInvalidAccountType   = 3007 // 无效的账户类型
	CodeConnectionFailed     = 3008 // 连接失败
	CodeAuthenticationFailed = 3009 // 认证失败
	CodeQuotaExceeded        = 3010 // 配额超限
	CodeOperationFailed      = 3011 // 操作失败
)

// HTTP状态码映射
var httpStatusMap = map[int]int{
	CodeSuccess:              200,
	CodeBadRequest:           400,
	CodeUnauthorized:         401,
	CodeForbidden:            403,
	CodeNotFound:             404,
	CodeMethodNotAllowed:     405,
	CodeConflict:             409,
	CodeTooManyRequests:      429,
	CodeValidationFailed:     400,
	CodeInvalidCredentials:   401,
	CodeTokenExpired:         401,
	CodeTokenInvalid:         401,
	CodeAccountNotActive:     403,
	CodePermissionDenied:     403,
	CodeInternalError:        500,
	CodeDatabaseError:        500,
	CodeCacheError:           500,
	CodeEmailSendError:       500,
	CodeEmailSyncError:       500,
	CodeExternalAPIError:     500,
	CodeFileOperationError:   500,
	CodeServiceUnavailable:   503,
	CodeUserNotFound:         404,
	CodeUserAlreadyExists:    409,
	CodeAccountNotFound:      404,
	CodeAccountAlreadyExists: 409,
	CodeEmailNotFound:        404,
	CodeFolderNotFound:       404,
	CodeTaskNotFound:         404,
	CodeInvalidAccountType:   400,
	CodeConnectionFailed:     500,
	CodeAuthenticationFailed: 401,
	CodeQuotaExceeded:        429,
	CodeOperationFailed:      500,
}

// GetHTTPStatus 获取对应的HTTP状态码
func GetHTTPStatus(code int) int {
	if status, ok := httpStatusMap[code]; ok {
		return status
	}
	// 根据范围返回默认状态码
	if code < 1000 {
		return 200
	} else if code < 2000 {
		return 400
	} else if code < 3000 {
		return 500
	}
	return 400 // 默认返回400
}
