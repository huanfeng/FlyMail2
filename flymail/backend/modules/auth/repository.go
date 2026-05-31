package auth

import (
	"errors"

	"gorm.io/gorm"
)

var ErrAdminNotFound = errors.New("admin user not found")

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) GetByUsername(username string) (*AdminUser, error) {
	var u AdminUser
	err := r.db.Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAdminNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) Count() (int64, error) {
	var n int64
	err := r.db.Model(&AdminUser{}).Count(&n).Error
	return n, err
}

func (r *Repository) Upsert(u *AdminUser) error {
	return r.db.Save(u).Error
}
