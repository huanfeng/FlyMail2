package message

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Repository interface for email message data access
type Repository interface {
	Create(ctx context.Context, email *Email) error
	CreateBatch(ctx context.Context, emails []*Email) error
	GetByID(ctx context.Context, id uint, userID uint) (*Email, error)
	GetByUID(ctx context.Context, accountID uint, folderName string, uid uint32) (*Email, error)
	GetByMessageID(ctx context.Context, accountID uint, messageID string) (*Email, error)
	GetList(ctx context.Context, userID uint, filter *EmailFilter) ([]*Email, int64, error)
	Update(ctx context.Context, email *Email) error
	UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error
	Delete(ctx context.Context, id uint) error
	DeleteByUIDs(ctx context.Context, accountID uint, folderName string, uids []uint32) error
	GetLatestUID(ctx context.Context, accountID uint, folderName string) (uint32, error)
	GetUIDList(ctx context.Context, accountID uint, folderName string) ([]uint32, error)
	CountByAccount(ctx context.Context, accountID uint) (int64, error)
	CountUnreadByAccount(ctx context.Context, accountID uint) (int64, error)
	CountByFolder(ctx context.Context, accountID uint, folderName string) (int64, error)
	CountUnreadByFolder(ctx context.Context, accountID uint, folderName string) (int64, error)
	GetFolderStats(ctx context.Context, accountID uint) (map[string]struct{ Total, Unread int64 }, error)
	// Attachment methods
	CreateAttachment(ctx context.Context, attachment *Attachment) error
	GetAttachmentsByEmailID(ctx context.Context, emailID uint) ([]*Attachment, error)
	GetAttachmentByID(ctx context.Context, id uint) (*Attachment, error)
	DeleteAttachmentsByEmailID(ctx context.Context, emailID uint) error
}

// repository implements Repository interface
type repository struct {
	db *gorm.DB
}

// NewRepository creates a new email repository
func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

// Create creates a new email
func (r *repository) Create(ctx context.Context, email *Email) error {
	return r.db.WithContext(ctx).Create(email).Error
}

// CreateBatch creates multiple emails in a batch
func (r *repository) CreateBatch(ctx context.Context, emails []*Email) error {
	if len(emails) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(emails, 100).Error
}

// GetByID retrieves an email by ID
func (r *repository) GetByID(ctx context.Context, id uint, userID uint) (*Email, error) {
	var email Email
	query := r.db.WithContext(ctx).Where("id = ?", id)

	// If userID is provided, ensure the email belongs to the user
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

// GetByUID retrieves an email by UID
func (r *repository) GetByUID(ctx context.Context, accountID uint, folderName string, uid uint32) (*Email, error) {
	var email Email
	err := r.db.WithContext(ctx).
		Where("account_id = ? AND folder_name = ? AND uid = ?", accountID, folderName, uid).
		First(&email).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &email, nil
}

// GetByMessageID retrieves an email by message ID
func (r *repository) GetByMessageID(ctx context.Context, accountID uint, messageID string) (*Email, error) {
	var email Email
	err := r.db.WithContext(ctx).
		Where("account_id = ? AND email_id = ?", accountID, messageID).
		First(&email).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &email, nil
}

// GetList retrieves a paginated list of emails
func (r *repository) GetList(ctx context.Context, userID uint, filter *EmailFilter) ([]*Email, int64, error) {
	query := r.db.WithContext(ctx).Model(&Email{})

	// Join with accounts to filter by user
	query = query.Joins("JOIN email_accounts ON emails.account_id = email_accounts.id").
		Where("email_accounts.user_id = ?", userID)

	// Apply filters
	if filter.AccountID > 0 {
		query = query.Where("emails.account_id = ?", filter.AccountID)
	}

	if filter.FolderID > 0 {
		query = query.Where("emails.folder_id = ?", filter.FolderID)
	} else if filter.FolderName != "" {
		query = query.Where("emails.folder_name = ?", filter.FolderName)
	}

	// Virtual folder support
	if filter.VirtualFolder != "" {
		switch filter.VirtualFolder {
		case "all-inbox":
			query = query.Where("emails.folder_type = ?", 1) // FolderTypeInbox
		case "all-sent":
			query = query.Where("emails.folder_type IN (?)", []int{2, 7}) // FolderTypeSent, FolderTypeSentMail
		case "all-drafts":
			query = query.Where("emails.folder_type = ?", 3) // FolderTypeDrafts
		case "all-trash":
			query = query.Where("emails.folder_type IN (?)", []int{4, 8}) // FolderTypeTrash, FolderTypeDeleted
		case "all-starred":
			query = query.Where("emails.is_starred = ?", true)
		case "all-unread":
			query = query.Where("emails.is_read = ?", false)
		}
	}

	if filter.IsRead != nil {
		query = query.Where("emails.is_read = ?", *filter.IsRead)
	}

	if filter.IsStarred != nil {
		query = query.Where("emails.is_starred = ?", *filter.IsStarred)
	}

	// Search functionality
	if filter.Search != "" {
		searchTerm := "%" + filter.Search + "%"
		query = query.Where("emails.subject LIKE ? OR emails.from LIKE ? OR emails.to LIKE ? OR emails.body LIKE ?",
			searchTerm, searchTerm, searchTerm, searchTerm)
	}

	// Count total before pagination
	var total int64
	countQuery := *query
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply sorting
	sortBy := "emails.date"
	if filter.SortBy == "subject" {
		sortBy = "emails.subject"
	}
	sortOrder := "DESC"
	if strings.ToUpper(filter.SortOrder) == "ASC" {
		sortOrder = "ASC"
	}
	query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder))

	// Apply pagination
	offset := (filter.Page - 1) * filter.PageSize
	query = query.Offset(offset).Limit(filter.PageSize)

	// Execute query
	var emails []*Email
	if err := query.Find(&emails).Error; err != nil {
		return nil, 0, err
	}

	return emails, total, nil
}

