package auth

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// LoginAttempt 按来源 IP 记录登录失败：15 分钟窗口内累计 10 次后封禁到窗口结束。
// 落库而不是内存：重启不清零，否则攻击者只要等一次重启。
// 封禁态不单独存列，由 failures 与 window_start 推导——少一个要与计数一起原子更新的字段。
type LoginAttempt struct {
	IP          string    `gorm:"primaryKey;size:64"`
	Failures    int       `gorm:"not null;default:0"`
	WindowStart time.Time `gorm:"not null"`
	UpdatedAt   time.Time
}

func (LoginAttempt) TableName() string { return "login_attempts" }

const (
	// loginWindow 是失败计数的窗口；loginMaxFailures 是窗口内允许的失败次数（第 11 次起 429）。
	loginWindow      = 15 * time.Minute
	loginMaxFailures = 10
)

// ErrTooManyAttempts 表示该 IP 已被限流；RetryAfter 是建议等待时间。
type ErrTooManyAttempts struct{ RetryAfter time.Duration }

func (e *ErrTooManyAttempts) Error() string { return "too many login attempts" }

// Limiter 是登录限流器，与 Service 共用数据库。
type Limiter struct {
	db  *gorm.DB
	now func() time.Time
}

func NewLimiter(db *gorm.DB) *Limiter { return &Limiter{db: db, now: time.Now} }

// MigrateLimiter 建表（auth 包自己的表，随 database.Migrate 一起调用）。
func MigrateLimiter(db *gorm.DB) error { return db.AutoMigrate(&LoginAttempt{}) }

// Check 在校验密码之前调用：窗口内失败已达阈值返回 *ErrTooManyAttempts。
func (l *Limiter) Check(ip string) error {
	var a LoginAttempt
	err := l.db.Where("ip = ?", ip).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	now := l.now()
	if until := a.WindowStart.Add(loginWindow); a.Failures >= loginMaxFailures && until.After(now) {
		return &ErrTooManyAttempts{RetryAfter: until.Sub(now)}
	}
	return nil
}

// Fail 记一次失败。单条原子 upsert：窗口已过期就从 1 重新计，否则 +1。
// 不能用「读 → 改 → 写」：并发的登录请求会各自读到旧值再互相覆盖，实测 40 个并发只记到 1 次，
// 攻击者用十来个连接就能让限流永远不触发。
func (l *Limiter) Fail(ip string) error {
	now := l.now()
	cutoff := now.Add(-loginWindow)
	return l.db.Exec(`INSERT INTO login_attempts (ip, failures, window_start, updated_at) VALUES (?, 1, ?, ?)
ON CONFLICT(ip) DO UPDATE SET
  failures = CASE WHEN login_attempts.window_start <= ? THEN 1 ELSE login_attempts.failures + 1 END,
  window_start = CASE WHEN login_attempts.window_start <= ? THEN excluded.window_start ELSE login_attempts.window_start END,
  updated_at = excluded.updated_at`, ip, now, now, cutoff, cutoff).Error
}

// Reset 登录成功后清掉该 IP 的记录。
func (l *Limiter) Reset(ip string) error {
	return l.db.Where("ip = ?", ip).Delete(&LoginAttempt{}).Error
}
