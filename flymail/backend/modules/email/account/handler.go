package account

import (
	"context"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"flymail/pkg/i18n"
	"flymail-core/logger"
	"flymail/pkg/response"
)

// EmailService interface for email operations needed by account handler
type EmailService interface {
	TestConnectionAndUpdateCapabilities(ctx context.Context, account *EmailAccount) (*TestConnectionResult, error)
}

// Handler handles account-related HTTP requests
type Handler struct {
	service      Service
	emailService EmailService
}

// NewHandler creates a new account handler
func NewHandler(service Service, emailService EmailService) *Handler {
	return &Handler{
		service:      service,
		emailService: emailService,
	}
}

// List returns all email accounts for the authenticated user
func (h *Handler) List(c *gin.Context) {
	userID := c.GetUint("userID")

	accounts, err := h.service.GetAccounts(c.Request.Context(), userID)
	if err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	// Convert to DTO
	accountResponses := ConvertAccountsToResponse(accounts)
	response.Success(c, i18n.MsgSuccess, gin.H{
		"accounts": accountResponses,
		"count":    len(accountResponses),
	})
}

// Get returns a specific email account
func (h *Handler) Get(c *gin.Context) {
	userID := c.GetUint("userID")
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	account, err := h.service.GetAccount(c.Request.Context(), userID, uint(accountID))
	if err != nil {
		response.NotFound(c, i18n.MsgAccountNotFound, err)
		return
	}

	// Convert to DTO
	accountResponse := ConvertToResponse(account)
	response.Success(c, i18n.MsgSuccess, accountResponse)
}

// Create creates a new email account
func (h *Handler) Create(c *gin.Context) {
	userID := c.GetUint("userID")

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("Invalid account creation request", zap.Error(err))
		response.ValidationError(c, "request", err.Error())
		return
	}

	// Convert DTO to model
	account := &EmailAccount{
		UserID:            userID,
		Name:              req.Name,
		Email:             req.Email,
		Type:              req.Type,
		ImapServer:        req.ImapServer,
		ImapPort:          req.ImapPort,
		ImapSSL:           req.ImapSSL,
		SmtpServer:        req.SmtpServer,
		SmtpPort:          req.SmtpPort,
		SmtpSSL:           req.SmtpSSL,
		Username:          req.Username,
		Password:          req.Password,
		OAuthToken:        req.OAuthToken,
		OAuthRefreshToken: req.OAuthRefreshToken,
		IsActive:          true,
		// Initial sync options
		InitialSyncOption: req.InitialSyncOption,
		InitialSyncDays:   req.InitialSyncDays,
		InitialSyncCount:  req.InitialSyncCount,
	}

	logger.Debug("Creating email account",
		zap.String("name", account.Name),
		zap.String("email", account.Email),
		zap.String("type", account.Type),
		zap.Bool("has_password", account.Password != ""),
	)

	if err := h.service.CreateAccount(c.Request.Context(), userID, account); err != nil {
		logger.Error("Failed to create account", zap.Error(err))
		// Check if account already exists
		if errors.Is(err, ErrAccountExists) {
			response.Error(c, response.CodeAccountAlreadyExists, i18n.MsgAccountAlreadyExists, err)
		} else {
			response.InternalError(c, i18n.MsgOperationFailed, err)
		}
		return
	}

	// Convert to DTO (password is already excluded in DTO)
	accountResponse := ConvertToResponse(account)
	response.Created(c, i18n.MsgAccountCreated, gin.H{
		"account": accountResponse,
	})
}

// Update updates an email account
func (h *Handler) Update(c *gin.Context) {
	userID := c.GetUint("userID")
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "request", err.Error())
		return
	}

	// Convert to map for update
	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.Type != nil {
		updates["type"] = *req.Type
	}
	if req.ImapServer != nil {
		updates["imap_server"] = *req.ImapServer
	}
	if req.ImapPort != nil {
		updates["imap_port"] = *req.ImapPort
	}
	if req.ImapSSL != nil {
		updates["imap_ssl"] = *req.ImapSSL
	}
	if req.SmtpServer != nil {
		updates["smtp_server"] = *req.SmtpServer
	}
	if req.SmtpPort != nil {
		updates["smtp_port"] = *req.SmtpPort
	}
	if req.SmtpSSL != nil {
		updates["smtp_ssl"] = *req.SmtpSSL
	}
	if req.Username != nil {
		updates["username"] = *req.Username
	}
	if req.Password != nil && *req.Password != "" {
		updates["password"] = *req.Password
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	logger.Debug("Updating email account",
		zap.Uint("accountID", uint(accountID)),
		zap.Bool("has_password_update", req.Password != nil),
	)

	if err := h.service.UpdateAccount(c.Request.Context(), userID, uint(accountID), updates); err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			response.NotFound(c, i18n.MsgAccountNotFound, err)
		} else {
			response.InternalError(c, i18n.MsgOperationFailed, err)
		}
		return
	}

	response.Success(c, i18n.MsgAccountUpdated, nil)
}

