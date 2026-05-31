package auth

import "time"

// AdminUser 单管理员账户。
type AdminUser struct {
	ID           uint   `gorm:"primaryKey"`
	Username     string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (AdminUser) TableName() string { return "admin_users" }
