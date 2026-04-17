package database

import (
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Options 配置 SQLite 连接选项
type Options struct {
	Path    string          // 数据库文件路径
	LogMode logger.LogLevel // 日志级别，默认 logger.Error
}

// OpenSQLite 打开一个 SQLite 数据库连接
func OpenSQLite(opts Options) (*gorm.DB, error) {
	if opts.Path == "" {
		return nil, fmt.Errorf("database path is required")
	}

	logMode := opts.LogMode
	if logMode == 0 {
		logMode = logger.Error
	}

	db, err := gorm.Open(sqlite.Open(opts.Path), &gorm.Config{
		Logger: logger.Default.LogMode(logMode),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	return db, nil
}

// Close 关闭数据库连接
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}
	return sqlDB.Close()
}
