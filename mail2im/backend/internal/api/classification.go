package api

import (
	"mail2im/internal/core"
	"mail2im/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// --- MailType CRUD ---

func GetMailTypes(c *gin.Context) {
	var types []models.MailType
	if err := core.DB.Order("priority desc").Find(&types).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, types)
}

func CreateMailType(c *gin.Context) {
	var mt models.MailType
	if err := c.ShouldBindJSON(&mt); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := core.DB.Create(&mt).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, mt)
}

func UpdateMailType(c *gin.Context) {
	id := c.Param("id")
	var mt models.MailType
	if err := core.DB.First(&mt, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}
	if err := c.ShouldBindJSON(&mt); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if mt.IsSystem {
		// Prevent changing key for system types
		var existing models.MailType
		core.DB.First(&existing, id)
		mt.Key = existing.Key
	}
	if err := core.DB.Save(&mt).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, mt)
}

func DeleteMailType(c *gin.Context) {
	id := c.Param("id")
	var mt models.MailType
	if err := core.DB.First(&mt, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}
	if mt.IsSystem {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot delete system type"})
		return
	}
	if err := core.DB.Delete(&mt).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// --- FolderRule CRUD ---

func GetFolderRules(c *gin.Context) {
	var rules []models.FolderRule
	if err := core.DB.Order("`order` asc").Find(&rules).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rules)
}

func CreateFolderRule(c *gin.Context) {
	var rule models.FolderRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := core.DB.Create(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rule)
}

func UpdateFolderRule(c *gin.Context) {
	id := c.Param("id")
	var rule models.FolderRule
	if err := core.DB.First(&rule, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := core.DB.Save(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rule)
}

func DeleteFolderRule(c *gin.Context) {
	id := c.Param("id")
	if err := core.DB.Delete(&models.FolderRule{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
