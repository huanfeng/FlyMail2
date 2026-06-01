package setting

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 负责 settings 表的 CRUD。
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Get 返回 key 对应的 value。第二个返回值 found=false 表示键不存在。
func (r *Repository) Get(key string) (string, bool, error) {
	var s Setting
	err := r.db.First(&s, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return s.Value, true, nil
}

// Set 插入或更新一条键值记录（upsert）。
func (r *Repository) Set(key, value string) error {
	s := Setting{Key: key, Value: value}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&s).Error
}

// All 返回所有已存储的键值对。
func (r *Repository) All() (map[string]string, error) {
	var list []Setting
	if err := r.db.Find(&list).Error; err != nil {
		return nil, err
	}
	m := make(map[string]string, len(list))
	for _, s := range list {
		m[s.Key] = s.Value
	}
	return m, nil
}
