package account

import "time"

// Account 表示一个被管理的邮箱账户。凭证字段以 AES 加密后存储。
type Account struct {
	ID    uint   `gorm:"primaryKey"`
	Name  string `gorm:"not null"`
	Email string `gorm:"uniqueIndex;not null"`

	AuthType string `gorm:"not null;default:password"`

	Username    string
	PasswordEnc string `json:"-"`

	IMAPHost     string
	IMAPPort     int
	IMAPSecurity string

	SMTPHost     string
	SMTPPort     int
	SMTPSecurity string

	ProxyType        string
	ProxyHost        string
	ProxyPort        int
	ProxyUsername    string
	ProxyPasswordEnc string `json:"-"`

	Enabled    bool   `gorm:"not null;default:true" json:"-"`
	Status     string `gorm:"default:new"`
	LastSyncAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Account) TableName() string { return "accounts" }

// LoginName 返回 IMAP/SMTP 登录用户名（Username 为空则用 Email）。
func (a *Account) LoginName() string {
	if a.Username != "" {
		return a.Username
	}
	return a.Email
}
