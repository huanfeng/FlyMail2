package httputil

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	Code    int    `json:"code"`    // 业务状态码
	Message string `json:"message"` // 消息
	Data    any    `json:"data"`    // 数据
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
		Code:    0,
		Message: "ok",
		Data:    data,
	})
}

// Error 错误响应
func Error(c *gin.Context, httpStatus int, code int, message string) {
	c.JSON(httpStatus, Response{
		Code:    code,
		Message: message,
		Data:    nil,
	})
}

// Paginated 分页响应
func Paginated(c *gin.Context, data any, total int64, page, pageSize int) {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, Response{
		Code:    0,
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
