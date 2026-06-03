package auth

import "time"

// AdminUser 单管理员账户。
type AdminUser struct {
	ID           uint   `gorm:"primaryKey"`
	Username     string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null"`
	DisplayName  string // 展示名（资料页可改，留空时 UI 回落到用户名）
	Email        string // 联系邮箱（资料页可改，信息性字段）
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (AdminUser) TableName() string { return "admin_users" }
