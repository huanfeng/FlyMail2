package db

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"flymail-core/logger"
	"flymail/shared/store"
	"flymail/shared/store/model"
)

// AccountRepository implements the account repository interface
type AccountRepository struct {
	db *gorm.DB
}

// NewAccountRepository creates a new account repository
func NewAccountRepository(db *gorm.DB) store.AccountRepository {
	return &AccountRepository{db: db}
}

// Create creates a new email account
func (r *AccountRepository) Create(ctx context.Context, account *model.EmailAccount) error {
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
	var created model.EmailAccount
	if err := r.db.WithContext(ctx).Where("id = ?", account.AccountID).First(&created).Error; err == nil {
		logger.Debug("Repository: Verified created account",
			zap.Uint("id", created.AccountID),
			zap.Bool("has_password_in_db", created.Password != ""),
		)
	}

	return nil
}

// GetByID retrieves an email account by ID
func (r *AccountRepository) GetByID(ctx context.Context, id uint, userID uint) (*model.EmailAccount, error) {
	var account model.EmailAccount
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
func (r *AccountRepository) GetByUserID(ctx context.Context, userID uint) ([]*model.EmailAccount, error) {
	var accounts []*model.EmailAccount
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("sort_order ASC, created_at ASC").Find(&accounts).Error
	return accounts, err
}

// Update updates an email account
func (r *AccountRepository) Update(ctx context.Context, account *model.EmailAccount) error {
	return r.db.WithContext(ctx).Save(account).Error
}

// UpdateFields updates specific fields of an email account
func (r *AccountRepository) UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&model.EmailAccount{}).Where("id = ?", id).Updates(updates).Error
}

// Delete deletes an email account
func (r *AccountRepository) Delete(ctx context.Context, id uint) error {
	// This will cascade delete related emails due to foreign key constraints
	return r.db.WithContext(ctx).Delete(&model.EmailAccount{}, id).Error
}

// Count returns the total number of accounts
func (r *AccountRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.EmailAccount{}).Count(&count).Error
	return count, err
}

// CountByUserID returns the number of accounts for a user
func (r *AccountRepository) CountByUserID(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.EmailAccount{}).
		Where("user_id = ?", userID).
		Count(&count).Error
	return count, err
}

// GetActiveAccounts returns all active email accounts
func (r *AccountRepository) GetActiveAccounts(ctx context.Context) ([]*model.EmailAccount, error) {
	var accounts []*model.EmailAccount
	err := r.db.WithContext(ctx).
		Where("is_active = ?", true).
		Find(&accounts).Error
	return accounts, err
}

// GetAll retrieves all email accounts
func (r *AccountRepository) GetAll(ctx context.Context) ([]*model.EmailAccount, error) {
	var accounts []*model.EmailAccount
	err := r.db.WithContext(ctx).Find(&accounts).Error
	return accounts, err
}
