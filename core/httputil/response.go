package httputil

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	Code    int        `json:"code"`            // 业务状态码
	Message string     `json:"message"`         // 消息
	Data    any        `json:"data"`            // 数据
	Error   *ErrorInfo `json:"error,omitempty"` // 错误信息
}

// ErrorInfo 错误详细信息
type ErrorInfo struct {
	Details    string         `json:"details,omitempty"`    // 错误详情
	Field      string         `json:"field,omitempty"`      // 错误字段
	Reason     string         `json:"reason,omitempty"`     // 错误原因
	Suggestion string         `json:"suggestion,omitempty"` // 建议
	ErrorCode  string         `json:"error_code,omitempty"` // 错误代码
	Metadata   map[string]any `json:"metadata,omitempty"`   // 额外信息
}

// PageData 分页数据结构
type PageData struct {
	List       any   `json:"list"`        // 数据列表
	Page       int   `json:"page"`        // 当前页
	PageSize   int   `json:"page_size"`   // 每页大小
	Total      int64 `json:"total"`       // 总数
	TotalPages int   `json:"total_pages"` // 总页数
}

// Success 成功响应
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: "ok",
		Data:    data,
	})
}

// SuccessMsg 带自定义消息的成功响应
func SuccessMsg(c *gin.Context, message string, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: message,
		Data:    data,
	})
}

// Created 创建成功响应
func Created(c *gin.Context, message string, data any) {
	c.JSON(http.StatusCreated, Response{
		Code:    CodeSuccess,
		Message: message,
		Data:    data,
	})
}

// NoContent 无内容响应
func NoContent(c *gin.Context, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: message,
	})
}

// Error 错误响应（通过业务码自动映射 HTTP 状态码）
func Error(c *gin.Context, code int, message string, err error) {
	var errorInfo *ErrorInfo
	if err != nil {
		errorInfo = &ErrorInfo{Details: err.Error()}
	}
	c.JSON(GetHTTPStatus(code), Response{
		Code:    code,
		Message: message,
		Error:   errorInfo,
	})
}

// ErrorHTTP 使用显式 HTTP 状态码的错误响应
func ErrorHTTP(c *gin.Context, httpStatus int, code int, message string) {
	c.JSON(httpStatus, Response{
		Code:    code,
		Message: message,
	})
}

// ErrorWithInfo 带详细信息的错误响应
func ErrorWithInfo(c *gin.Context, code int, message string, errorInfo *ErrorInfo) {
	c.JSON(GetHTTPStatus(code), Response{
		Code:    code,
		Message: message,
		Error:   errorInfo,
	})
}

// ValidationError 验证错误响应
func ValidationError(c *gin.Context, field string, reason string) {
	c.JSON(GetHTTPStatus(CodeValidationFailed), Response{
		Code:    CodeValidationFailed,
		Message: "validation failed",
		Error:   &ErrorInfo{Field: field, Reason: reason},
	})
}

// BadRequest 400 错误快捷响应
func BadRequest(c *gin.Context, message string, err error) {
	Error(c, CodeBadRequest, message, err)
}

// Unauthorized 401 错误快捷响应
func Unauthorized(c *gin.Context, message string, err error) {
	Error(c, CodeUnauthorized, message, err)
}

// Forbidden 403 错误快捷响应
func Forbidden(c *gin.Context, message string, err error) {
	Error(c, CodeForbidden, message, err)
}

// NotFound 404 错误快捷响应
func NotFound(c *gin.Context, message string, err error) {
	Error(c, CodeNotFound, message, err)
}

// InternalError 500 错误快捷响应
func InternalError(c *gin.Context, message string, err error) {
	Error(c, CodeInternalError, message, err)
}

// DatabaseError 数据库错误快捷响应
func DatabaseError(c *gin.Context, err error) {
	Error(c, CodeDatabaseError, "database error", err)
}

// Paginated 分页响应
func Paginated(c *gin.Context, data any, total int64, page, pageSize int) {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: "ok",
		Data: PageData{
			List:       data,
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// PaginatedMsg 带自定义消息的分页响应
func PaginatedMsg(c *gin.Context, message string, data any, total int64, page, pageSize int) {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: message,
		Data: PageData{
			List:       data,
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}
