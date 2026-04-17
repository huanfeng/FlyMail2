package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"mail2im/internal/core"
	"mail2im/internal/models"
)

type setupRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email"`
}

type loginRequest struct {
	Identifier string `json:"identifier" binding:"required"` // username or email
	Password   string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type updateProfileRequest struct {
	Username        string `json:"username" binding:"required"`
	Email           string `json:"email"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func SetupUser(c *gin.Context) {
	var count int64
	if err := core.DB.Model(&models.User{}).Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check user state"})
		return
	}

	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_exists"})
		return
	}

	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)

	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username_and_password_required"})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_hash_password"})
		return
	}

	user := models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashed),
	}

	if err := core.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed_to_create_user"})
		return
	}

	pair, err := core.IssueTokenPair(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_issue_tokens"})
		return
	}

	c.JSON(http.StatusOK, authResponse(user, pair))
}

func Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" || strings.TrimSpace(req.Password) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username_and_password_required"})
		return
	}

	var user models.User
	if err := core.DB.Where("username = ? OR email = ?", identifier, identifier).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_query_user"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
		return
	}

	now := time.Now()
	core.DB.Model(&user).Update("last_login_at", &now)

	pair, err := core.IssueTokenPair(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_issue_tokens"})
		return
	}

	c.JSON(http.StatusOK, authResponse(user, pair))
}

func RefreshToken(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	tokenRecord, err := core.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		status := http.StatusUnauthorized
		if err == gorm.ErrRecordNotFound {
			status = http.StatusUnauthorized
		}
		c.JSON(status, gin.H{"error": "invalid_refresh_token"})
		return
	}

	var user models.User
	if err := core.DB.First(&user, tokenRecord.UserID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_refresh_token"})
		return
	}

	if err := core.RevokeRefreshToken(req.RefreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_revoke_refresh_token"})
		return
	}

	pair, err := core.IssueTokenPair(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_issue_tokens"})
		return
	}

	c.JSON(http.StatusOK, authResponse(user, pair))
}

func GetMe(c *gin.Context) {
	user, ok := CurrentUser(c)
	if !ok {
		respondUnauthorized(c)
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": publicUser(user)})
}

func UpdateProfile(c *gin.Context) {
	user, ok := CurrentUser(c)
	if !ok {
		respondUnauthorized(c)
		return
	}

	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
		return
	}

	username := strings.TrimSpace(req.Username)
	email := strings.TrimSpace(req.Email)

	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username_required"})
		return
	}

	if err := core.DB.First(&user, user.ID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_user"})
		return
	}

	updates := map[string]interface{}{
		"username": username,
		"email":    email,
	}

	if strings.TrimSpace(req.NewPassword) != "" {
		if strings.TrimSpace(req.CurrentPassword) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "current_password_required"})
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "current_password_invalid"})
			return
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_hash_password"})
			return
		}
		updates["password_hash"] = string(hashed)
	}

	if err := core.DB.Model(&user).Updates(updates).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed_to_update_user"})
		return
	}

	if err := core.DB.First(&user, user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_load_user"})
		return
	}

	// Rotate refresh tokens after profile changes
	if err := core.RevokeAllRefreshTokensForUser(user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_revoke_tokens"})
		return
	}

	pair, err := core.IssueTokenPair(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_issue_tokens"})
		return
	}

	c.JSON(http.StatusOK, authResponse(user, pair))
}

func authResponse(user models.User, pair *core.TokenPair) gin.H {
	return gin.H{
		"user":               publicUser(user),
		"access_token":       pair.AccessToken,
		"refresh_token":      pair.RefreshToken,
		"access_expires_at":  pair.AccessExpiresAt,
		"refresh_expires_at": pair.RefreshExpiresAt,
		"token_type":         "Bearer",
		"expires_in_sec":     int(time.Until(pair.AccessExpiresAt).Seconds()),
		"refresh_in_sec":     int(time.Until(pair.RefreshExpiresAt).Seconds()),
	}
}

func publicUser(user models.User) gin.H {
	return gin.H{
		"id":        user.ID,
		"username":  user.Username,
		"email":     user.Email,
		"last_seen": user.LastLoginAt,
	}
}
