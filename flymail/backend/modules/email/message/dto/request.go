package dto

// UpdateFlagsRequest represents the request to update email flags
type UpdateFlagsRequest struct {
	IsRead    *bool `json:"is_read"`
	IsStarred *bool `json:"is_starred"`
	// Future flags can be added here
	// IsFlagged *bool `json:"is_flagged"`
	// IsDeleted *bool `json:"is_deleted"`
	// Labels []string `json:"labels"`
}

// BatchUpdateFlagsRequest represents the request to update flags for multiple emails
type BatchUpdateFlagsRequest struct {
	EmailIDs  []uint `json:"email_ids" binding:"required,min=1"`
	IsRead    *bool  `json:"is_read"`
	IsStarred *bool  `json:"is_starred"`
}

// BatchDeleteRequest represents the request to delete multiple emails
type BatchDeleteRequest struct {
	EmailIDs         []uint `json:"email_ids" binding:"required,min=1"`
	DeleteFromServer bool   `json:"delete_from_server"`
}
