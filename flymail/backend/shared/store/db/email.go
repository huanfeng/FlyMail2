package db

import (
	"context"
	"errors"

	"gorm.io/gorm"

	serviceInterfaces "flymail/modules/service"
	repoInterfaces "flymail/shared/store"
	"flymail/shared/store/model"
)

// EmailRepository implements the email repository interface
type EmailRepository struct {
	db *gorm.DB
}

// NewEmailRepository creates a new email repository
func NewEmailRepository(db *gorm.DB) repoInterfaces.EmailRepository {
	return &EmailRepository{db: db}
}

// Create creates a new email
func (r *EmailRepository) Create(ctx context.Context, email *model.Email) error {
	return r.db.WithContext(ctx).Create(email).Error
}

// GetByID retrieves an email by ID
func (r *EmailRepository) GetByID(ctx context.Context, id uint) (*model.Email, error) {
	var email model.Email
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&email).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("email not found")
		}
		return nil, err
	}
	return &email, nil
}

// GetByMessageID retrieves an email by message ID
func (r *EmailRepository) GetByMessageID(ctx context.Context, messageID string, accountID uint) (*model.Email, error) {
	var email model.Email
	err := r.db.WithContext(ctx).
		Where("email_id = ? AND account_id = ?", messageID, accountID).
		First(&email).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Return nil without error for non-existent emails
		}
		return nil, err
	}

	return &email, nil
}

// GetByUID retrieves an email by UID, account ID and folder name
func (r *EmailRepository) GetByUID(ctx context.Context, accountID uint, folderName string, uid uint32) (*model.Email, error) {
	var email model.Email
	err := r.db.WithContext(ctx).
		Where("account_id = ? AND folder_name = ? AND uid = ?", accountID, folderName, uid).
		First(&email).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Return nil without error for non-existent emails
		}
		return nil, err
	}

	return &email, nil
}

// List retrieves emails with filtering and pagination
func (r *EmailRepository) List(ctx context.Context, userID uint, filterInterface interface{}) ([]model.Email, int64, error) {
	filter := filterInterface.(*serviceInterfaces.EmailFilter)
	var emails []model.Email
	var total int64

	// Base query
	query := r.db.WithContext(ctx).Model(&model.Email{}).
		Joins("JOIN email_accounts ON emails.account_id = email_accounts.id").
		Where("email_accounts.user_id = ?", userID)

	// Handle virtual folders first
	if filter.VirtualFolder != "" {
		switch filter.VirtualFolder {
		case "all-inbox":
			// All emails from inbox folders across all accounts
			query = query.Where("emails.folder_type = ?", model.FolderTypeInbox)
		case "all-starred":
			// All starred emails across all accounts
			query = query.Where("emails.is_starred = ?", true)
		case "all-unread":
			// All unread emails across all accounts
			query = query.Where("emails.is_read = ?", false)
		case "all-sent":
			// All sent emails across all accounts
			query = query.Where("emails.folder_type = ?", model.FolderTypeSent)
		case "all-drafts":
			// All draft emails across all accounts
			query = query.Where("emails.folder_type = ?", model.FolderTypeDrafts)
		case "all-trash":
			// All trash emails across all accounts
			query = query.Where("emails.folder_type = ?", model.FolderTypeTrash)
		case "all-junk":
			// All junk/spam emails across all accounts
			query = query.Where("emails.folder_type = ?", model.FolderTypeJunk)
		case "all-archive":
			// All archived emails across all accounts
			query = query.Where("emails.folder_type = ?", model.FolderTypeArchive)
		default:
			// Unknown virtual folder, return empty result
			return emails, 0, nil
		}
	} else {
		// Apply regular filters when not using virtual folders
		if filter.AccountID > 0 {
			query = query.Where("emails.account_id = ?", filter.AccountID)
		}

		// FolderID takes priority over FolderName
		if filter.FolderID > 0 {
			query = query.Joins("JOIN folders ON emails.folder_name = folders.name AND emails.account_id = folders.account_id").
				Where("folders.id = ?", filter.FolderID)
		} else if filter.FolderName != "" {
			query = query.Where("emails.folder_name = ?", filter.FolderName)
		}
	}

	// Apply common filters
	if filter.IsRead != nil {
		query = query.Where("emails.is_read = ?", *filter.IsRead)
	}

	if filter.IsStarred != nil {
		query = query.Where("emails.is_starred = ?", *filter.IsStarred)
	}

	if filter.Search != "" {
		searchPattern := "%" + filter.Search + "%"
		query = query.Where("(emails.subject LIKE ? OR emails.body LIKE ? OR emails.from LIKE ? OR emails.to LIKE ?)",
			searchPattern, searchPattern, searchPattern, searchPattern)
	}

	// Count total before pagination
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply sorting
	orderClause := "emails.date DESC" // default
	if filter.SortBy != "" && filter.SortOrder != "" {
		// Validate sort order for security
		var sortOrder string
		if filter.SortOrder == "asc" || filter.SortOrder == "ASC" {
			sortOrder = "ASC"
		} else {
			sortOrder = "DESC"
		}

		switch filter.SortBy {
		case "date":
			orderClause = "emails.date " + sortOrder
		case "subject":
			orderClause = "emails.subject " + sortOrder
		default:
			orderClause = "emails.date DESC" // fallback to default
		}
	}
	query = query.Order(orderClause)

	// Apply pagination
	offset := (filter.Page - 1) * filter.PageSize
	query = query.Offset(offset).Limit(filter.PageSize)

	// Execute query
	if err := query.Find(&emails).Error; err != nil {
		return nil, 0, err
	}

	return emails, total, nil
}

