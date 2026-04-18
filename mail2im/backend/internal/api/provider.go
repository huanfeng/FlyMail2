package api

import (
	appconfig "mail2im/internal/config"

	"flymail-core/httputil"

	"github.com/gin-gonic/gin"
)

func GetProviders(c *gin.Context) {
	httputil.Success(c, gin.H{
		"providers": appconfig.Providers(),
	})
}
