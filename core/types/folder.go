package types

import "strings"

// FolderType represents the semantic type of an email folder.
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

// String returns the string key for a FolderType.
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

// ParseFolderType converts a string key to FolderType.
func ParseFolderType(s string) FolderType {
	switch strings.ToLower(s) {
	case "inbox", "primary":
		return FolderTypeInbox
	case "sent":
		return FolderTypeSent
	case "drafts", "draft":
		return FolderTypeDrafts
	case "trash":
		return FolderTypeTrash
	case "junk", "spam":
		return FolderTypeJunk
	case "archive":
		return FolderTypeArchive
	case "custom":
		return FolderTypeCustom
	default:
		return FolderTypeUnknown
	}
}

// FolderInfo represents folder metadata returned from IMAP LIST.
type FolderInfo struct {
	Name       string   `json:"name"`       // decoded UTF-8 display name
	Path       string   `json:"path"`       // raw IMAP path (may be UTF-7 encoded)
	Delimiter  string   `json:"delimiter"`  // hierarchy delimiter (e.g. "/", ".")
	Attributes []string `json:"attributes"` // IMAP flags like \Noselect, \Sent, etc.
}

// HasAttribute checks if the folder has a specific IMAP attribute (case-insensitive).
func (f *FolderInfo) HasAttribute(attr string) bool {
	lower := strings.ToLower(attr)
	for _, a := range f.Attributes {
		if strings.ToLower(a) == lower {
			return true
		}
	}
	return false
}

// IsNoSelect returns true if the folder has the \Noselect attribute.
func (f *FolderInfo) IsNoSelect() bool {
	return f.HasAttribute(`\Noselect`)
}

// ClassifyFolder determines a folder's FolderType using IMAP attributes first,
// then falling back to name-based matching with Chinese locale support.
func ClassifyFolder(name, path string, attributes []string) FolderType {
	// 1. IMAP special-use attributes (RFC 6154) — most reliable
	for _, attr := range attributes {
		switch strings.ToLower(attr) {
		case `\inbox`:
			return FolderTypeInbox
		case `\sent`:
			return FolderTypeSent
		case `\drafts`:
			return FolderTypeDrafts
		case `\trash`:
			return FolderTypeTrash
		case `\junk`:
			return FolderTypeJunk
		case `\archive`, `\all`:
			return FolderTypeArchive
		}
	}

	// 2. Name-based classification with Chinese support
	return classifyByName(name, path)
}

func classifyByName(name, path string) FolderType {
	lower := strings.ToLower(name)
	lowerPath := strings.ToLower(path)

	// Inbox
	if lower == "inbox" || name == "收件箱" || lowerPath == "inbox" {
		return FolderTypeInbox
	}

	// Sent
	sentNames := []string{"sent", "sent items", "sent mail", "已发送", "已发送邮件", "已发邮件"}
	for _, s := range sentNames {
		if lower == s || name == s {
			return FolderTypeSent
		}
	}
	if strings.Contains(lower, "sent") || strings.Contains(lowerPath, "sent") {
		return FolderTypeSent
	}

	// Drafts
	draftNames := []string{"drafts", "draft", "草稿", "草稿箱"}
	for _, s := range draftNames {
		if lower == s || name == s {
			return FolderTypeDrafts
		}
	}
	if strings.Contains(lower, "draft") || strings.Contains(lowerPath, "draft") {
		return FolderTypeDrafts
	}

	// Trash
	trashNames := []string{"trash", "deleted", "deleted items", "已删除", "垃圾箱", "废纸篓"}
	for _, s := range trashNames {
		if lower == s || name == s {
			return FolderTypeTrash
		}
	}
	if strings.Contains(lower, "trash") || strings.Contains(lower, "deleted") ||
		strings.Contains(lowerPath, "trash") || strings.Contains(lowerPath, "deleted") {
		return FolderTypeTrash
	}

	// Junk / Spam
	junkNames := []string{"junk", "spam", "垃圾邮件"}
	for _, s := range junkNames {
		if lower == s || name == s {
			return FolderTypeJunk
		}
	}
	if strings.Contains(lower, "spam") || strings.Contains(lower, "junk") ||
		strings.Contains(lowerPath, "spam") || strings.Contains(lowerPath, "junk") {
		return FolderTypeJunk
	}

	// Archive
	archiveNames := []string{"archive", "归档", "存档"}
	for _, s := range archiveNames {
		if lower == s || name == s {
			return FolderTypeArchive
		}
	}
	if strings.Contains(lower, "archive") || strings.Contains(lowerPath, "archive") {
		return FolderTypeArchive
	}

	return FolderTypeCustom
}
