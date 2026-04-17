package folder

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"flymail/pkg/i18n"
	"flymail-core/logger"
	"flymail/pkg/response"
)

// IMAPClient interface for IMAP operations
type IMAPClient interface {
	GetFolders() ([]*FolderInfo, error)
}

// IMAPClientFactory interface for creating IMAP clients
type IMAPClientFactory interface {
	CreateIMAPClient(server string, port int, username, password string, useSSL bool) IMAPClient
}

// AccountService interface for account operations needed by folder handler
type AccountService interface {
	GetAccount(ctx context.Context, userID uint, accountID uint) (*Account, error)
}

// Account represents basic account info needed by folder handler
type Account struct {
	AccountID  uint
	Type       string
	ImapServer string
	ImapPort   int
	Username   string
	Password   string
	ImapSSL    bool
}

// Handler handles folder-related HTTP requests
type Handler struct {
	service           Service
	accountService    AccountService
	imapClientFactory IMAPClientFactory
}

// NewHandler creates a new folder handler
func NewHandler(service Service, accountService AccountService, imapClientFactory IMAPClientFactory) *Handler {
	return &Handler{
		service:           service,
		accountService:    accountService,
		imapClientFactory: imapClientFactory,
	}
}

// List gets all folders for an account
func (h *Handler) List(c *gin.Context) {
	userID := c.GetUint("userID")
	accountIDStr := c.Param("id")

	accountID, err := strconv.ParseUint(accountIDStr, 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	// Get folders with counts
	folders, err := h.service.GetFolders(c.Request.Context(), userID, uint(accountID))
	if err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	// Convert to DTO
	folderResponses := ConvertFoldersToResponse(folders)
	response.Success(c, i18n.MsgSuccess, gin.H{
		"folders": folderResponses,
	})
}

// Sync syncs folders from IMAP server
func (h *Handler) Sync(c *gin.Context) {
	userID := c.GetUint("userID")
	accountIDStr := c.Param("id")

	accountID, err := strconv.ParseUint(accountIDStr, 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	// Get account details
	account, err := h.accountService.GetAccount(c.Request.Context(), userID, uint(accountID))
	if err != nil {
		response.NotFound(c, i18n.MsgNotFound, err)
		return
	}

	if account.Type != "imap" {
		response.BadRequest(c, i18n.MsgInvalidRequest, nil)
		return
	}

	// Connect to IMAP server
	imapClient := h.imapClientFactory.CreateIMAPClient(
		account.ImapServer,
		account.ImapPort,
		account.Username,
		account.Password,
		account.ImapSSL,
	)

	// Get folders from IMAP server
	folderInfos, err := imapClient.GetFolders()
	if err != nil {
		logger.Error("Failed to get folders from IMAP", zap.Error(err))
		response.InternalError(c, i18n.MsgInternalError, err)
		return
	}

	// Sync folders
	if err := h.service.SyncFolders(c.Request.Context(), userID, uint(accountID), folderInfos); err != nil {
		logger.Error("Failed to sync folders", zap.Error(err))
		response.InternalError(c, i18n.MsgInternalError, err)
		return
	}

	// Get updated folders
	updatedFolders, err := h.service.GetFolders(c.Request.Context(), userID, uint(accountID))
	if err != nil {
		logger.Error("Failed to get updated folders", zap.Error(err))
		response.InternalError(c, i18n.MsgInternalError, err)
		return
	}

	// Convert to DTO
	folderResponses := ConvertFoldersToResponse(updatedFolders)
	response.Success(c, i18n.MsgSuccess, &SyncResponse{
		Folders:  folderResponses,
		Count:    len(folderResponses),
		SyncedAt: time.Now(),
	})
}

// UpdateFoldersOrder updates the sort order of folders
func (h *Handler) UpdateFoldersOrder(c *gin.Context) {
	userID := c.GetUint("userID")
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	var req UpdateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "request", err.Error())
		return
	}

	// Convert request to service format
	var orders []FolderOrder
	for _, order := range req.FolderOrders {
		orders = append(orders, FolderOrder{
			FolderID:  order.FolderID,
			SortOrder: order.SortOrder,
		})
	}

	// Update folder orders
	if err := h.service.UpdateMultipleFolderOrders(c.Request.Context(), userID, uint(accountID), orders); err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, "Folder order updated successfully", nil)
}
