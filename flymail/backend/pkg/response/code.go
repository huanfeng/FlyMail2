package response

import "flymail-core/httputil"

// 从 core/httputil 导出所有状态码，保持 flymail 调用方无需修改 import
const (
	CodeSuccess            = httputil.CodeSuccess
	CodeBadRequest         = httputil.CodeBadRequest
	CodeUnauthorized       = httputil.CodeUnauthorized
	CodeForbidden          = httputil.CodeForbidden
	CodeNotFound           = httputil.CodeNotFound
	CodeMethodNotAllowed   = httputil.CodeMethodNotAllowed
	CodeConflict           = httputil.CodeConflict
	CodeTooManyRequests    = httputil.CodeTooManyRequests
	CodeValidationFailed   = httputil.CodeValidationFailed
	CodeInvalidCredentials = httputil.CodeInvalidCredentials
	CodeTokenExpired       = httputil.CodeTokenExpired
	CodeTokenInvalid       = httputil.CodeTokenInvalid
	CodeAccountNotActive   = httputil.CodeAccountNotActive
	CodePermissionDenied   = httputil.CodePermissionDenied
	CodeInternalError      = httputil.CodeInternalError
	CodeDatabaseError      = httputil.CodeDatabaseError
	CodeCacheError         = httputil.CodeCacheError
	CodeEmailSendError     = httputil.CodeEmailSendError
	CodeEmailSyncError     = httputil.CodeEmailSyncError
	CodeExternalAPIError   = httputil.CodeExternalAPIError
	CodeFileOperationError = httputil.CodeFileOperationError
	CodeServiceUnavailable = httputil.CodeServiceUnavailable
	CodeUserNotFound         = httputil.CodeUserNotFound
	CodeUserAlreadyExists    = httputil.CodeUserAlreadyExists
	CodeAccountNotFound      = httputil.CodeAccountNotFound
	CodeAccountAlreadyExists = httputil.CodeAccountAlreadyExists
	CodeEmailNotFound        = httputil.CodeEmailNotFound
	CodeFolderNotFound       = httputil.CodeFolderNotFound
	CodeTaskNotFound         = httputil.CodeTaskNotFound
	CodeInvalidAccountType   = httputil.CodeInvalidAccountType
	CodeConnectionFailed     = httputil.CodeConnectionFailed
	CodeAuthenticationFailed = httputil.CodeAuthenticationFailed
	CodeQuotaExceeded        = httputil.CodeQuotaExceeded
	CodeOperationFailed      = httputil.CodeOperationFailed
)

// GetHTTPStatus 委托给 core
func GetHTTPStatus(code int) int {
	return httputil.GetHTTPStatus(code)
}
