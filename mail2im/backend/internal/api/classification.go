package api

import (
	"flymail-core/httputil"
	"mail2im/internal/core"
	"mail2im/internal/models"

	"github.com/gin-gonic/gin"
)

// --- MailType CRUD ---

func GetMailTypes(c *gin.Context) {
	var types []models.MailType
	if err := core.DB.Order("priority desc").Find(&types).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	c.JSON(200, types)
}

func CreateMailType(c *gin.Context) {
	var mt models.MailType
	if err := c.ShouldBindJSON(&mt); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}
	if err := core.DB.Create(&mt).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	c.JSON(200, mt)
}

func UpdateMailType(c *gin.Context) {
	id := c.Param("id")
	var mt models.MailType
	if err := core.DB.First(&mt, id).Error; err != nil {
		httputil.NotFound(c, "Not found", nil)
		return
	}
	if err := c.ShouldBindJSON(&mt); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}
	if mt.IsSystem {
		// Prevent changing key for system types
		var existing models.MailType
		core.DB.First(&existing, id)
		mt.Key = existing.Key
	}
	if err := core.DB.Save(&mt).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	c.JSON(200, mt)
}

func DeleteMailType(c *gin.Context) {
	id := c.Param("id")
	var mt models.MailType
	if err := core.DB.First(&mt, id).Error; err != nil {
		httputil.NotFound(c, "Not found", nil)
		return
	}
	if mt.IsSystem {
		httputil.Forbidden(c, "Cannot delete system type", nil)
		return
	}
	if err := core.DB.Delete(&mt).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	httputil.NoContent(c, "deleted")
}

// --- FolderRule CRUD ---

func GetFolderRules(c *gin.Context) {
	var rules []models.FolderRule
	if err := core.DB.Order("`order` asc").Find(&rules).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	c.JSON(200, rules)
}

func CreateFolderRule(c *gin.Context) {
	var rule models.FolderRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}
	if err := core.DB.Create(&rule).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	c.JSON(200, rule)
}

func UpdateFolderRule(c *gin.Context) {
	id := c.Param("id")
	var rule models.FolderRule
	if err := core.DB.First(&rule, id).Error; err != nil {
		httputil.NotFound(c, "Not found", nil)
		return
	}
	if err := c.ShouldBindJSON(&rule); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}
	if err := core.DB.Save(&rule).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	c.JSON(200, rule)
}

func DeleteFolderRule(c *gin.Context) {
	id := c.Param("id")
	if err := core.DB.Delete(&models.FolderRule{}, id).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	httputil.NoContent(c, "deleted")
}

// --- EmailContentRule CRUD ---

func GetContentRules(c *gin.Context) {
	var rules []models.EmailContentRule
	if err := core.DB.Order("`order` asc").Find(&rules).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	c.JSON(200, rules)
}

func CreateContentRule(c *gin.Context) {
	var rule models.EmailContentRule
	rule.Enabled = true // application-level default before JSON binding
	if err := c.ShouldBindJSON(&rule); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}
	// Select("*") forces GORM to include zero-value bool fields (e.g. enabled=false)
	if err := core.DB.Select("*").Create(&rule).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	c.JSON(200, rule)
}

func UpdateContentRule(c *gin.Context) {
	id := c.Param("id")
	var rule models.EmailContentRule
	if err := core.DB.First(&rule, id).Error; err != nil {
		httputil.NotFound(c, "Not found", nil)
		return
	}
	if err := c.ShouldBindJSON(&rule); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}
	if err := core.DB.Save(&rule).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	c.JSON(200, rule)
}

func DeleteContentRule(c *gin.Context) {
	id := c.Param("id")
	if err := core.DB.Delete(&models.EmailContentRule{}, id).Error; err != nil {
		httputil.InternalError(c, err.Error(), err)
		return
	}
	httputil.NoContent(c, "deleted")
}
