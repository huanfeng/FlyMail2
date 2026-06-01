package database

import (
	"flymail/modules/auth"
	"flymail/modules/email/account"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"

	coredb "flymail-core/database"
	"gorm.io/gorm"
)

// Open 打开 SQLite 数据库（经 core，glebarez 纯 Go 驱动）。
func Open(path string) (*gorm.DB, error) {
	return coredb.OpenSQLite(coredb.Options{Path: path})
}

// Migrate 迁移所有 FlyMail 模型。后续里程碑在此追加模型。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&auth.AdminUser{},
		&account.Account{},
		&folder.Folder{},
		&message.Message{},
		&message.MessageBody{},
		&message.Attachment{},
	)
}
