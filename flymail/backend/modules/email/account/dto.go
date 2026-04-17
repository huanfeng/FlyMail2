package account

import "time"

// CreateRequest represents the request to create an email account
type CreateRequest struct {
	Name              string `json:"name" binding:"required"`
	Email             string `json:"email" binding:"required,email"`
	Type              string `json:"type" binding:"required,oneof=smtp imap oauth"`
	ImapServer        string `json:"imap_server"`
	ImapPort          int    `json:"imap_port"`
	ImapSSL           bool   `json:"imap_ssl"`
	SmtpServer        string `json:"smtp_server"`
	SmtpPort          int    `json:"smtp_port"`
	SmtpSSL           bool   `json:"smtp_ssl"`
	Username          string `json:"username" binding:"required"`
	Password          string `json:"password" binding:"required"` // This will be bound from JSON
	OAuthToken        string `json:"oauth_token,omitempty"`
	OAuthRefreshToken string `json:"oauth_refresh_token,omitempty"`
	// Initial sync options
	InitialSyncOption string `json:"initial_sync_option,omitempty" binding:"omitempty,oneof=none recent_days recent_count full"` // none, recent_days, recent_count, full
	InitialSyncDays   int    `json:"initial_sync_days,omitempty" binding:"omitempty,min=1,max=365"`                              // When initial_sync_option is recent_days
	InitialSyncCount  int    `json:"initial_sync_count,omitempty" binding:"omitempty,min=1,max=1000"`                            // When initial_sync_option is recent_count
}

// UpdateRequest represents the request to update an email account
type UpdateRequest struct {
	Name       *string `json:"name,omitempty"`
	Email      *string `json:"email,omitempty"`
	Type       *string `json:"type,omitempty"`
	ImapServer *string `json:"imap_server,omitempty"`
	ImapPort   *int    `json:"imap_port,omitempty"`
	ImapSSL    *bool   `json:"imap_ssl,omitempty"`
	SmtpServer *string `json:"smtp_server,omitempty"`
	SmtpPort   *int    `json:"smtp_port,omitempty"`
	SmtpSSL    *bool   `json:"smtp_ssl,omitempty"`
	Username   *string `json:"username,omitempty"`
	Password   *string `json:"password,omitempty"`
	IsActive   *bool   `json:"is_active,omitempty"`
}

// TestRequest represents the request to test an email account configuration
type TestRequest struct {
	Name              string `json:"name" binding:"required"`
	Email             string `json:"email" binding:"required,email"`
	Type              string `json:"type" binding:"required,oneof=smtp imap oauth"`
	ImapServer        string `json:"imap_server"`
	ImapPort          int    `json:"imap_port"`
	ImapSSL           bool   `json:"imap_ssl"`
	SmtpServer        string `json:"smtp_server"`
	SmtpPort          int    `json:"smtp_port"`
	SmtpSSL           bool   `json:"smtp_ssl"`
	Username          string `json:"username" binding:"required"`
	Password          string `json:"password" binding:"required"`
	OAuthToken        string `json:"oauth_token,omitempty"`
	OAuthRefreshToken string `json:"oauth_refresh_token,omitempty"`
}

// Response represents the email account response
type Response struct {
	AccountID         uint       `json:"account_id"`
	Name              string     `json:"name"`
	Email             string     `json:"email"`
	Type              string     `json:"type"`
	ImapServer        string     `json:"imap_server"`
	ImapPort          int        `json:"imap_port"`
	ImapSSL           bool       `json:"imap_ssl"`
	SmtpServer        string     `json:"smtp_server"`
	SmtpPort          int        `json:"smtp_port"`
	SmtpSSL           bool       `json:"smtp_ssl"`
	Username          string     `json:"username"`
	IsActive          bool       `json:"is_active"`
	LastSync          *time.Time `json:"last_sync"`
	SupportsIDLE      *bool      `json:"supports_idle"`
	SortOrder         int        `json:"sort_order"`
	IsFullSynced      bool       `json:"is_full_synced"`
	IsSyncing         bool       `json:"is_syncing"`
	SyncProgress      int        `json:"sync_progress"`
	SyncError         string     `json:"sync_error"`
	InitialSyncOption string     `json:"initial_sync_option"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// ConvertToResponse converts an EmailAccount to Response DTO
func ConvertToResponse(account *EmailAccount) *Response {
	return &Response{
		AccountID:         account.AccountID,
		Name:              account.Name,
		Email:             account.Email,
		Type:              account.Type,
		ImapServer:        account.ImapServer,
		ImapPort:          account.ImapPort,
		ImapSSL:           account.ImapSSL,
		SmtpServer:        account.SmtpServer,
		SmtpPort:          account.SmtpPort,
		SmtpSSL:           account.SmtpSSL,
		Username:          account.Username,
		IsActive:          account.IsActive,
		LastSync:          account.LastSync,
		SupportsIDLE:      account.SupportsIDLE,
		SortOrder:         account.SortOrder,
		IsFullSynced:      account.IsFullSynced,
		IsSyncing:         account.IsSyncing,
		SyncProgress:      account.SyncProgress,
		SyncError:         account.SyncError,
		InitialSyncOption: account.InitialSyncOption,
		CreatedAt:         account.CreatedAt,
		UpdatedAt:         account.UpdatedAt,
	}
}

// ConvertAccountsToResponse converts multiple EmailAccounts to Response DTOs
func ConvertAccountsToResponse(accounts []EmailAccount) []Response {
	responses := make([]Response, len(accounts))
	for i, account := range accounts {
		responses[i] = *ConvertToResponse(&account)
	}
	return responses
}

// UpdateAccountOrderRequest represents the request to update account order
type UpdateAccountOrderRequest struct {
	AccountOrders []struct {
		AccountID uint `json:"account_id" binding:"required"`
		SortOrder int  `json:"sort_order" binding:"required"`
	} `json:"account_orders" binding:"required,min=1"`
}

// SyncStatusResponse represents the sync status of an account
type SyncStatusResponse struct {
	IsSyncing         bool       `json:"is_syncing"`
	IsFullSynced      bool       `json:"is_full_synced"`
	SyncProgress      int        `json:"sync_progress"`
	SyncError         string     `json:"sync_error"`
	LastSync          *time.Time `json:"last_sync"`
	LastSyncError     *time.Time `json:"last_sync_error"`
	InitialSyncOption string     `json:"initial_sync_option"`
}
