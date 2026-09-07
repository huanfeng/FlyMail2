package database

import (
	"flymail/modules/auth"
	"flymail/modules/email/account"
	"flymail/modules/email/draft"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	"flymail/modules/system/notify"
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
		&folder.Folder{},
		&message.Message{},
		&message.MessageBody{},
		&message.Attachment{},
		&setting.Setting{},
		&draft.Draft{},
		&notify.Notification{},
		&notify.Channel{},
		&notify.Log{},
	); err != nil {
		return err
	}
	// 触发器引用 messages / message_bodies，必须在 AutoMigrate 之后
	return message.EnsureFTS(db)
}
