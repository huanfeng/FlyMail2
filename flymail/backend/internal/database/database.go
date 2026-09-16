package database

import (
	"flymail/modules/auth"
	"flymail/modules/email/account"
	"flymail/modules/email/draft"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	"flymail/modules/email/rule"
	"flymail/modules/system/notify"
	"flymail/modules/system/privacy"
	"flymail/modules/system/setting"

	coredb "flymail-core/database"
	"gorm.io/gorm"
)

// Open 打开 SQLite 数据库（经 core，glebarez 纯 Go 驱动）。
func Open(path string) (*gorm.DB, error) {
	return coredb.OpenSQLite(coredb.Options{Path: path})
}

// Migrate 迁移所有 FlyMail 模型，随后建全文索引（虚表 + 触发器，AutoMigrate 管不了）。
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&auth.AdminUser{},
		&account.Account{},
		&account.Alias{},
		&account.Signature{},
		&folder.Folder{},
		&message.Message{},
		&message.MessageBody{},
		&message.Attachment{},
		&rule.Rule{},
		&rule.BlockEntry{},
		&rule.RuleRun{},
		&setting.Setting{},
		&draft.Draft{},
		&notify.Notification{},
		&notify.Channel{},
		&notify.Log{},
		&privacy.TrustedSender{},
		&auth.LoginAttempt{},
	); err != nil {
		return err
	}
	// 触发器引用 messages / message_bodies，必须在 AutoMigrate 之后
	if err := message.EnsureFTS(db); err != nil {
		return err
	}
	// 老库升级：还没归属线程的邮件整库重建一次
	if err := message.EnsureThreads(db); err != nil {
		return err
	}
	// 老库升级：date 列统一成 UTC 文本，否则跨服务商排序按字节序会乱
	if err := message.EnsureUTCDates(db); err != nil {
		return err
	}
	// 刷新规划器统计：没有 sqlite_stat1 时，会话列表的 JOIN folders 会选成先扫 messages
	// 再探 folders（12.8k 封上 40ms，有统计后 5ms）。analysis_limit 让每个索引最多采样 1000 行，
	// 开销不随库线性增长（SQLite 推荐的有界 ANALYZE 用法）；只在启动时跑一次。
	if err := db.Exec("PRAGMA analysis_limit = 1000").Error; err != nil {
		return err
	}
	return db.Exec("ANALYZE").Error
}
