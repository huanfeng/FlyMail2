package api

import (
	"strings"
	"time"

	coreauth "flymail-core/auth"
	"flymail-core/httputil"

	"github.com/gin-gonic/gin"
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
		httputil.InternalError(c, "failed to check user state", err)
		return
	}

	if count > 0 {
		httputil.BadRequest(c, "user_exists", nil)
		return
	}

	var req setupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.BadRequest(c, "invalid_payload", nil)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)

	if req.Username == "" || req.Password == "" {
		httputil.BadRequest(c, "username_and_password_required", nil)
		return
	}

	hashed, err := coreauth.HashPassword(req.Password)
	if err != nil {
		httputil.InternalError(c, "failed_to_hash_password", err)
		return
	}

	user := models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hashed,
	}

	if err := core.DB.Create(&user).Error; err != nil {
		httputil.BadRequest(c, "failed_to_create_user", nil)
		return
	}

	pair, err := core.IssueTokenPair(user.ID)
	if err != nil {
		httputil.InternalError(c, "failed_to_issue_tokens", err)
		return
	}

	httputil.Success(c, authResponse(user, pair))
}

func Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.BadRequest(c, "invalid_payload", nil)
		return
	}

	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" || strings.TrimSpace(req.Password) == "" {
		httputil.BadRequest(c, "username_and_password_required", nil)
		return
	}

	var user models.User
	if err := core.DB.Where("username = ? OR email = ?", identifier, identifier).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			httputil.Unauthorized(c, "invalid_credentials", nil)
			return
		}
		httputil.InternalError(c, "failed_to_query_user", err)
		return
	}

	if !coreauth.VerifyPassword(user.PasswordHash, req.Password) {
		httputil.Unauthorized(c, "invalid_credentials", nil)
		return
	}

	now := time.Now()
	core.DB.Model(&user).Update("last_login_at", &now)

	pair, err := core.IssueTokenPair(user.ID)
	if err != nil {
		httputil.InternalError(c, "failed_to_issue_tokens", err)
		return
	}

	httputil.Success(c, authResponse(user, pair))
}

func RefreshToken(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.BadRequest(c, "invalid_payload", nil)
		return
	}

	tokenRecord, err := core.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		status := 401
		if err == gorm.ErrRecordNotFound {
			status = 401
		}
		httputil.ErrorHTTP(c, status, httputil.CodeUnauthorized, "invalid_refresh_token")
		return
	}

	var user models.User
	if err := core.DB.First(&user, tokenRecord.UserID).Error; err != nil {
		httputil.Unauthorized(c, "invalid_refresh_token", nil)
		return
	}

	if err := core.RevokeRefreshToken(req.RefreshToken); err != nil {
		httputil.InternalError(c, "failed_to_revoke_refresh_token", err)
		return
	}

	pair, err := core.IssueTokenPair(user.ID)
	if err != nil {
		httputil.InternalError(c, "failed_to_issue_tokens", err)
		return
	}

	httputil.Success(c, authResponse(user, pair))
}

func GetMe(c *gin.Context) {
	user, ok := CurrentUser(c)
	if !ok {
		respondUnauthorized(c)
		return
	}

	httputil.Success(c, gin.H{"user": publicUser(user)})
}

func UpdateProfile(c *gin.Context) {
	user, ok := CurrentUser(c)
	if !ok {
		respondUnauthorized(c)
		return
	}

	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.BadRequest(c, "invalid_payload", nil)
		return
	}

	username := strings.TrimSpace(req.Username)
	email := strings.TrimSpace(req.Email)

	if username == "" {
		httputil.BadRequest(c, "username_required", nil)
		return
	}

	if err := core.DB.First(&user, user.ID).Error; err != nil {
		httputil.Unauthorized(c, "invalid_user", nil)
		return
	}

	updates := map[string]interface{}{
		"username": username,
		"email":    email,
	}

	if strings.TrimSpace(req.NewPassword) != "" {
		if strings.TrimSpace(req.CurrentPassword) == "" {
			httputil.BadRequest(c, "current_password_required", nil)
			return
		}
		if !coreauth.VerifyPassword(user.PasswordHash, req.CurrentPassword) {
			httputil.Unauthorized(c, "current_password_invalid", nil)
			return
		}
		hashed, err := coreauth.HashPassword(req.NewPassword)
		if err != nil {
			httputil.InternalError(c, "failed_to_hash_password", err)
			return
		}
		updates["password_hash"] = hashed
	}

	if err := core.DB.Model(&user).Updates(updates).Error; err != nil {
		httputil.BadRequest(c, "failed_to_update_user", nil)
		return
	}

	if err := core.DB.First(&user, user.ID).Error; err != nil {
		httputil.InternalError(c, "failed_to_load_user", err)
		return
	}

	// Rotate refresh tokens after profile changes
	if err := core.RevokeAllRefreshTokensForUser(user.ID); err != nil {
		httputil.InternalError(c, "failed_to_revoke_tokens", err)
		return
	}

	pair, err := core.IssueTokenPair(user.ID)
	if err != nil {
		httputil.InternalError(c, "failed_to_issue_tokens", err)
		return
	}

	httputil.Success(c, authResponse(user, pair))
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
