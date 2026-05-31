package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const ContextUsernameKey = "username"

// Middleware 校验 Authorization: Bearer <access token>。
func Middleware(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "缺少凭证"})
			return
		}
		claims, err := svc.VerifyAccessToken(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "凭证无效"})
			return
		}
		c.Set(ContextUsernameKey, claims.Username)
		c.Next()
	}
}
