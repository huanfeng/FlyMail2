package folder

import "time"

// Response represents the folder response
type Response struct {
	FolderID    uint       `json:"folder_id"`
	AccountID   uint       `json:"account_id"`
	Name        string     `json:"name"`
	RawName     string     `json:"raw_name"`
	Type        string     `json:"type"`
	Delimiter   string     `json:"delimiter"`
	ParentName  string     `json:"parent_name"`
	Flags       string     `json:"flags"`
	EmailCount  int64      `json:"email_count"`
	UnreadCount int64      `json:"unread_count"`
	SortOrder   int        `json:"sort_order"`
	LastSyncAt  *time.Time `json:"last_sync_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ConvertToResponse converts a Folder to Response DTO
func ConvertToResponse(folder *Folder) *Response {
	return &Response{
		FolderID:    folder.FolderID,
		AccountID:   folder.AccountID,
		Name:        folder.Name,
		RawName:     folder.RawName,
		Type:        folder.Type.String(),
		Delimiter:   folder.Delimiter,
		ParentName:  folder.ParentName,
		Flags:       folder.Flags,
		EmailCount:  folder.EmailCount,
		UnreadCount: folder.UnreadCount,
		SortOrder:   folder.SortOrder,
		LastSyncAt:  folder.LastSyncAt,
		CreatedAt:   folder.CreatedAt,
		UpdatedAt:   folder.UpdatedAt,
	}
}

// ConvertFoldersToResponse converts multiple folders to response
func ConvertFoldersToResponse(folders []*Folder) []*Response {
	responses := make([]*Response, len(folders))
	for i, folder := range folders {
		responses[i] = ConvertToResponse(folder)
	}
	return responses
}

// UpdateOrderRequest represents the request to update folder order
type UpdateOrderRequest struct {
	FolderOrders []struct {
		FolderID  uint `json:"folder_id" binding:"required"`
		SortOrder int  `json:"sort_order" binding:"required"`
	} `json:"folder_orders" binding:"required,min=1"`
}

// SyncResponse represents the folder sync response
type SyncResponse struct {
	Folders  []*Response `json:"folders"`
	Count    int         `json:"count"`
	SyncedAt time.Time   `json:"synced_at"`
}
