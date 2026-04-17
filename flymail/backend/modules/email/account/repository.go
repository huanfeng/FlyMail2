package account

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"flymail/pkg/logger"
)

// Repository interface for email account data access
type Repository interface {
	Create(ctx context.Context, account *EmailAccount) error
	GetByID(ctx context.Context, id uint, userID uint) (*EmailAccount, error)
	GetByUserID(ctx context.Context, userID uint) ([]*EmailAccount, error)
	Update(ctx context.Context, account *EmailAccount) error
	UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error
	Delete(ctx context.Context, id uint) error
	Count(ctx context.Context) (int64, error)
	CountByUserID(ctx context.Context, userID uint) (int64, error)
	GetActiveAccounts(ctx context.Context) ([]*EmailAccount, error)
	GetAll(ctx context.Context) ([]*EmailAccount, error)
}

// repository implements Repository interface
type repository struct {
	db *gorm.DB
}

// NewRepository creates a new account repository
func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

// Create creates a new email account
func (r *repository) Create(ctx context.Context, account *EmailAccount) error {
	logger.Debug("Repository: Creating account",
		zap.String("name", account.Name),
		zap.String("email", account.Email),
		zap.String("username", account.Username),
		zap.Bool("has_password", account.Password != ""),
		zap.Int("password_length", len(account.Password)),
	)

	err := r.db.WithContext(ctx).Create(account).Error
	if err != nil {
		logger.Error("Repository: Failed to create account", zap.Error(err))
		return err
	}

	// Verify the account was created with password
	var created EmailAccount
	if err := r.db.WithContext(ctx).Where("id = ?", account.AccountID).First(&created).Error; err == nil {
		logger.Debug("Repository: Verified created account",
			zap.Uint("id", created.AccountID),
			zap.Bool("has_password_in_db", created.Password != ""),
		)
	}

	return nil
}

// GetByID retrieves an email account by ID
func (r *repository) GetByID(ctx context.Context, id uint, userID uint) (*EmailAccount, error) {
	var account EmailAccount
	query := r.db.WithContext(ctx).Where("id = ?", id)

	// If userID is provided, filter by user
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}

	err := query.First(&account).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("account not found")
		}
		return nil, err
	}

	return &account, nil
}

// GetByUserID retrieves all email accounts for a user
func (r *repository) GetByUserID(ctx context.Context, userID uint) ([]*EmailAccount, error) {
	var accounts []*EmailAccount
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("sort_order ASC, created_at ASC").Find(&accounts).Error
	return accounts, err
}

// Update updates an email account
func (r *repository) Update(ctx context.Context, account *EmailAccount) error {
	return r.db.WithContext(ctx).Save(account).Error
}

// UpdateFields updates specific fields of an email account
func (r *repository) UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&EmailAccount{}).Where("id = ?", id).Updates(updates).Error
}

// Delete deletes an email account
func (r *repository) Delete(ctx context.Context, id uint) error {
	// This will cascade delete related emails due to foreign key constraints
	return r.db.WithContext(ctx).Delete(&EmailAccount{}, id).Error
}

// Count returns the total number of accounts
func (r *repository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&EmailAccount{}).Count(&count).Error
	return count, err
}

// CountByUserID returns the number of accounts for a user
func (r *repository) CountByUserID(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&EmailAccount{}).
		Where("user_id = ?", userID).
		Count(&count).Error
	return count, err
}

// GetActiveAccounts returns all active email accounts
func (r *repository) GetActiveAccounts(ctx context.Context) ([]*EmailAccount, error) {
	var accounts []*EmailAccount
	err := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Find(&accounts).Error
	return accounts, err
}

// GetAll retrieves all email accounts
func (r *repository) GetAll(ctx context.Context) ([]*EmailAccount, error) {
	var accounts []*EmailAccount
	err := r.db.WithContext(ctx).Find(&accounts).Error
	return accounts, err
}
