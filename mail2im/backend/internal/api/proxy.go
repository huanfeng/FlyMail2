package api

import (
	"flymail-core/httputil"
	"mail2im/internal/core"
	"mail2im/internal/models"

	"github.com/gin-gonic/gin"
)

func CreateProxy(c *gin.Context) {
	var input models.Proxy
	if err := c.ShouldBindJSON(&input); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}

	// TODO: Validate input (e.g. Type must be socks5 or http)

	if err := core.DB.Create(&input).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}

	httputil.Success(c, input)
}

func GetProxies(c *gin.Context) {
	var proxies []models.Proxy
	if err := core.DB.Find(&proxies).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	httputil.Success(c, proxies)
}

func UpdateProxy(c *gin.Context) {
	id := c.Param("id")
	var proxy models.Proxy
	if err := core.DB.First(&proxy, id).Error; err != nil {
		httputil.NotFound(c, "Proxy not found", nil)
		return
	}

	var input models.Proxy
	if err := c.ShouldBindJSON(&input); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}

	proxy.Name = input.Name
	proxy.Type = input.Type
	proxy.Host = input.Host
	proxy.Port = input.Port
	proxy.Username = input.Username
	if input.Password != "" {
		proxy.Password = input.Password
	}

	if err := core.DB.Save(&proxy).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}

	httputil.Success(c, proxy)
}

func DeleteProxy(c *gin.Context) {
	id := c.Param("id")
	if err := core.DB.Delete(&models.Proxy{}, id).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	httputil.NoContent(c, "deleted")
}
