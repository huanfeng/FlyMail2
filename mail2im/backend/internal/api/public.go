package api

import (
	"mail2im/internal/core"
	"mail2im/internal/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// GenerateShareLink creates a temporary token for an email
func GenerateShareLink(c *gin.Context) {
	id := c.Param("id")

	// Verify email exists and belongs to user (middleware handles auth)
	var email models.Email
	if err := core.DB.First(&email, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Email not found"})
		return
	}

	// Generate token valid for 1 hour
	token, err := core.GenerateOneTimeToken(id, 1*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"expires_at": time.Now().Add(1 * time.Hour),
		"url":        "/share/" + token, // Frontend route
	})
}

// GetSharedEmail retrieves email content via token (Public Endpoint)
func GetSharedEmail(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token required"})
		return
	}

	email, err := core.RedeemOneTimeToken(token)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Invalid or expired link"})
		return
	}

	c.JSON(http.StatusOK, email)
}