// Update updates an email
func (r *EmailRepository) Update(ctx context.Context, email *model.Email) error {
	return r.db.WithContext(ctx).Save(email).Error
}

// UpdateFields updates specific fields of an email
func (r *EmailRepository) UpdateFields(ctx context.Context, id uint, fields map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&model.Email{}).Where("id = ?", id).Updates(fields).Error
}

// Delete deletes an email
func (r *EmailRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.Email{}, id).Error
}

// CountByAccount counts emails for a specific account
func (r *EmailRepository) CountByAccount(ctx context.Context, accountID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Email{}).
		Where("account_id = ?", accountID).
		Count(&count).Error
	return count, err
}

// CountUnreadByAccount counts unread emails for a specific account
func (r *EmailRepository) CountUnreadByAccount(ctx context.Context, accountID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Email{}).
		Where("account_id = ? AND is_read = ?", accountID, false).
		Count(&count).Error
	return count, err
}

// GetTotalSizeByAccount gets total size of emails for an account
func (r *EmailRepository) GetTotalSizeByAccount(ctx context.Context, accountID uint) (int64, error) {
	var totalSize int64
	err := r.db.WithContext(ctx).Model(&model.Email{}).
		Where("account_id = ?", accountID).
		Select("COALESCE(SUM(size), 0)").
		Scan(&totalSize).Error
	return totalSize, err
}

// GetByIDWithAccount retrieves an email by ID with account preloaded
func (r *EmailRepository) GetByIDWithAccount(ctx context.Context, id uint, userID uint) (*model.Email, error) {
	var email model.Email
	query := r.db.WithContext(ctx).Preload("Account").Where("emails.id = ?", id)

	// If userID is provided, join with accounts to verify ownership
	if userID > 0 {
		query = query.Joins("JOIN email_accounts ON emails.account_id = email_accounts.id").
			Where("email_accounts.user_id = ?", userID)
	}

	err := query.First(&email).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("email not found")
		}
		return nil, err
	}

	return &email, nil
}

// BulkCreate creates multiple emails in a single transaction
func (r *EmailRepository) BulkCreate(ctx context.Context, emails []*model.Email) error {
	if len(emails) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Create(&emails).Error
}

// GetByAccountID retrieves emails by account ID with pagination
func (r *EmailRepository) GetByAccountID(ctx context.Context, accountID uint, limit, offset int) ([]*model.Email, error) {
	var emails []*model.Email
	err := r.db.WithContext(ctx).
		Where("account_id = ?", accountID).
		Order("date DESC").
		Limit(limit).
		Offset(offset).
		Find(&emails).Error
	return emails, err
}

// GetLatestByAccountID gets the latest email for an account
func (r *EmailRepository) GetLatestByAccountID(ctx context.Context, accountID uint) (*model.Email, error) {
	var email model.Email
	err := r.db.WithContext(ctx).
		Where("account_id = ?", accountID).
		Order("date DESC").
		First(&email).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &email, nil
}
