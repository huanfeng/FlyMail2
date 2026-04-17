package response

import (
	"flymail/pkg/i18n"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`    // 状态码
	Message string      `json:"message"` // 消息ID
	Data    interface{} `json:"data"`    // 数据
	Error   *ErrorInfo  `json:"error"`   // 错误信息
}

// ErrorInfo 错误详细信息
type ErrorInfo struct {
	Details    string                 `json:"details,omitempty"`    // 错误详情
	Field      string                 `json:"field,omitempty"`      // 错误字段
	Reason     string                 `json:"reason,omitempty"`     // 错误原因
	Suggestion string                 `json:"suggestion,omitempty"` // 建议
	ErrorCode  string                 `json:"error_code,omitempty"` // 错误代码
	Metadata   map[string]interface{} `json:"metadata,omitempty"`   // 额外信息
}

// PageData 分页数据结构
type PageData struct {
	List       interface{} `json:"list"`        // 数据列表
	Page       int         `json:"page"`        // 当前页
	PageSize   int         `json:"page_size"`   // 每页大小
	Total      int64       `json:"total"`       // 总数
	TotalPages int         `json:"total_pages"` // 总页数
}

// Success 成功响应
func Success(c *gin.Context, messageKey string, data interface{}) {
	resp := Response{
		Code:    CodeSuccess,
		Message: messageKey,
		Data:    data,
		Error:   nil,
	}
	c.JSON(GetHTTPStatus(CodeSuccess), resp)
}

// SuccessWithPage 成功响应（带分页）
func SuccessWithPage(c *gin.Context, messageKey string, list interface{}, page, pageSize int, total int64) {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	pageData := PageData{
		List:       list,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}

	resp := Response{
		Code:    CodeSuccess,
		Message: messageKey,
		Data:    pageData,
		Error:   nil,
	}
	c.JSON(GetHTTPStatus(CodeSuccess), resp)
}

// Error 错误响应
func Error(c *gin.Context, code int, messageKey string, err error) {
	errorInfo := &ErrorInfo{}
	if err != nil {
		errorInfo.Details = err.Error()
	}

	resp := Response{
		Code:    code,
		Message: messageKey,
		Data:    nil,
		Error:   errorInfo,
	}
	c.JSON(GetHTTPStatus(code), resp)
}

// ErrorWithInfo 带详细信息的错误响应
func ErrorWithInfo(c *gin.Context, code int, messageKey string, errorInfo *ErrorInfo) {
	resp := Response{
		Code:    code,
		Message: messageKey,
		Data:    nil,
		Error:   errorInfo,
	}
	c.JSON(GetHTTPStatus(code), resp)
}

// ValidationError 验证错误响应
func ValidationError(c *gin.Context, field string, reason string) {
	errorInfo := &ErrorInfo{
		Field:  field,
		Reason: reason,
	}

	resp := Response{
		Code:    CodeValidationFailed,
		Message: i18n.MsgValidationFailed,
		Data:    nil,
		Error:   errorInfo,
	}
	c.JSON(GetHTTPStatus(CodeValidationFailed), resp)
}

// BadRequest 400错误快捷响应
func BadRequest(c *gin.Context, messageKey string, err error) {
	Error(c, CodeBadRequest, messageKey, err)
}

// Unauthorized 401错误快捷响应
func Unauthorized(c *gin.Context, messageKey string, err error) {
	Error(c, CodeUnauthorized, messageKey, err)
}

// Forbidden 403错误快捷响应
func Forbidden(c *gin.Context, messageKey string, err error) {
	Error(c, CodeForbidden, messageKey, err)
}

// NotFound 404错误快捷响应
func NotFound(c *gin.Context, messageKey string, err error) {
	Error(c, CodeNotFound, messageKey, err)
}

// InternalError 500错误快捷响应
func InternalError(c *gin.Context, messageKey string, err error) {
	Error(c, CodeInternalError, messageKey, err)
}

// DatabaseError 数据库错误快捷响应
func DatabaseError(c *gin.Context, err error) {
	Error(c, CodeDatabaseError, i18n.MsgDatabaseError, err)
}

// Created 创建成功响应
func Created(c *gin.Context, messageKey string, data interface{}) {
	resp := Response{
		Code:    CodeSuccess,
		Message: messageKey,
		Data:    data,
		Error:   nil,
	}
	c.JSON(201, resp) // 201 Created
}

// NoContent 无内容响应（用于删除等操作）
func NoContent(c *gin.Context, messageKey string) {
	resp := Response{
		Code:    CodeSuccess,
		Message: messageKey,
		Data:    nil,
		Error:   nil,
	}
	c.JSON(200, resp) // 返回200而不是204，因为我们需要返回统一格式
}