// Update updates an email
func (r *repository) Update(ctx context.Context, email *Email) error {
	return r.db.WithContext(ctx).Save(email).Error
}

// UpdateFields updates specific fields of an email
func (r *repository) UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&Email{}).Where("id = ?", id).Updates(updates).Error
}

// Delete deletes an email
func (r *repository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&Email{}, id).Error
}

// DeleteByUIDs deletes emails by UIDs
func (r *repository) DeleteByUIDs(ctx context.Context, accountID uint, folderName string, uids []uint32) error {
	if len(uids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("account_id = ? AND folder_name = ? AND uid IN ?", accountID, folderName, uids).
		Delete(&Email{}).Error
}

// GetLatestUID gets the latest UID for a folder
func (r *repository) GetLatestUID(ctx context.Context, accountID uint, folderName string) (uint32, error) {
	var uid uint32
	err := r.db.WithContext(ctx).Model(&Email{}).
		Where("account_id = ? AND folder_name = ?", accountID, folderName).
		Select("MAX(uid)").
		Scan(&uid).Error
	return uid, err
}

// GetUIDList gets all UIDs for a folder
func (r *repository) GetUIDList(ctx context.Context, accountID uint, folderName string) ([]uint32, error) {
	var uids []uint32
	err := r.db.WithContext(ctx).Model(&Email{}).
		Where("account_id = ? AND folder_name = ?", accountID, folderName).
		Pluck("uid", &uids).Error
	return uids, err
}

// CountByAccount counts emails for an account
func (r *repository) CountByAccount(ctx context.Context, accountID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Email{}).
		Where("account_id = ?", accountID).
		Count(&count).Error
	return count, err
}

// CountUnreadByAccount counts unread emails for an account
func (r *repository) CountUnreadByAccount(ctx context.Context, accountID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Email{}).
		Where("account_id = ? AND is_read = ?", accountID, false).
		Count(&count).Error
	return count, err
}

// CountByFolder counts emails in a folder
func (r *repository) CountByFolder(ctx context.Context, accountID uint, folderName string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Email{}).
		Where("account_id = ? AND folder_name = ?", accountID, folderName).
		Count(&count).Error
	return count, err
}

// CountUnreadByFolder counts unread emails in a folder
func (r *repository) CountUnreadByFolder(ctx context.Context, accountID uint, folderName string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Email{}).
		Where("account_id = ? AND folder_name = ? AND is_read = ?", accountID, folderName, false).
		Count(&count).Error
	return count, err
}

// GetFolderStats gets statistics for all folders in an account
func (r *repository) GetFolderStats(ctx context.Context, accountID uint) (map[string]struct{ Total, Unread int64 }, error) {
	type FolderStat struct {
		FolderName string
		Total      int64
		Unread     int64
	}

	var stats []FolderStat
	err := r.db.WithContext(ctx).Model(&Email{}).
		Select("folder_name, COUNT(*) as total, COUNT(CASE WHEN is_read = false THEN 1 END) as unread").
		Where("account_id = ?", accountID).
		Group("folder_name").
		Scan(&stats).Error

	if err != nil {
		return nil, err
	}

	result := make(map[string]struct{ Total, Unread int64 })
	for _, stat := range stats {
		result[stat.FolderName] = struct{ Total, Unread int64 }{
			Total:  stat.Total,
			Unread: stat.Unread,
		}
	}

	return result, nil
}

// CreateAttachment creates a new attachment
func (r *repository) CreateAttachment(ctx context.Context, attachment *Attachment) error {
	return r.db.WithContext(ctx).Create(attachment).Error
}

// GetAttachmentsByEmailID gets all attachments for an email
func (r *repository) GetAttachmentsByEmailID(ctx context.Context, emailID uint) ([]*Attachment, error) {
	var attachments []*Attachment
	err := r.db.WithContext(ctx).
		Where("email_id = ?", emailID).
		Find(&attachments).Error
	return attachments, err
}

// GetAttachmentByID gets an attachment by ID
func (r *repository) GetAttachmentByID(ctx context.Context, id uint) (*Attachment, error) {
	var attachment Attachment
	err := r.db.WithContext(ctx).First(&attachment, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("attachment not found")
		}
		return nil, err
	}
	return &attachment, nil
}

// DeleteAttachmentsByEmailID deletes all attachments for an email
func (r *repository) DeleteAttachmentsByEmailID(ctx context.Context, emailID uint) error {
	return r.db.WithContext(ctx).
		Where("email_id = ?", emailID).
		Delete(&Attachment{}).Error
}
