package api

import (
	appconfig "mail2im/internal/config"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetProviders(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"providers": appconfig.Providers(),
	})
}
