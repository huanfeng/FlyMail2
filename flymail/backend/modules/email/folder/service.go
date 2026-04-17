package folder

import (
	"context"
	"sort"
	"strings"

	"go.uber.org/zap"

	"flymail/pkg/logger"
)

// Service interface for folder operations
type Service interface {
	GetFolders(ctx context.Context, userID uint, accountID uint) ([]*Folder, error)
	SyncFolders(ctx context.Context, userID uint, accountID uint, folders []*FolderInfo) error
	UpdateFolderOrder(ctx context.Context, userID uint, accountID uint, folderID uint, sortOrder int) error
	UpdateMultipleFolderOrders(ctx context.Context, userID uint, accountID uint, orders []FolderOrder) error
	CountEmailsByFolder(ctx context.Context, accountID uint, folderName string) (int64, error)
	CountUnreadEmailsByFolder(ctx context.Context, accountID uint, folderName string) (int64, error)
}

// FolderOrder represents a folder order update
type FolderOrder struct {
	FolderID  uint
	SortOrder int
}

// AccountVerifier interface for verifying account ownership
type AccountVerifier interface {
	VerifyAccountOwnership(ctx context.Context, userID uint, accountID uint) error
}

// service implements Service interface
type service struct {
	repo            Repository
	accountVerifier AccountVerifier
}

// NewService creates a new folder service
func NewService(repo Repository, accountVerifier AccountVerifier) Service {
	return &service{
		repo:            repo,
		accountVerifier: accountVerifier,
	}
}

// GetFolders retrieves all folders for an account
func (s *service) GetFolders(ctx context.Context, userID uint, accountID uint) ([]*Folder, error) {
	// Verify account belongs to user
	if err := s.accountVerifier.VerifyAccountOwnership(ctx, userID, accountID); err != nil {
		return nil, err
	}

	// Get folders from database
	folders, err := s.repo.GetByAccountID(ctx, accountID)
	if err != nil {
		logger.Error("Failed to get folders", zap.Error(err))
		return nil, err
	}

	// Check and fix invalid sort orders
	for _, folder := range folders {
		expectedOrder := calculateSortOrder(folder.Type, folder.SortOrder, folder.Name, folders)
		if folder.SortOrder != expectedOrder {
			folder.SortOrder = expectedOrder
			// Update in database
			s.repo.UpdateFields(ctx, folder.FolderID, map[string]interface{}{
				"sort_order": expectedOrder,
			})
		}
	}

	// Update email counts for each folder if counter is available
	if counter, ok := s.repo.(EmailCounter); ok {
		for _, folder := range folders {
			emailCount, err := counter.CountByFolder(ctx, folder.AccountID, folder.Name)
			if err != nil {
				logger.Warn("Failed to count emails for folder", zap.String("folder", folder.Name), zap.Error(err))
				emailCount = 0
			}

			unreadCount, err := counter.CountUnreadByFolder(ctx, folder.AccountID, folder.Name)
			if err != nil {
				logger.Warn("Failed to count unread emails for folder", zap.String("folder", folder.Name), zap.Error(err))
				unreadCount = 0
			}

			folder.EmailCount = emailCount
			folder.UnreadCount = unreadCount
		}
	}

	return folders, nil
}

