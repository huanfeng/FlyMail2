package middleware

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"flymail/pkg/logger"
)

// bodyLogWriter is a wrapper for gin.ResponseWriter that captures response body
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// LoggerMiddleware returns a gin middleware for logging requests
func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip logging for health check endpoint
		if c.Request.URL.Path == "/api/v1/monitor/health" {
			c.Next()
			return
		}

		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Log request body in debug mode
		if logger.Logger != nil && logger.Logger.Core().Enabled(zap.DebugLevel) {
			if c.Request.Body != nil && c.Request.ContentLength > 0 {
				bodyBytes, err := io.ReadAll(c.Request.Body)
				if err == nil {
					c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

					// Don't log passwords
					if c.Request.URL.Path != "/api/v1/auth/login" {
						logger.Debug("Request body",
							zap.String("path", path),
							zap.String("body", string(bodyBytes)),
						)
					} else {
						logger.Debug("Request to login endpoint",
							zap.String("path", path),
							zap.String("note", "body hidden for security"),
						)
					}
				}
			}
		}

		// Create a response body writer to capture response
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)

		// Get request details
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()
		errorMessage := c.Errors.ByType(gin.ErrorTypePrivate).String()

		// Base fields for logging
		fields := []zap.Field{
			zap.String("client_ip", clientIP),
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", statusCode),
			zap.Duration("latency", latency),
			zap.String("user_agent", c.Request.UserAgent()),
		}

		if raw != "" {
			fields = append(fields, zap.String("query", raw))
		}

		if errorMessage != "" {
			fields = append(fields, zap.String("error", errorMessage))
		}

		// Log response body in debug mode for non-2xx responses
		if logger.Logger != nil && logger.Logger.Core().Enabled(zap.DebugLevel) && statusCode >= 400 {
			responseBody := blw.body.String()
			if len(responseBody) > 0 && len(responseBody) < 1024 { // Limit response body logging
				fields = append(fields, zap.String("response_body", responseBody))
			}
		}

		// Log based on status code
		switch {
		case statusCode >= 500:
			logger.Error("Server error", fields...)
		case statusCode >= 400:
			logger.Warn("Client error", fields...)
		case statusCode >= 300:
			logger.Info("Redirect", fields...)
		default:
			logger.Info("Request", fields...)
		}
	}
}
