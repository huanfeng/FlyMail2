package api

import (
	"mail2im/internal/core"
	"mail2im/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

func CreateProxy(c *gin.Context) {
	var input models.Proxy
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: Validate input (e.g. Type must be socks5 or http)

	if err := core.DB.Create(&input).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, input)
}

func GetProxies(c *gin.Context) {
	var proxies []models.Proxy
	if err := core.DB.Find(&proxies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, proxies)
}

func UpdateProxy(c *gin.Context) {
	id := c.Param("id")
	var proxy models.Proxy
	if err := core.DB.First(&proxy, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proxy not found"})
		return
	}

	var input models.Proxy
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, proxy)
}

func DeleteProxy(c *gin.Context) {
	id := c.Param("id")
	if err := core.DB.Delete(&models.Proxy{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Deleted"})
}
