package db

import (
	"context"
	"flymail/shared/store/model"
	"gorm.io/gorm"
	"time"
)

// FolderRepository implements the FolderRepository interface
type FolderRepository struct {
	db *gorm.DB
}

// NewFolderRepository creates a new folder repository
func NewFolderRepository(db *gorm.DB) *FolderRepository {
	return &FolderRepository{db: db}
}

// Create creates a new folder
func (r *FolderRepository) Create(ctx context.Context, folder *model.Folder) error {
	return r.db.WithContext(ctx).Create(folder).Error
}

// GetByID retrieves a folder by ID
func (r *FolderRepository) GetByID(ctx context.Context, id uint) (*model.Folder, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).First(&folder, id).Error
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

// GetByAccountID retrieves all folders for an account
func (r *FolderRepository) GetByAccountID(ctx context.Context, accountID uint) ([]*model.Folder, error) {
	var folders []*model.Folder
	err := r.db.WithContext(ctx).
		Where("account_id = ?", accountID).
		Order("sort_order ASC, name ASC").
		Find(&folders).Error
	return folders, err
}

// CountEmailsByFolder counts total emails in a folder
func (r *FolderRepository) CountEmailsByFolder(ctx context.Context, accountID uint, folderName string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Email{}).
		Where("account_id = ? AND folder_name = ?", accountID, folderName).
		Count(&count).Error
	return count, err
}

// CountUnreadEmailsByFolder counts unread emails in a folder
func (r *FolderRepository) CountUnreadEmailsByFolder(ctx context.Context, accountID uint, folderName string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Email{}).
		Where("account_id = ? AND folder_name = ? AND is_read = ?", accountID, folderName, false).
		Count(&count).Error
	return count, err
}

// GetByName retrieves a folder by name and account ID
func (r *FolderRepository) GetByName(ctx context.Context, name string, accountID uint) (*model.Folder, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).
		Where("account_id = ? AND name = ?", accountID, name).
		First(&folder).Error
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

// GetByAccountAndRawName retrieves a folder by account ID and raw name
func (r *FolderRepository) GetByAccountAndRawName(ctx context.Context, accountID uint, rawName string) (*model.Folder, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).
		Where("account_id = ? AND raw_name = ?", accountID, rawName).
		First(&folder).Error
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

// Update updates a folder
func (r *FolderRepository) Update(ctx context.Context, folder *model.Folder) error {
	return r.db.WithContext(ctx).Save(folder).Error
}

// UpdateFields updates specific fields of a folder
func (r *FolderRepository) UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&model.Folder{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateCounts updates email counts for a folder
func (r *FolderRepository) UpdateCounts(ctx context.Context, folderID uint, emailCount, unreadCount int64) error {
	return r.db.WithContext(ctx).
		Model(&model.Folder{}).
		Where("id = ?", folderID).
		Updates(map[string]interface{}{
			"email_count":  emailCount,
			"unread_count": unreadCount,
		}).Error
}

// Delete deletes a folder
func (r *FolderRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.Folder{}, id).Error
}

// SyncFolders syncs folders for an account, creating/updating as needed
func (r *FolderRepository) SyncFolders(ctx context.Context, accountID uint, folders []*model.Folder) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Get existing folders
		var existingFolders []*model.Folder
		if err := tx.Where("account_id = ?", accountID).Find(&existingFolders).Error; err != nil {
			return err
		}

		// Create a map for quick lookup
		existingMap := make(map[string]*model.Folder)
		for _, f := range existingFolders {
			existingMap[f.RawName] = f
		}

		// Process each folder
		for _, folder := range folders {
			folder.AccountID = accountID

			if existing, ok := existingMap[folder.RawName]; ok {
				// Update existing folder, but preserve sync state fields
				updates := map[string]interface{}{
					"name":       folder.Name,
					"delimiter":  folder.Delimiter,
					"flags":      folder.Flags,
					"type":       folder.Type,
					"updated_at": time.Now(),
				}
				if err := tx.Model(&model.Folder{}).Where("id = ?", existing.FolderID).Updates(updates).Error; err != nil {
					return err
				}
				delete(existingMap, folder.RawName)
			} else {
				// Create new folder
				if err := tx.Create(folder).Error; err != nil {
					return err
				}
			}
		}

		// Delete folders that no longer exist on the server
		for _, folder := range existingMap {
			if err := tx.Delete(folder).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// RepairInvalidUIDValidity repairs folders with invalid UIDVALIDITY
func (r *FolderRepository) RepairInvalidUIDValidity(ctx context.Context, accountID uint) error {
	var folders []*model.Folder
	err := r.db.WithContext(ctx).
		Where("account_id = ? AND uid_validity = 0", accountID).
		Find(&folders).Error

	if err != nil {
		return err
	}

	for _, folder := range folders {
		// Reset sync state to force re-sync
		updates := map[string]interface{}{
			"last_synced_uid": 0,
			"last_sync_at":    nil,
		}
		if err := r.db.WithContext(ctx).Model(&model.Folder{}).
			Where("id = ?", folder.FolderID).
			Updates(updates).Error; err != nil {
			return err
		}
	}

	return nil
}
