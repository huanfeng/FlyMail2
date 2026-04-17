package api

import (
	"errors"
	"fmt"
	"mail2im/internal/core"
	"mail2im/internal/models"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetEmails(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	search := c.Query("search")
	sortField := c.DefaultQuery("sortField", "received_at")
	sortOrder := c.DefaultQuery("sortOrder", "desc")

	var emails []models.Email
	var total int64

	query := core.DB.Model(&models.Email{})

	if search != "" {
		query = query.Where("subject LIKE ? OR `from` LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	// Omit HTMLBody and TextBody for list view to save bandwidth
	if err := query.Select("id, created_at, updated_at, deleted_at, account_id, mailbox_id, message_id, uid, seq_num, mailbox, mailbox_path, mail_type, `from`, `to`, subject, received_at, is_read").
		Order(buildEmailOrder(sortField, sortOrder)).
		Limit(pageSize).
		Offset(offset).
		Find(&emails).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch emails"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  emails,
		"total": total,
	})
}

func GetEmail(c *gin.Context) {
	id := c.Param("id")
	var email models.Email
	if err := core.DB.First(&email, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Email not found"})
		return
	}

	// Mark as read if not already
	if !email.IsRead {
		email.IsRead = true
		core.DB.Save(&email)
	}

	c.JSON(http.StatusOK, email)
}

func GetEmailHTML(c *gin.Context) {
	id := c.Param("id")
	var email models.Email
	if err := core.DB.Select("html_body, text_body").First(&email, "id = ?", id).Error; err != nil {
		c.String(http.StatusNotFound, "Email not found")
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	if email.HTMLBody != "" {
		c.String(http.StatusOK, email.HTMLBody)
	} else {
		// Fallback to text body wrapped in pre
		c.String(http.StatusOK, "<html><body><pre>"+email.TextBody+"</pre></body></html>")
	}
}

// buildEmailOrder sanitizes sort field/order for the query.
func buildEmailOrder(field, order string) string {
	allowed := map[string]string{
		"subject":     "subject",
		"from":        "`from`",
		"to":          "`to`",
		"mailbox":     "mailbox",
		"mail_type":   "mail_type",
		"received_at": "received_at",
		"created_at":  "created_at",
		"updated_at":  "updated_at",
	}

	col, ok := allowed[field]
	if !ok {
		col = "received_at"
	}

	dir := "DESC"
	if strings.EqualFold(order, "asc") || order == "1" {
		dir = "ASC"
	}

	return col + " " + dir
}

func DeleteEmail(c *gin.Context) {
	id := c.Param("id")
	var email models.Email
	if err := core.DB.First(&email, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Email not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := core.DB.Delete(&email).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	core.RecordSystemLog("email_delete", "success", fmt.Sprintf("Deleted email #%s", email.ID), email.Subject)
	c.JSON(http.StatusOK, gin.H{"message": "Deleted"})
}

func DeleteAllEmails(c *gin.Context) {
	result := core.DB.Where("1 = 1").Delete(&models.Email{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	core.RecordSystemLog("email_delete_all", "success", "Deleted all local emails", fmt.Sprintf("deleted %d emails", result.RowsAffected))
	c.JSON(http.StatusOK, gin.H{"message": "Deleted"})
}
