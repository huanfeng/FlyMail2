package response

import (
	"flymail/pkg/i18n"

	"flymail-core/httputil"

	"github.com/gin-gonic/gin"
)

// 类型别名，保持 flymail 调用方兼容
type Response = httputil.Response
type ErrorInfo = httputil.ErrorInfo
type PageData = httputil.PageData

// Success 成功响应
func Success(c *gin.Context, messageKey string, data any) {
	c.JSON(GetHTTPStatus(CodeSuccess), Response{
		Code:    CodeSuccess,
		Message: messageKey,
		Data:    data,
	})
}

// SuccessWithPage 成功响应（带分页）
func SuccessWithPage(c *gin.Context, messageKey string, list any, page, pageSize int, total int64) {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	c.JSON(GetHTTPStatus(CodeSuccess), Response{
		Code:    CodeSuccess,
		Message: messageKey,
		Data: PageData{
			List:       list,
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// Error 错误响应
func Error(c *gin.Context, code int, messageKey string, err error) {
	var errorInfo *ErrorInfo
	if err != nil {
		errorInfo = &ErrorInfo{Details: err.Error()}
	}

	c.JSON(GetHTTPStatus(code), Response{
		Code:    code,
		Message: messageKey,
		Error:   errorInfo,
	})
}

// ErrorWithInfo 带详细信息的错误响应
func ErrorWithInfo(c *gin.Context, code int, messageKey string, errorInfo *ErrorInfo) {
	c.JSON(GetHTTPStatus(code), Response{
		Code:    code,
		Message: messageKey,
		Error:   errorInfo,
	})
}

// ValidationError 验证错误响应
func ValidationError(c *gin.Context, field string, reason string) {
	c.JSON(GetHTTPStatus(CodeValidationFailed), Response{
		Code:    CodeValidationFailed,
		Message: i18n.MsgValidationFailed,
		Error:   &ErrorInfo{Field: field, Reason: reason},
	})
}

// BadRequest 400 错误快捷响应
func BadRequest(c *gin.Context, messageKey string, err error) {
	Error(c, CodeBadRequest, messageKey, err)
}

// Unauthorized 401 错误快捷响应
func Unauthorized(c *gin.Context, messageKey string, err error) {
	Error(c, CodeUnauthorized, messageKey, err)
}

// Forbidden 403 错误快捷响应
func Forbidden(c *gin.Context, messageKey string, err error) {
	Error(c, CodeForbidden, messageKey, err)
}

// NotFound 404 错误快捷响应
func NotFound(c *gin.Context, messageKey string, err error) {
	Error(c, CodeNotFound, messageKey, err)
}

// InternalError 500 错误快捷响应
func InternalError(c *gin.Context, messageKey string, err error) {
	Error(c, CodeInternalError, messageKey, err)
}

// DatabaseError 数据库错误快捷响应
func DatabaseError(c *gin.Context, err error) {
	Error(c, CodeDatabaseError, i18n.MsgDatabaseError, err)
}

// Created 创建成功响应
func Created(c *gin.Context, messageKey string, data any) {
	c.JSON(201, Response{
		Code:    CodeSuccess,
		Message: messageKey,
		Data:    data,
	})
}

// NoContent 无内容响应
func NoContent(c *gin.Context, messageKey string) {
	c.JSON(200, Response{
		Code:    CodeSuccess,
		Message: messageKey,
	})
}
