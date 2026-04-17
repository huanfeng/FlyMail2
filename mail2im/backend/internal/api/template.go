package api

import (
	"mail2im/internal/core"
	"mail2im/internal/dispatcher"
	"mail2im/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetTemplates(c *gin.Context) {
	var templates []models.NotificationTemplate
	if err := core.DB.Find(&templates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, templates)
}

func CreateTemplate(c *gin.Context) {
	var t models.NotificationTemplate
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := core.DB.Create(&t).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

func UpdateTemplate(c *gin.Context) {
	idParam := c.Param("id")
	var t models.NotificationTemplate
	if err := core.DB.First(&t, idParam).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}

	// Bind JSON but ignore ID changes from body by forcing the ID back
	originalID := t.ID
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	t.ID = originalID // Ensure ID remains the same

	if err := core.DB.Save(&t).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

func DeleteTemplate(c *gin.Context) {
	id := c.Param("id")
	if err := core.DB.Delete(&models.NotificationTemplate{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// PreviewTemplate renders a template with sample data and returns the result.
func PreviewTemplate(c *gin.Context) {
	var req struct {
		Content     string `json:"content"`
		ChannelType string `json:"channel_type"` // "telegram", "discord", "all"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}

	sampleData := dispatcher.SampleTemplateData()
	var rendered string

	switch req.ChannelType {
	case "telegram":
		rendered = dispatcher.RenderTemplateHTML(req.Content, sampleData, "")
	default:
		rendered = dispatcher.RenderTemplate(req.Content, sampleData, "")
	}

	if rendered == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "template rendering failed, check syntax"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"preview": rendered})
}

// GetTemplateVariables returns all available template variables with descriptions and examples.
func GetTemplateVariables(c *gin.Context) {
	c.JSON(http.StatusOK, dispatcher.GetTemplateVariables())
}

// GetDefaultTemplates returns the built-in default templates.
func GetDefaultTemplates(c *gin.Context) {
	var templates []models.NotificationTemplate
	if err := core.DB.Where("is_default = ?", true).Find(&templates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, templates)
}
