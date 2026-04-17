package model

import "strings"

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

// ParseFolderType parses a string to FolderType
func ParseFolderType(s string) FolderType {
	switch s {
	case "inbox":
		return FolderTypeInbox
	case "sent":
		return FolderTypeSent
	case "drafts":
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

// DetermineFolderType determines folder type based on folder name
func DetermineFolderType(folderName string) FolderType {
	return DetermineFolderTypeAdvanced(folderName, "")
}

// DetermineFolderTypeAdvanced determines folder type with advanced matching
// 支持模糊匹配和 rawName 参数
func DetermineFolderTypeAdvanced(folderName string, rawName string) FolderType {
	lowerName := strings.ToLower(folderName)
	lowerRawName := strings.ToLower(rawName)

	// 精确匹配优先
	switch lowerName {
	case "inbox", "收件箱":
		return FolderTypeInbox
	case "sent", "sent items", "sent mail", "已发送", "已发送邮件":
		return FolderTypeSent
	case "drafts", "draft", "草稿", "草稿箱":
		return FolderTypeDrafts
	case "trash", "deleted", "deleted items", "已删除", "垃圾箱", "废纸篓":
		return FolderTypeTrash
	case "junk", "spam", "垃圾邮件":
		return FolderTypeJunk
	case "archive", "归档", "存档":
		return FolderTypeArchive
	}

	// 模糊匹配 - 收件箱 (精确匹配已处理过常见情况，这里处理特殊情况)
	if lowerRawName == "inbox" || rawName == "收件箱" {
		return FolderTypeInbox
	}

	// 模糊匹配 - 已发送
	if strings.Contains(lowerName, "sent") || strings.Contains(lowerRawName, "sent") ||
		folderName == "已发送" || rawName == "已发送" ||
		folderName == "已发邮件" || rawName == "已发邮件" {
		return FolderTypeSent
	}

	// 模糊匹配 - 草稿
	if strings.Contains(lowerName, "draft") || strings.Contains(lowerRawName, "draft") ||
		folderName == "草稿箱" || rawName == "草稿箱" ||
		folderName == "草稿" || rawName == "草稿" {
		return FolderTypeDrafts
	}

	// 模糊匹配 - 垃圾箱
	if strings.Contains(lowerName, "trash") || strings.Contains(lowerRawName, "trash") ||
		strings.Contains(lowerName, "deleted") || strings.Contains(lowerRawName, "deleted") ||
		folderName == "垃圾箱" || rawName == "垃圾箱" ||
		folderName == "已删除" || rawName == "已删除" ||
		folderName == "废纸篓" || rawName == "废纸篓" {
		return FolderTypeTrash
	}

	// 模糊匹配 - 垃圾邮件
	if strings.Contains(lowerName, "spam") || strings.Contains(lowerRawName, "spam") ||
		strings.Contains(lowerName, "junk") || strings.Contains(lowerRawName, "junk") ||
		folderName == "垃圾邮件" || rawName == "垃圾邮件" {
		return FolderTypeJunk
	}

	// 模糊匹配 - 归档
	if strings.Contains(lowerName, "archive") || strings.Contains(lowerRawName, "archive") ||
		folderName == "归档" || rawName == "归档" ||
		folderName == "存档" || rawName == "存档" {
		return FolderTypeArchive
	}

	return FolderTypeCustom
}
