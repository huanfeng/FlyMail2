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

	// OAuth 凭据（AuthType 为 oauth 时有效）。
	// OAuthProvider 取 google / microsoft；OAuthTokenEnc 是整包令牌 JSON 的密文
	// （access + refresh + scope 一起加密，字段增减不必改表）；OAuthExpiresAt 以明文
	// 冗余存一份过期时间，让「哪些账户快过期了」这类诊断查询不必解密全表。
	//
	// 显式指定列名：GORM 的默认命名策略会把 OAuthProvider 拆成 o_auth_provider，
	// 与按列名局部更新时手写的键对不上。
	OAuthProvider  string     `gorm:"column:oauth_provider"`
	OAuthTokenEnc  string     `gorm:"column:oauth_token_enc" json:"-"`
	OAuthExpiresAt *time.Time `gorm:"column:oauth_expires_at"`

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

	// SortOrder 是账户在侧栏与设置页里的显示位次，由用户手动调整。
	//
	// 排序是**服务端的单一事实**：List 按它排好再返回，前端照数组顺序渲染，
	// 不再自己排一遍。否则侧栏、设置页、写信的发件人选择器就会各有一份排序逻辑，
	// 迟早漂移成三种顺序。
	//
	// 老库经 AutoMigrate 加列后全为 0，靠 List 里的 id 兜底排序保持原样，
	// 直到用户第一次调整顺序（Reorder 会把全表重写成 0..n-1）。
	SortOrder int `gorm:"not null;default:0" json:"-"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Account) TableName() string { return "accounts" }

// 账户状态取值。needs_reauth 专指 OAuth 刷新令牌失效（用户撤销授权、改密码、
// 或长期未使用被服务商回收），此时任何重试都不会成功，只能引导用户重新授权。
const (
	StatusNew         = "new"
	StatusOK          = "ok"
	StatusNeedsReauth = "needs_reauth"
)

// IsOAuth 报告该账户是否使用 OAuth 认证。
func (a *Account) IsOAuth() bool { return a.AuthType == AuthTypeOAuth }

// 认证方式取值。
const (
	AuthTypePassword = "password"
	AuthTypeOAuth    = "oauth"
)

// LoginName 返回 IMAP/SMTP 登录用户名（Username 为空则用 Email）。
func (a *Account) LoginName() string {
	if a.Username != "" {
		return a.Username
	}
	return a.Email
}
