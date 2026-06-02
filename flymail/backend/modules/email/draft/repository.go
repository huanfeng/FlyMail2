package draft

import (
	"errors"

	"gorm.io/gorm"
)

// ErrDraftNotFound 草稿不存在时返回此错误。
var ErrDraftNotFound = errors.New("draft not found")

// Repository 草稿数据访问层。
type Repository struct{ db *gorm.DB }

// NewRepository 构建 Repository。
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Create 持久化新草稿并回填 ID。
func (r *Repository) Create(d *Draft) error {
	return r.db.Create(d).Error
}

// Update 全量更新草稿字段。
func (r *Repository) Update(d *Draft) error {
	return r.db.Save(d).Error
}

// GetByID 按主键查询草稿；不存在时返回 ErrDraftNotFound。
func (r *Repository) GetByID(id uint) (*Draft, error) {
	var d Draft
	err := r.db.First(&d, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrDraftNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ListByAccount 列出指定账户的所有草稿（按创建时间倒序）。
func (r *Repository) ListByAccount(accountID uint) ([]Draft, error) {
	var list []Draft
	err := r.db.Where("account_id = ?", accountID).
		Order("created_at DESC").Find(&list).Error
	return list, err
}

// Delete 删除指定草稿；不存在时返回 ErrDraftNotFound。
func (r *Repository) Delete(id uint) error {
	result := r.db.Delete(&Draft{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrDraftNotFound
	}
	return nil
}
