package account

import (
	"errors"

	"gorm.io/gorm"
)

var ErrAccountNotFound = errors.New("account not found")

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(a *Account) error { return r.db.Create(a).Error }

func (r *Repository) GetByID(id uint) (*Account, error) {
	var a Account
	err := r.db.First(&a, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) List() ([]Account, error) {
	var list []Account
	err := r.db.Order("id asc").Find(&list).Error
	return list, err
}

func (r *Repository) Update(a *Account) error { return r.db.Save(a).Error }

func (r *Repository) SetEnabled(id uint, enabled bool) error {
	res := r.db.Model(&Account{}).Where("id = ?", id).Update("enabled", enabled)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAccountNotFound
	}
	return nil
}

func (r *Repository) IsEnabled(id uint) (bool, error) {
	var a Account
	err := r.db.Select("id", "enabled").First(&a, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, ErrAccountNotFound
	}
	if err != nil {
		return false, err
	}
	return a.Enabled, nil
}

// ListEnabledIDs 返回所有 enabled=true 账户的 ID。
func (r *Repository) ListEnabledIDs() ([]uint, error) {
	var ids []uint
	err := r.db.Model(&Account{}).Where("enabled = ?", true).Pluck("id", &ids).Error
	return ids, err
}

// Delete 删除账户，并在同一事务内清理其别名与签名（否则重建同 id 账户会捡到旧身份）。
func (r *Repository) Delete(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Delete(&Account{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrAccountNotFound
		}
		return r.deleteIdentityOf(tx, id)
	})
}
