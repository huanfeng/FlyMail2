package httputil

import (
	"log"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORSConfig CORS 中间件配置
type CORSConfig struct {
	AllowOrigins     []string      // 允许的来源
	AllowMethods     []string      // 允许的 HTTP 方法
	AllowHeaders     []string      // 允许的请求头
	AllowCredentials bool          // 是否允许携带凭证
	MaxAge           time.Duration // 预检请求缓存时间
}

// CORSMiddleware 返回 CORS 中间件
func CORSMiddleware(config CORSConfig) gin.HandlerFunc {
	corsConfig := cors.DefaultConfig()

	if len(config.AllowOrigins) > 0 {
		corsConfig.AllowOrigins = config.AllowOrigins
	}
	if len(config.AllowMethods) > 0 {
		corsConfig.AllowMethods = config.AllowMethods
	}
	if len(config.AllowHeaders) > 0 {
		corsConfig.AllowHeaders = config.AllowHeaders
	}
	corsConfig.AllowCredentials = config.AllowCredentials
	if config.MaxAge > 0 {
		corsConfig.MaxAge = config.MaxAge
	}

	return cors.New(corsConfig)
}

// LoggerMiddleware 返回请求日志中间件（使用标准 log 包）
func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 计算耗时
		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		switch {
		case statusCode >= 500:
			log.Printf("[ERROR] %d | %13v | %15s | %-7s %s", statusCode, latency, clientIP, method, path)
		case statusCode >= 400:
			log.Printf("[WARN]  %d | %13v | %15s | %-7s %s", statusCode, latency, clientIP, method, path)
		default:
			log.Printf("[INFO]  %d | %13v | %15s | %-7s %s", statusCode, latency, clientIP, method, path)
		}
	}
}
