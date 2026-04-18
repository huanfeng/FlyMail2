package api

import (
	"flymail-core/httputil"
	"mail2im/internal/core"
	"mail2im/internal/dispatcher"
	"mail2im/internal/models"

	"github.com/gin-gonic/gin"
)

func GetTemplates(c *gin.Context) {
	var templates []models.NotificationTemplate
	if err := core.DB.Find(&templates).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	httputil.Success(c, templates)
}

func CreateTemplate(c *gin.Context) {
	var t models.NotificationTemplate
	if err := c.ShouldBindJSON(&t); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}
	if err := core.DB.Create(&t).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	httputil.Success(c, t)
}

func UpdateTemplate(c *gin.Context) {
	idParam := c.Param("id")
	var t models.NotificationTemplate
	if err := core.DB.First(&t, idParam).Error; err != nil {
		httputil.NotFound(c, "Not found", nil)
		return
	}

	// Bind JSON but ignore ID changes from body by forcing the ID back
	originalID := t.ID
	if err := c.ShouldBindJSON(&t); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}
	t.ID = originalID // Ensure ID remains the same

	if err := core.DB.Save(&t).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	httputil.Success(c, t)
}

func DeleteTemplate(c *gin.Context) {
	id := c.Param("id")
	if err := core.DB.Delete(&models.NotificationTemplate{}, id).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	httputil.NoContent(c, "deleted")
}

// PreviewTemplate renders a template with sample data and returns the result.
func PreviewTemplate(c *gin.Context) {
	var req struct {
		Content     string `json:"content"`
		ChannelType string `json:"channel_type"` // "telegram", "discord", "all"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}

	if req.Content == "" {
		httputil.BadRequest(c, "content is required", nil)
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
		httputil.BadRequest(c, "template rendering failed, check syntax", nil)
		return
	}

	httputil.Success(c, gin.H{"preview": rendered})
}

// GetTemplateVariables returns all available template variables with descriptions and examples.
func GetTemplateVariables(c *gin.Context) {
	httputil.Success(c, dispatcher.GetTemplateVariables())
}

// GetDefaultTemplates returns the built-in default templates.
func GetDefaultTemplates(c *gin.Context) {
	var templates []models.NotificationTemplate
	if err := core.DB.Where("is_default = ?", true).Find(&templates).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	httputil.Success(c, templates)
}
