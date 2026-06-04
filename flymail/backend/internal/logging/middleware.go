package logging

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"flymail-core/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	// RequestIDKey 是 request_id 在 gin.Context 中的键。
	RequestIDKey = "request_id"
	// RequestIDHeader 是 request_id 的 HTTP 头名。
	RequestIDHeader = "X-Request-ID"
)

// genRequestID 生成 16 位 hex 短 ID；失败时返回空串（不阻断请求）。
func genRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}

// RequestID 为每个请求生成（或透传传入的）request_id，写入 context 与响应头。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(RequestIDHeader)
		if rid == "" {
			rid = genRequestID()
		}
		c.Set(RequestIDKey, rid)
		c.Header(RequestIDHeader, rid)
		c.Next()
	}
}

// GinLogger 记录结构化访问日志；skipPaths 中的路径不记录（健康检查/SSE 长连接）。
func GinLogger(skipPaths ...string) gin.HandlerFunc {
	skip := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = struct{}{}
	}
	return func(c *gin.Context) {
		if _, ok := skip[c.Request.URL.Path]; ok {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", status),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
		}
		if rid, ok := c.Get(RequestIDKey); ok {
			fields = append(fields, zap.String("request_id", rid.(string)))
		}
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("error", c.Errors.String()))
		}

		switch {
		case status >= 500:
			logger.Error("http request", fields...)
		case status >= 400:
			logger.Warn("http request", fields...)
		default:
			logger.Info("http request", fields...)
		}
	}
}

// GinRecovery 捕获 panic 并记录带堆栈的 error 日志，返回 500。
func GinRecovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, err any) {
		fields := []zap.Field{
			zap.Any("panic", err),
			zap.String("path", c.Request.URL.Path),
			zap.Stack("stack"),
		}
		if rid, ok := c.Get(RequestIDKey); ok {
			fields = append(fields, zap.String("request_id", rid.(string)))
		}
		logger.Error("panic recovered", fields...)
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}

// FromGin 返回带 request_id 字段的子 logger，供 handler 内打日志关联请求。
func FromGin(c *gin.Context) *zap.Logger {
	if rid, ok := c.Get(RequestIDKey); ok {
		return logger.With(zap.String("request_id", rid.(string)))
	}
	return logger.With()
}
