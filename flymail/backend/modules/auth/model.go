package auth

import (
	"time"

	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	UserID       uint           `gorm:"primaryKey;column:id" json:"user_id"`
	Username     string         `gorm:"unique;not null" json:"username"`
	Email        string         `json:"email"`
	Password     string         `gorm:"not null" json:"-"`
	PasswordHash string         `gorm:"column:password" json:"-"`
	IsAdmin      bool           `gorm:"default:false" json:"is_admin"`
	LastLogin    *time.Time     `json:"last_login"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// AuthResponse represents the authentication response
type AuthResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}
