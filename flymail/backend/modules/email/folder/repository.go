package folder

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// Repository interface for folder data access
type Repository interface {
	Create(ctx context.Context, folder *Folder) error
	GetByID(ctx context.Context, id uint) (*Folder, error)
	GetByAccountID(ctx context.Context, accountID uint) ([]*Folder, error)
	GetByName(ctx context.Context, name string, accountID uint) (*Folder, error)
	GetByAccountAndRawName(ctx context.Context, accountID uint, rawName string) (*Folder, error)
	Update(ctx context.Context, folder *Folder) error
	UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error
	UpdateCounts(ctx context.Context, folderID uint, emailCount, unreadCount int64) error
	Delete(ctx context.Context, id uint) error
	SyncFolders(ctx context.Context, accountID uint, folders []*Folder) error
	RepairInvalidUIDValidity(ctx context.Context, accountID uint) error
}

// EmailCounter interface for counting emails in folders
type EmailCounter interface {
	CountByFolder(ctx context.Context, accountID uint, folderName string) (int64, error)
	CountUnreadByFolder(ctx context.Context, accountID uint, folderName string) (int64, error)
}

// repository implements Repository interface
type repository struct {
	db           *gorm.DB
	emailCounter EmailCounter
}

// NewRepository creates a new folder repository
func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

// SetEmailCounter sets the email counter (to avoid circular dependency)
func (r *repository) SetEmailCounter(counter EmailCounter) {
	r.emailCounter = counter
}

// Create creates a new folder
func (r *repository) Create(ctx context.Context, folder *Folder) error {
	return r.db.WithContext(ctx).Create(folder).Error
}

// GetByID retrieves a folder by ID
func (r *repository) GetByID(ctx context.Context, id uint) (*Folder, error) {
	var folder Folder
	err := r.db.WithContext(ctx).First(&folder, id).Error
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

// GetByAccountID retrieves all folders for an account
func (r *repository) GetByAccountID(ctx context.Context, accountID uint) ([]*Folder, error) {
	var folders []*Folder
	err := r.db.WithContext(ctx).
		Where("account_id = ?", accountID).
		Order("sort_order ASC, name ASC").
		Find(&folders).Error
	return folders, err
}

// GetByName retrieves a folder by name and account ID
func (r *repository) GetByName(ctx context.Context, name string, accountID uint) (*Folder, error) {
	var folder Folder
	err := r.db.WithContext(ctx).
		Where("account_id = ? AND name = ?", accountID, name).
		First(&folder).Error
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

// GetByAccountAndRawName retrieves a folder by account ID and raw name
func (r *repository) GetByAccountAndRawName(ctx context.Context, accountID uint, rawName string) (*Folder, error) {
	var folder Folder
	err := r.db.WithContext(ctx).
		Where("account_id = ? AND raw_name = ?", accountID, rawName).
		First(&folder).Error
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

// Update updates a folder
func (r *repository) Update(ctx context.Context, folder *Folder) error {
	return r.db.WithContext(ctx).Save(folder).Error
}

// UpdateFields updates specific fields of a folder
func (r *repository) UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&Folder{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateCounts updates email counts for a folder
func (r *repository) UpdateCounts(ctx context.Context, folderID uint, emailCount, unreadCount int64) error {
	return r.db.WithContext(ctx).
		Model(&Folder{}).
		Where("id = ?", folderID).
		Updates(map[string]interface{}{
			"email_count":  emailCount,
			"unread_count": unreadCount,
		}).Error
}

// Delete deletes a folder
func (r *repository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&Folder{}, id).Error
}

// SyncFolders syncs folders for an account, creating/updating as needed
func (r *repository) SyncFolders(ctx context.Context, accountID uint, folders []*Folder) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Get existing folders
		var existingFolders []*Folder
		if err := tx.Where("account_id = ?", accountID).Find(&existingFolders).Error; err != nil {
			return err
		}

		// Create a map for quick lookup
		existingMap := make(map[string]*Folder)
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
				if err := tx.Model(&Folder{}).Where("id = ?", existing.FolderID).Updates(updates).Error; err != nil {
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
func (r *repository) RepairInvalidUIDValidity(ctx context.Context, accountID uint) error {
	var folders []*Folder
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
		if err := r.db.WithContext(ctx).Model(&Folder{}).
			Where("id = ?", folder.FolderID).
			Updates(updates).Error; err != nil {
			return err
		}
	}

	return nil
}
