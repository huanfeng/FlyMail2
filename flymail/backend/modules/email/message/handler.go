package message

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"flymail/modules/email/message/dto"
	"flymail/pkg/i18n"
	"flymail/pkg/response"
)

// Handler handles HTTP requests for email messages
type Handler struct {
	service Service
}

// NewHandler creates a new email message handler
func NewHandler(service Service) *Handler {
	return &Handler{
		service: service,
	}
}

// List returns paginated emails
func (h *Handler) List(c *gin.Context) {
	userID := c.GetUint("userID")

	// Parse query parameters
	filter := &EmailFilter{
		Page:     1,
		PageSize: 20,
	}

	// Parse account_id
	if accountIDStr := c.Query("account_id"); accountIDStr != "" {
		if accountID, err := strconv.ParseUint(accountIDStr, 10, 32); err == nil {
			filter.AccountID = uint(accountID)
		}
	}

	// Parse folder_id
	if folderIDStr := c.Query("folder_id"); folderIDStr != "" {
		if folderID, err := strconv.ParseUint(folderIDStr, 10, 32); err == nil {
			filter.FolderID = uint(folderID)
		}
	}

	// Parse folder_name (legacy support)
	if folderName := c.Query("folder"); folderName != "" {
		filter.FolderName = folderName
	}

	// Parse virtual_folder
	if virtualFolder := c.Query("virtual_folder"); virtualFolder != "" {
		filter.VirtualFolder = virtualFolder
	}

	// Parse is_read
	if isReadStr := c.Query("is_read"); isReadStr != "" {
		if isRead, err := strconv.ParseBool(isReadStr); err == nil {
			filter.IsRead = &isRead
		}
	}

	// Parse is_starred
	if isStarredStr := c.Query("is_starred"); isStarredStr != "" {
		if isStarred, err := strconv.ParseBool(isStarredStr); err == nil {
			filter.IsStarred = &isStarred
		}
	}

	// Parse search
	if search := c.Query("search"); search != "" {
		filter.Search = search
	}

	// Parse pagination
	if pageStr := c.Query("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			filter.Page = page
		}
	}

	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if pageSize, err := strconv.Atoi(pageSizeStr); err == nil && pageSize > 0 && pageSize <= 100 {
			filter.PageSize = pageSize
		}
	} else if limitStr := c.Query("limit"); limitStr != "" {
		// Legacy support for 'limit' parameter
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 100 {
			filter.PageSize = limit
		}
	}

	result, err := h.service.GetEmails(c.Request.Context(), userID, filter)
	if err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	// Convert to DTO
	emailResponses := ConvertEmailsToListResponse(result.Emails)
	response.SuccessWithPage(c, i18n.MsgSuccess, emailResponses, filter.Page, filter.PageSize, result.Total)
}

// Get returns a specific email
func (h *Handler) Get(c *gin.Context) {
	userID := c.GetUint("userID")
	emailID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	email, err := h.service.GetEmail(c.Request.Context(), userID, uint(emailID))
	if err != nil {
		response.NotFound(c, i18n.MsgEmailNotFound, err)
		return
	}

	// Convert to detail DTO
	emailResponse := ConvertEmailToDetailResponse(email)
	response.Success(c, i18n.MsgSuccess, emailResponse)
}

// MarkAsRead marks an email as read
func (h *Handler) MarkAsRead(c *gin.Context) {
	userID := c.GetUint("userID")
	emailID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	updates := map[string]interface{}{
		"is_read": true,
	}

	if err := h.service.UpdateEmailStatus(c.Request.Context(), userID, uint(emailID), updates); err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgEmailUpdated, nil)
}

// MarkAsUnread marks an email as unread
func (h *Handler) MarkAsUnread(c *gin.Context) {
	userID := c.GetUint("userID")
	emailID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	updates := map[string]interface{}{
		"is_read": false,
	}

	if err := h.service.UpdateEmailStatus(c.Request.Context(), userID, uint(emailID), updates); err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgEmailUpdated, nil)
}

// Star stars an email
func (h *Handler) Star(c *gin.Context) {
	userID := c.GetUint("userID")
	emailID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	updates := map[string]interface{}{
		"is_starred": true,
	}

	if err := h.service.UpdateEmailStatus(c.Request.Context(), userID, uint(emailID), updates); err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgEmailUpdated, nil)
}

// Unstar unstars an email
func (h *Handler) Unstar(c *gin.Context) {
	userID := c.GetUint("userID")
	emailID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	updates := map[string]interface{}{
		"is_starred": false,
	}

	if err := h.service.UpdateEmailStatus(c.Request.Context(), userID, uint(emailID), updates); err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgEmailUpdated, nil)
}

