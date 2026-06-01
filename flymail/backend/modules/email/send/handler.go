package send

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册发送相关路由到给定的路由组。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	rg.POST("/send", func(c *gin.Context) {
		var req SendRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := svc.Send(req); err != nil {
			if err == ErrNoRecipient {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}
