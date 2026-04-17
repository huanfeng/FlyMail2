package middleware

import (
	"fmt"
	"strings"

	"flymail/modules/service"
	"github.com/gin-gonic/gin"
)

// MonitorMiddleware creates a middleware for collecting monitoring data
func MonitorMiddleware() gin.HandlerFunc {
	// 暂时简化，不使用 collector
	return func(c *gin.Context) {
		c.Next()
	}
}

// MonitorMiddlewareWithCollector creates a middleware for collecting monitoring data
func MonitorMiddlewareWithCollector(collector *service.MonitorCollector) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip monitor endpoints to avoid recursion
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/monitor/") {
			c.Next()
			return
		}

		operationName := fmt.Sprintf("http_%s", c.Request.Method)
		collector.StartOperation(operationName)
		collector.IncrementRequestCount()

		// Process request
		c.Next()

		// End operation
		collector.EndOperation(operationName)

		// Record error if status indicates failure
		status := c.Writer.Status()
		if status >= 400 {
			collector.RecordError("http_request", fmt.Errorf("%s %s - HTTP %d", c.Request.Method, c.Request.URL.Path, status))
		}
	}
}
