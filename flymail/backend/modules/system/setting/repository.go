package setting

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Repository interface for setting data access
type Repository interface {
	Get(ctx context.Context, key string) (*Setting, error)
	GetAll(ctx context.Context) ([]*Setting, error)
	Set(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
	GetMultiple(ctx context.Context, keys []string) ([]*Setting, error)
	SetMultiple(ctx context.Context, settings map[string]string) error
}

// repository implements Repository interface
type repository struct {
	db *gorm.DB
}

// NewRepository creates a new setting repository
func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

// Get retrieves a setting by key
func (r *repository) Get(ctx context.Context, key string) (*Setting, error) {
	var setting Setting
	err := r.db.WithContext(ctx).Where("key = ?", key).First(&setting).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Return nil without error for non-existent settings
		}
		return nil, err
	}
	return &setting, nil
}

// GetAll retrieves all settings
func (r *repository) GetAll(ctx context.Context) ([]*Setting, error) {
	var settings []*Setting
	err := r.db.WithContext(ctx).Order("key ASC").Find(&settings).Error
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// Set creates or updates a setting
func (r *repository) Set(ctx context.Context, key, value string) error {
	setting := &Setting{
		Key:   key,
		Value: value,
	}

	// Use FirstOrCreate with assignment to handle upsert
	err := r.db.WithContext(ctx).Where("key = ?", key).Assign(Setting{Value: value}).FirstOrCreate(setting).Error
	if err != nil {
		return err
	}

	// If the record already existed, update it
	if setting.ID != 0 && setting.Value != value {
		setting.Value = value
		setting.UpdatedAt = time.Now()
		return r.db.WithContext(ctx).Save(setting).Error
	}

	return nil
}

// Delete deletes a setting by key
func (r *repository) Delete(ctx context.Context, key string) error {
	result := r.db.WithContext(ctx).Where("key = ?", key).Delete(&Setting{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("setting not found")
	}
	return nil
}

// GetMultiple retrieves multiple settings by keys
func (r *repository) GetMultiple(ctx context.Context, keys []string) ([]*Setting, error) {
	var settings []*Setting
	err := r.db.WithContext(ctx).Where("key IN ?", keys).Find(&settings).Error
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// SetMultiple sets multiple settings at once
func (r *repository) SetMultiple(ctx context.Context, settings map[string]string) error {
	// Use transaction for atomic updates
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for key, value := range settings {
			setting := &Setting{
				Key:   key,
				Value: value,
			}

			// Upsert each setting
			if err := tx.Where("key = ?", key).Assign(Setting{Value: value}).FirstOrCreate(setting).Error; err != nil {
				return err
			}

			// If the record already existed, update it
			if setting.ID != 0 && setting.Value != value {
				setting.Value = value
				setting.UpdatedAt = time.Now()
				if err := tx.Save(setting).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}
