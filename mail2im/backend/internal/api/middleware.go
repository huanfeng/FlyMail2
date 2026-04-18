package api

import (
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"flymail-core/httputil"
	"mail2im/internal/core"
	"mail2im/internal/models"
)

const userContextKey = "currentUser"

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractAccessToken(c)
		if token == "" {
			respondUnauthorized(c)
			return
		}

		userID, err := core.ValidateAccessToken(token)
		if err != nil {
			respondUnauthorized(c)
			return
		}

		var user models.User
		if err := core.DB.First(&user, userID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				respondUnauthorized(c)
				return
			}
			httputil.InternalError(c, "failed to load user", err)
			c.Abort()
			return
		}

		c.Set(userContextKey, user)
		c.Next()
	}
}

func CurrentUser(c *gin.Context) (models.User, bool) {
	val, ok := c.Get(userContextKey)
	if !ok {
		return models.User{}, false
	}
	user, ok := val.(models.User)
	return user, ok
}

func extractAccessToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" && strings.TrimSpace(parts[1]) != "" {
		return strings.TrimSpace(parts[1])
	}

	// Fallback for iframe/previews where custom headers are not available.
	if q := strings.TrimSpace(c.Query("access_token")); q != "" {
		return q
	}
	if q := strings.TrimSpace(c.Query("token")); q != "" {
		return q
	}
	if cookie, err := c.Cookie("access_token"); err == nil && strings.TrimSpace(cookie) != "" {
		return strings.TrimSpace(cookie)
	}
	return ""
}

func respondUnauthorized(c *gin.Context) {
	httputil.Unauthorized(c, "unauthorized", nil)
	c.Abort()
}
