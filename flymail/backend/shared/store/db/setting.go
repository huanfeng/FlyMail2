package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"flymail/shared/store"
	"flymail/shared/store/model"
)

// SettingRepository implements the setting repository interface
type SettingRepository struct {
	db *gorm.DB
}

// NewSettingRepository creates a new setting repository
func NewSettingRepository(db *gorm.DB) store.SettingRepository {
	return &SettingRepository{db: db}
}

// Get retrieves a setting by key
func (r *SettingRepository) Get(ctx context.Context, key string) (*model.Setting, error) {
	var setting model.Setting
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
func (r *SettingRepository) GetAll(ctx context.Context) ([]*model.Setting, error) {
	var settings []*model.Setting
	err := r.db.WithContext(ctx).Order("key ASC").Find(&settings).Error
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// Set creates or updates a setting
func (r *SettingRepository) Set(ctx context.Context, key, value string) error {
	setting := &model.Setting{
		Key:   key,
		Value: value,
	}

	// Use FirstOrCreate with assignment to handle upsert
	err := r.db.WithContext(ctx).Where("key = ?", key).Assign(model.Setting{Value: value}).FirstOrCreate(setting).Error
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
func (r *SettingRepository) Delete(ctx context.Context, key string) error {
	result := r.db.WithContext(ctx).Where("key = ?", key).Delete(&model.Setting{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("setting not found")
	}
	return nil
}

// GetMultiple retrieves multiple settings by keys
func (r *SettingRepository) GetMultiple(ctx context.Context, keys []string) ([]*model.Setting, error) {
	var settings []*model.Setting
	err := r.db.WithContext(ctx).Where("key IN ?", keys).Find(&settings).Error
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// SetMultiple sets multiple settings at once
func (r *SettingRepository) SetMultiple(ctx context.Context, settings map[string]string) error {
	// Use transaction for atomic updates
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for key, value := range settings {
			setting := &model.Setting{
				Key:   key,
				Value: value,
			}

			// Upsert each setting
			if err := tx.Where("key = ?", key).Assign(model.Setting{Value: value}).FirstOrCreate(setting).Error; err != nil {
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
