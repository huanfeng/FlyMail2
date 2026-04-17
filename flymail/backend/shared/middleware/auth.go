// TODO: migrate to flymail-core/auth for JWT
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"flymail/pkg/i18n"
	"flymail/pkg/jwt"
	"flymail/pkg/response"
	"flymail/shared/config"
)

// AuthMiddleware creates an authentication middleware
func AuthMiddleware() gin.HandlerFunc {
	cfg := config.GetConfig()

	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.ErrorWithInfo(c, response.CodeUnauthorized, i18n.MsgUnauthorized, &response.ErrorInfo{
				Details:    "Authorization header is required",
				Suggestion: "Please provide a valid Bearer token in the Authorization header",
			})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			response.ErrorWithInfo(c, response.CodeUnauthorized, i18n.MsgUnauthorized, &response.ErrorInfo{
				Details:    "Bearer token format is required",
				Suggestion: "Use format: Authorization: Bearer <token>",
			})
			c.Abort()
			return
		}

		claims, err := jwt.ValidateToken(tokenString, cfg.Auth.JWTSecret)
		if err != nil {
			// 根据错误类型返回不同的响应
			if strings.Contains(err.Error(), "expired") {
				response.Error(c, response.CodeTokenExpired, i18n.MsgTokenExpired, err)
			} else {
				response.Error(c, response.CodeTokenInvalid, i18n.MsgTokenInvalid, err)
			}
			c.Abort()
			return
		}

		// Set user ID for downstream handlers
		c.Set("userID", claims.UserID)
		c.Next()
	}
}

// SSEAuthMiddleware creates an authentication middleware for SSE connections
// It supports both Authorization header and URL parameters (token or access_token)
func SSEAuthMiddleware() gin.HandlerFunc {
	cfg := config.GetConfig()

	return func(c *gin.Context) {
		var tokenString string

		// First, try to get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
			if tokenString == authHeader {
				response.ErrorWithInfo(c, response.CodeUnauthorized, i18n.MsgUnauthorized, &response.ErrorInfo{
					Details:    "Bearer token format is required",
					Suggestion: "Use format: Authorization: Bearer <token>",
				})
				c.Abort()
				return
			}
		} else {
			// If no Authorization header, try URL parameters
			tokenString = c.Query("token")
			if tokenString == "" {
				tokenString = c.Query("access_token")
			}
		}

		if tokenString == "" {
			response.ErrorWithInfo(c, response.CodeUnauthorized, i18n.MsgUnauthorized, &response.ErrorInfo{
				Details:    "Authentication token is required",
				Suggestion: "Provide token via Authorization header or URL parameter (?token=xxx)",
			})
			c.Abort()
			return
		}

		claims, err := jwt.ValidateToken(tokenString, cfg.Auth.JWTSecret)
		if err != nil {
			// 根据错误类型返回不同的响应
			if strings.Contains(err.Error(), "expired") {
				response.Error(c, response.CodeTokenExpired, i18n.MsgTokenExpired, err)
			} else {
				response.Error(c, response.CodeTokenInvalid, i18n.MsgTokenInvalid, err)
			}
			c.Abort()
			return
		}

		// Set user ID for downstream handlers
		c.Set("userID", claims.UserID)
		c.Next()
	}
}

// AdminMiddleware ensures the user is an admin
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// For now, we'll check if user ID is 1 (admin)
		// In a real implementation, you'd check against the database
		userID := c.GetUint("userID")
		if userID != 1 {
			response.ErrorWithInfo(c, response.CodePermissionDenied, i18n.MsgForbidden, &response.ErrorInfo{
				Details:    "Admin privileges required",
				Suggestion: "Please contact system administrator if you need admin access",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