// Delete deletes an email account
func (h *Handler) Delete(c *gin.Context) {
	userID := c.GetUint("userID")
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	if err := h.service.DeleteAccount(c.Request.Context(), userID, uint(accountID)); err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			response.NotFound(c, i18n.MsgAccountNotFound, err)
		} else {
			response.InternalError(c, i18n.MsgOperationFailed, err)
		}
		return
	}

	response.NoContent(c, i18n.MsgAccountDeleted)
}

// GetStats returns statistics for an email account
func (h *Handler) GetStats(c *gin.Context) {
	userID := c.GetUint("userID")
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	stats, err := h.service.GetAccountStats(c.Request.Context(), userID, uint(accountID))
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			response.NotFound(c, i18n.MsgAccountNotFound, err)
		} else {
			response.InternalError(c, i18n.MsgOperationFailed, err)
		}
		return
	}

	response.Success(c, i18n.MsgSuccess, stats)
}

// UpdateAccountsOrder updates the sort order of accounts
func (h *Handler) UpdateAccountsOrder(c *gin.Context) {
	userID := c.GetUint("userID")

	var req UpdateAccountOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "request", err.Error())
		return
	}

	// Call service to update orders
	for _, order := range req.AccountOrders {
		if err := h.service.UpdateAccount(c.Request.Context(), userID, order.AccountID, map[string]interface{}{
			"sort_order": order.SortOrder,
		}); err != nil {
			response.InternalError(c, i18n.MsgOperationFailed, err)
			return
		}
	}

	response.Success(c, "Account order updated successfully", nil)
}

// TestAccount tests an email account configuration without saving it
func (h *Handler) TestAccount(c *gin.Context) {
	var req TestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "request", err.Error())
		return
	}

	// Create a temporary account model for testing
	account := &EmailAccount{
		Name:       req.Name,
		Email:      req.Email,
		Type:       req.Type,
		ImapServer: req.ImapServer,
		ImapPort:   req.ImapPort,
		ImapSSL:    req.ImapSSL,
		SmtpServer: req.SmtpServer,
		SmtpPort:   req.SmtpPort,
		SmtpSSL:    req.SmtpSSL,
		Username:   req.Username,
		Password:   req.Password,
	}

	// Use email service to test connection if available
	var result *TestConnectionResult
	var err error

	if h.emailService != nil {
		result, err = h.emailService.TestConnectionAndUpdateCapabilities(c.Request.Context(), account)
	} else {
		// Use account service's test connection
		result, err = h.service.TestConnection(c.Request.Context(), account)
	}

	if err != nil {
		logger.Error("Failed to test account connection", zap.Error(err))
		response.InternalError(c, "Failed to test account connection", err)
		return
	}

	// Check if both IMAP and SMTP failed
	if !result.IMAP && !result.SMTP {
		response.Error(c, response.CodeBadRequest, "Connection test failed for both IMAP and SMTP", nil)
		return
	}

	response.Success(c, "Account test completed", gin.H{
		"test_result": result,
	})
}

// GetSyncStatus returns the sync status of a specific email account
func (h *Handler) GetSyncStatus(c *gin.Context) {
	userID := c.GetUint("userID")
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	account, err := h.service.GetAccount(c.Request.Context(), userID, uint(accountID))
	if err != nil {
		response.NotFound(c, i18n.MsgAccountNotFound, err)
		return
	}

	// Return sync status fields
	response.Success(c, i18n.MsgSuccess, &SyncStatusResponse{
		IsSyncing:         account.IsSyncing,
		IsFullSynced:      account.IsFullSynced,
		SyncProgress:      account.SyncProgress,
		SyncError:         account.SyncError,
		LastSync:          account.LastSync,
		LastSyncError:     account.LastSyncError,
		InitialSyncOption: account.InitialSyncOption,
	})
}
