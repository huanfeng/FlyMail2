package monitor

import (
	"time"

	"github.com/gin-gonic/gin"
)

// Middleware creates a monitoring middleware
func Middleware(collector *Collector) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 记录请求开始
		collector.IncrementRequest()

		// 记录活跃操作
		operation := c.Request.Method + " " + c.Request.URL.Path
		collector.StartOperation(operation)

		// 记录开始时间
		start := time.Now()

		// 处理请求
		c.Next()

		// 记录结束
		collector.EndOperation(operation)

		// 如果有错误，记录错误
		if len(c.Errors) > 0 {
			for _, err := range c.Errors {
				collector.RecordError(operation, err.Err)
			}
		}

		// 记录慢请求
		duration := time.Since(start)
		if duration > 5*time.Second {
			collector.RecordError("slow_request", &slowRequestError{
				Path:     c.Request.URL.Path,
				Method:   c.Request.Method,
				Duration: duration,
			})
		}
	}
}

// slowRequestError represents a slow request error
type slowRequestError struct {
	Path     string
	Method   string
	Duration time.Duration
}

func (e *slowRequestError) Error() string {
	return "慢请求: " + e.Method + " " + e.Path + " 耗时 " + e.Duration.String()
}
