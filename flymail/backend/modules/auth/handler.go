package auth

import (
	"errors"

	"github.com/gin-gonic/gin"

	"flymail/pkg/i18n"
	"flymail/pkg/response"
)

// Handler handles authentication HTTP requests
type Handler struct {
	service Service
}

// NewHandler creates a new auth handler
func NewHandler(service Service) *Handler {
	return &Handler{
		service: service,
	}
}

// Login handles user login
func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "request", err.Error())
		return
	}

	authResponse, err := h.service.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		// 根据错误类型返回不同的响应
		if err.Error() == "invalid credentials" || err.Error() == "user not found" {
			response.ErrorWithInfo(c, response.CodeInvalidCredentials, i18n.MsgInvalidCredentials, &response.ErrorInfo{
				Details:    "Username or password is incorrect",
				Suggestion: "Please check your credentials and try again",
			})
		} else {
			response.Error(c, response.CodeAuthenticationFailed, i18n.MsgLoginFailed, err)
		}
		return
	}

	response.Success(c, i18n.MsgLoginSuccess, authResponse)
}

// Refresh handles token refresh
func (h *Handler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "refresh_token", "Refresh token is required")
		return
	}

	authResponse, err := h.service.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		// 根据错误类型返回不同的响应
		if err.Error() == "invalid refresh token" || err.Error() == "refresh token expired" {
			response.Error(c, response.CodeTokenInvalid, i18n.MsgTokenInvalid, err)
		} else {
			response.Unauthorized(c, i18n.MsgUnauthorized, err)
		}
		return
	}

	response.Success(c, i18n.MsgTokenRefreshed, authResponse)
}

// Me returns the current user's information
func (h *Handler) Me(c *gin.Context) {
	userID := c.GetUint("userID")
	if userID == 0 {
		response.Unauthorized(c, i18n.MsgUnauthorized, errors.New("user not authenticated"))
		return
	}

	user, err := h.service.GetUser(c.Request.Context(), userID)
	if err != nil {
		response.NotFound(c, i18n.MsgUserNotFound, err)
		return
	}

	// Convert to response DTO
	userResponse := struct {
		UserID    uint   `json:"user_id"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		IsAdmin   bool   `json:"is_admin"`
		CreatedAt string `json:"created_at"`
	}{
		UserID:    user.UserID,
		Username:  user.Username,
		Email:     user.Email,
		IsAdmin:   user.IsAdmin,
		CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	response.Success(c, i18n.MsgSuccess, userResponse)
}

// UpdateAdminCredentials handles admin credentials update
func (h *Handler) UpdateAdminCredentials(c *gin.Context) {
	var req struct {
		Username    string `json:"username"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		OldPassword string `json:"old_password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "request", err.Error())
		return
	}

	// Validate that at least one field is provided
	if req.Username == "" && req.Password == "" && req.Email == "" {
		response.ValidationError(c, "request", "At least one of username, email or password must be provided")
		return
	}

	// Validate old password is provided when changing password
	if req.Password != "" && req.OldPassword == "" {
		response.ValidationError(c, "request", "Old password is required when changing password")
		return
	}

	// Basic email validation
	if req.Email != "" && !isValidEmail(req.Email) {
		response.ValidationError(c, "email", "Invalid email format")
		return
	}

	userID := c.GetUint("userID")
	if userID == 0 {
		response.Unauthorized(c, i18n.MsgUnauthorized, errors.New("user not authenticated"))
		return
	}

	err := h.service.UpdateAdminCredentials(c.Request.Context(), userID, req.Username, req.Email, req.Password, req.OldPassword)
	if err != nil {
		if err.Error() == "permission denied: user is not admin" {
			response.Forbidden(c, "Permission denied", err)
		} else if err.Error() == "username already exists" {
			response.ErrorWithInfo(c, response.CodeValidationFailed, "Username already exists", &response.ErrorInfo{
				Details:    "The username is already taken by another user",
				Suggestion: "Please choose a different username",
			})
		} else if err.Error() == "incorrect old password" {
			response.ErrorWithInfo(c, response.CodeInvalidCredentials, "Incorrect old password", &response.ErrorInfo{
				Details:    "The old password provided is incorrect",
				Suggestion: "Please verify your current password and try again",
			})
		} else {
			response.Error(c, response.CodeInternalError, "Failed to update admin credentials", err)
		}
		return
	}

	response.Success(c, "Admin credentials updated successfully", nil)
}

// isValidEmail performs basic email validation
func isValidEmail(email string) bool {
	// Basic email validation
	if len(email) < 3 || len(email) > 254 {
		return false
	}
	// Check for @ symbol
	atIndex := -1
	for i, char := range email {
		if char == '@' {
			if atIndex != -1 {
				return false // Multiple @ symbols
			}
			atIndex = i
		}
	}
	if atIndex == -1 || atIndex == 0 || atIndex == len(email)-1 {
		return false
	}
	return true
}
