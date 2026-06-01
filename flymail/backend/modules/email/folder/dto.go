package folder

// FolderResponse 是文件夹的对外表示。
type FolderResponse struct {
	ID          uint   `json:"id"`
	AccountID   uint   `json:"account_id"`
	Path        string `json:"path"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	Selectable  bool   `json:"selectable"`
	TotalCount  int    `json:"total_count"`
	UnreadCount int    `json:"unread_count"`
	SortOrder   int    `json:"sort_order"`
}

func toResponse(f *Folder) FolderResponse {
	return FolderResponse{
		ID: f.ID, AccountID: f.AccountID, Path: f.Path, DisplayName: f.DisplayName,
		Type: f.Type, Selectable: f.Selectable, TotalCount: f.TotalCount,
		UnreadCount: f.UnreadCount, SortOrder: f.SortOrder,
	}
}