// Delete deletes an email
func (h *Handler) Delete(c *gin.Context) {
	userID := c.GetUint("userID")
	emailID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	// Parse query parameter for deleting from server
	deleteFromServer := c.Query("delete_from_server") == "true"

	if err := h.service.DeleteEmail(c.Request.Context(), userID, uint(emailID), deleteFromServer); err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.NoContent(c, i18n.MsgEmailDeleted)
}

// UpdateFlags updates email flags (read/unread, starred/unstarred, etc.)
func (h *Handler) UpdateFlags(c *gin.Context) {
	userID := c.GetUint("userID")
	emailID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	// Parse request body
	var req dto.UpdateFlagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	// Build updates map
	updates := make(map[string]interface{})
	if req.IsRead != nil {
		updates["is_read"] = *req.IsRead
	}
	if req.IsStarred != nil {
		updates["is_starred"] = *req.IsStarred
	}

	// Check if there are any updates
	if len(updates) == 0 {
		response.BadRequest(c, i18n.MsgInvalidRequest, nil)
		return
	}

	// Update email flags
	ctx := c.Request.Context()
	if err := h.service.UpdateEmailStatus(ctx, userID, uint(emailID), updates); err != nil {
		response.InternalError(c, i18n.MsgOperationFailed, err)
		return
	}

	response.Success(c, i18n.MsgEmailUpdated, nil)
}

// GetFlags retrieves email flags without fetching the entire email
func (h *Handler) GetFlags(c *gin.Context) {
	userID := c.GetUint("userID")
	emailID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	ctx := c.Request.Context()
	email, err := h.service.GetEmail(ctx, userID, uint(emailID))
	if err != nil {
		response.NotFound(c, i18n.MsgEmailNotFound, err)
		return
	}

	// Return only flags
	flags := map[string]interface{}{
		"is_read":    email.IsRead,
		"is_starred": email.IsStarred,
		// Add more flags as needed
	}

	response.Success(c, i18n.MsgSuccess, flags)
}

// BatchUpdateFlags updates flags for multiple emails
func (h *Handler) BatchUpdateFlags(c *gin.Context) {
	userID := c.GetUint("userID")

	var req dto.BatchUpdateFlagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	// Build updates map
	updates := make(map[string]interface{})
	if req.IsRead != nil {
		updates["is_read"] = *req.IsRead
	}
	if req.IsStarred != nil {
		updates["is_starred"] = *req.IsStarred
	}

	// Check if there are any updates
	if len(updates) == 0 {
		response.BadRequest(c, i18n.MsgInvalidRequest, nil)
		return
	}

	ctx := c.Request.Context()
	result := &dto.BatchOperationResult{
		Success: []uint{},
		Failed:  []uint{},
		Errors:  []string{},
	}

	// Process each email
	for _, emailID := range req.EmailIDs {
		if err := h.service.UpdateEmailStatus(ctx, userID, emailID, updates); err != nil {
			result.Failed = append(result.Failed, emailID)
			result.Errors = append(result.Errors, err.Error())
		} else {
			result.Success = append(result.Success, emailID)
		}
	}

	// Return appropriate status based on results
	if len(result.Failed) == 0 {
		response.Success(c, i18n.MsgEmailsUpdated, result)
	} else if len(result.Success) == 0 {
		response.InternalError(c, i18n.MsgOperationFailed, nil)
	} else {
		// Partial success
		response.Success(c, i18n.MsgPartialSuccess, result)
	}
}

// BatchDelete deletes multiple emails
func (h *Handler) BatchDelete(c *gin.Context) {
	userID := c.GetUint("userID")

	var req dto.BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, i18n.MsgInvalidRequest, err)
		return
	}

	ctx := c.Request.Context()
	result := &dto.BatchOperationResult{
		Success: []uint{},
		Failed:  []uint{},
		Errors:  []string{},
	}

	// Process each email
	for _, emailID := range req.EmailIDs {
		if err := h.service.DeleteEmail(ctx, userID, emailID, req.DeleteFromServer); err != nil {
			result.Failed = append(result.Failed, emailID)
			result.Errors = append(result.Errors, err.Error())
		} else {
			result.Success = append(result.Success, emailID)
		}
	}

	// Return appropriate status based on results
	if len(result.Failed) == 0 {
		response.Success(c, i18n.MsgEmailsDeleted, result)
	} else if len(result.Success) == 0 {
		response.InternalError(c, i18n.MsgOperationFailed, nil)
	} else {
		// Partial success
		response.Success(c, i18n.MsgPartialSuccess, result)
	}
}
