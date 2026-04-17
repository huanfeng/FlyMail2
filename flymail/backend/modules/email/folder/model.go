package folder

import (
	"time"

	"gorm.io/gorm"
)

// FolderType represents the type of email folder
type FolderType int

const (
	FolderTypeUnknown FolderType = iota
	FolderTypeInbox
	FolderTypeSent
	FolderTypeDrafts
	FolderTypeTrash
	FolderTypeJunk
	FolderTypeArchive
	FolderTypeCustom
)

// String returns the string representation of FolderType
func (ft FolderType) String() string {
	switch ft {
	case FolderTypeInbox:
		return "inbox"
	case FolderTypeSent:
		return "sent"
	case FolderTypeDrafts:
		return "drafts"
	case FolderTypeTrash:
		return "trash"
	case FolderTypeJunk:
		return "junk"
	case FolderTypeArchive:
		return "archive"
	case FolderTypeCustom:
		return "custom"
	default:
		return "unknown"
	}
}

// Folder represents an email folder
type Folder struct {
	FolderID      uint           `gorm:"primaryKey;column:id" json:"folder_id"`
	AccountID     uint           `gorm:"not null" json:"account_id"`
	Name          string         `gorm:"not null" json:"name"`        // Decoded folder name (UTF-8)
	RawName       string         `gorm:"not null" json:"raw_name"`    // Original folder name (UTF-7)
	Type          FolderType     `json:"type" gorm:"default:0"`       // Folder type enum
	Delimiter     string         `json:"delimiter"`                   // Folder hierarchy delimiter (e.g., "/", ".")
	ParentName    string         `json:"parent_name"`                 // Parent folder name
	Flags         string         `json:"flags"`                       // IMAP folder flags
	EmailCount    int64          `json:"email_count"`                 // Total email count in folder
	UnreadCount   int64          `json:"unread_count"`                // Unread email count
	UIDValidity   uint32         `json:"uid_validity"`                // IMAP UIDVALIDITY value
	UIDNext       uint32         `json:"uid_next"`                    // Next expected UID
	LastSyncedUID uint32         `json:"last_synced_uid"`             // Last synced UID
	LastSyncAt    *time.Time     `json:"last_sync_at"`                // Last sync timestamp
	SortOrder     int            `json:"sort_order" gorm:"default:0"` // Sort order for display
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// FolderInfo represents folder information from IMAP
type FolderInfo struct {
	Name      string
	RawName   string
	Delimiter string
	Flags     []string
}