// SyncFolders syncs folders from IMAP server
func (s *service) SyncFolders(ctx context.Context, userID uint, accountID uint, folderInfos []*FolderInfo) error {
	// Verify account belongs to user
	if err := s.accountVerifier.VerifyAccountOwnership(ctx, userID, accountID); err != nil {
		return err
	}

	// Get existing folders to preserve custom sort orders
	existingFolders, _ := s.repo.GetByAccountID(ctx, accountID)
	existingFolderMap := make(map[string]*Folder)
	for _, f := range existingFolders {
		existingFolderMap[f.Name] = f
	}

	// Convert to models
	var folders []*Folder
	for _, info := range folderInfos {
		folderType := DetermineFolderTypeByFlags(info.Name, info.RawName, info.Flags)

		// Check if folder already exists and has a valid sort order
		currentOrder := 0
		if existing, ok := existingFolderMap[info.Name]; ok {
			currentOrder = existing.SortOrder
		}

		folder := &Folder{
			AccountID: accountID,
			Name:      info.Name,
			RawName:   info.RawName,
			Delimiter: info.Delimiter,
			Flags:     joinFlags(info.Flags),
			Type:      folderType,
			SortOrder: calculateSortOrder(folderType, currentOrder, info.Name, existingFolders),
		}

		// Extract parent name if delimiter exists
		if info.Delimiter != "" && strings.Contains(info.Name, info.Delimiter) {
			parts := strings.Split(info.Name, info.Delimiter)
			if len(parts) > 1 {
				folder.ParentName = strings.Join(parts[:len(parts)-1], info.Delimiter)
			}
		}

		folders = append(folders, folder)
	}

	// Sync folders to database
	return s.repo.SyncFolders(ctx, accountID, folders)
}

// UpdateFolderOrder updates the sort order of a single folder
func (s *service) UpdateFolderOrder(ctx context.Context, userID uint, accountID uint, folderID uint, sortOrder int) error {
	// Verify account belongs to user
	if err := s.accountVerifier.VerifyAccountOwnership(ctx, userID, accountID); err != nil {
		return err
	}

	return s.repo.UpdateFields(ctx, folderID, map[string]interface{}{
		"sort_order": sortOrder,
	})
}

// UpdateMultipleFolderOrders updates the sort order of multiple folders
func (s *service) UpdateMultipleFolderOrders(ctx context.Context, userID uint, accountID uint, orders []FolderOrder) error {
	// Verify account belongs to user
	if err := s.accountVerifier.VerifyAccountOwnership(ctx, userID, accountID); err != nil {
		return err
	}

	// Update each folder order
	for _, order := range orders {
		if err := s.repo.UpdateFields(ctx, order.FolderID, map[string]interface{}{
			"sort_order": order.SortOrder,
		}); err != nil {
			return err
		}
	}

	return nil
}

// CountEmailsByFolder counts total emails in a folder
func (s *service) CountEmailsByFolder(ctx context.Context, accountID uint, folderName string) (int64, error) {
	if counter, ok := s.repo.(EmailCounter); ok {
		return counter.CountByFolder(ctx, accountID, folderName)
	}
	return 0, nil
}

// CountUnreadEmailsByFolder counts unread emails in a folder
func (s *service) CountUnreadEmailsByFolder(ctx context.Context, accountID uint, folderName string) (int64, error) {
	if counter, ok := s.repo.(EmailCounter); ok {
		return counter.CountUnreadByFolder(ctx, accountID, folderName)
	}
	return 0, nil
}

// Helper functions

func joinFlags(flags []string) string {
	return strings.Join(flags, ",")
}

// calculateSortOrder generates appropriate sort order based on folder type
// INBOX = 1, Standard folders = 10-99, Custom folders = 100+
func calculateSortOrder(folderType FolderType, currentOrder int, folderName string, existingFolders []*Folder) int {
	switch folderType {
	case FolderTypeInbox:
		return 1
	case FolderTypeSent:
		return 10
	case FolderTypeDrafts:
		return 11
	case FolderTypeTrash:
		return 12
	case FolderTypeJunk:
		return 13
	default:
		// Custom folders start from 100
		if currentOrder >= 100 {
			return currentOrder
		}

		// Sort custom folders alphabetically starting from 100
		// Collect all custom folders
		var customFolders []string
		for _, f := range existingFolders {
			if f.Type == FolderTypeCustom {
				customFolders = append(customFolders, f.Name)
			}
		}
		// Add current folder if it's new
		isNew := true
		for _, name := range customFolders {
			if name == folderName {
				isNew = false
				break
			}
		}
		if isNew {
			customFolders = append(customFolders, folderName)
		}

		// Sort alphabetically
		sort.Strings(customFolders)

		// Find position and calculate sort order
		for i, name := range customFolders {
			if name == folderName {
				return 100 + i*10 // Leave gaps for manual reordering
			}
		}

		return 100 // Default fallback
	}
}
