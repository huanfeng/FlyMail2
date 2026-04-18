package api

import (
	"flymail-core/httputil"
	"mail2im/internal/core"
	"mail2im/internal/models"
	"time"

	"github.com/gin-gonic/gin"
)

// GenerateShareLink creates a temporary token for an email
func GenerateShareLink(c *gin.Context) {
	id := c.Param("id")

	// Verify email exists and belongs to user (middleware handles auth)
	var email models.Email
	if err := core.DB.First(&email, "id = ?", id).Error; err != nil {
		httputil.NotFound(c, "Email not found", nil)
		return
	}

	// Generate token valid for 1 hour
	token, err := core.GenerateOneTimeToken(id, 1*time.Hour)
	if err != nil {
		httputil.InternalError(c, "Failed to generate token", err)
		return
	}

	httputil.Success(c, gin.H{
		"token":      token,
		"expires_at": time.Now().Add(1 * time.Hour),
		"url":        "/share/" + token, // Frontend route
	})
}

// GetSharedEmail retrieves email content via token (Public Endpoint)
func GetSharedEmail(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		httputil.BadRequest(c, "Token required", nil)
		return
	}

	email, err := core.RedeemOneTimeToken(token)
	if err != nil {
		httputil.Forbidden(c, "Invalid or expired link", nil)
		return
	}

	httputil.Success(c, email)
}
