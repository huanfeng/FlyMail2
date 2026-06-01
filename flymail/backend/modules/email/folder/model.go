package folder

import "time"

// Folder 是账户下的一个 IMAP 文件夹（邮箱）。
// DisplayName 为 UTF-7 解码后的展示名；Path 为原始 IMAP 路径，所有 IMAP 操作必须用 Path。
type Folder struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	AccountID   uint   `gorm:"index;uniqueIndex:idx_folder_account_path;not null" json:"account_id"`
	Path        string `gorm:"uniqueIndex:idx_folder_account_path;not null" json:"path"`
	DisplayName string `gorm:"not null" json:"display_name"`
	Delimiter   string `json:"delimiter"`
	Type        string `gorm:"not null;default:custom" json:"type"`
	Attributes  string `json:"attributes"`
	Selectable  bool   `gorm:"not null" json:"selectable"`

	UIDValidity   uint32     `json:"uid_validity"`
	UIDNext       uint32     `json:"uid_next"`
	LastSyncedUID uint32     `json:"last_synced_uid"`
	LastSyncedAt  *time.Time `json:"last_synced_at"`

	TotalCount  int `gorm:"not null;default:0" json:"total_count"`
	UnreadCount int `gorm:"not null;default:0" json:"unread_count"`
	SortOrder   int `gorm:"not null;default:100" json:"sort_order"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Folder) TableName() string { return "folders" }

// SortOrderForType 给系统文件夹固定排序，自定义返回 100（由调用方按名称微调）。
func SortOrderForType(folderType string) int {
	switch folderType {
	case "inbox":
		return 1
	case "sent":
		return 10
	case "drafts":
		return 11
	case "trash":
		return 12
	case "junk":
		return 13
	case "archive":
		return 14
	default:
		return 100
	}
}
