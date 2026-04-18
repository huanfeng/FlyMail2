package httputil

// 响应状态码定义
const (
	// 成功状态码 (0-999)
	CodeSuccess = 0

	// 客户端错误 (1000-1999)
	CodeBadRequest         = 1000
	CodeUnauthorized       = 1001
	CodeForbidden          = 1002
	CodeNotFound           = 1003
	CodeMethodNotAllowed   = 1004
	CodeConflict           = 1005
	CodeTooManyRequests    = 1006
	CodeValidationFailed   = 1007
	CodeInvalidCredentials = 1008
	CodeTokenExpired       = 1009
	CodeTokenInvalid       = 1010
	CodeAccountNotActive   = 1011
	CodePermissionDenied   = 1012

	// 服务器错误 (2000-2999)
	CodeInternalError      = 2000
	CodeDatabaseError      = 2001
	CodeCacheError         = 2002
	CodeEmailSendError     = 2003
	CodeEmailSyncError     = 2004
	CodeExternalAPIError   = 2005
	CodeFileOperationError = 2006
	CodeServiceUnavailable = 2007

	// 业务错误 (3000-3999)
	CodeUserNotFound         = 3000
	CodeUserAlreadyExists    = 3001
	CodeAccountNotFound      = 3002
	CodeAccountAlreadyExists = 3003
	CodeEmailNotFound        = 3004
	CodeFolderNotFound       = 3005
	CodeTaskNotFound         = 3006
	CodeInvalidAccountType   = 3007
	CodeConnectionFailed     = 3008
	CodeAuthenticationFailed = 3009
	CodeQuotaExceeded        = 3010
	CodeOperationFailed      = 3011
)

// HTTP 状态码映射
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

// GetHTTPStatus 获取业务码对应的 HTTP 状态码
func GetHTTPStatus(code int) int {
	if status, ok := httpStatusMap[code]; ok {
		return status
	}
	if code < 1000 {
		return 200
	} else if code < 2000 {
		return 400
	} else if code < 3000 {
		return 500
	}
	return 400
}
