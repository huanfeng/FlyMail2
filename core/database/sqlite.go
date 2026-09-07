package database

import (
	"fmt"
	"strings"

	"github.com/glebarez/sqlite"
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

	db, err := gorm.Open(sqlite.Open(dsn(opts.Path)), &gorm.Config{
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

// busyTimeoutMS 是等待写锁的上限。SQLite 同一时刻只允许一个写者，默认 busy_timeout=0
// 意味着第二个写者会立刻收到 SQLITE_BUSY——多账户同步 worker、回写队列、服务端搜索补抓
// 都是并发写库，实测两个账户同时落库就会有一个失败。5 秒足以覆盖一次正常的批量写入。
const busyTimeoutMS = 5000

// dsn 给文件路径附上连接级 PRAGMA。已带查询串或内存库（:memory:）的路径原样返回，
// 由调用方自己决定参数。
func dsn(path string) string {
	if strings.Contains(path, "?") || strings.HasPrefix(path, ":memory:") || strings.HasPrefix(path, "file:") {
		return path
	}
	return fmt.Sprintf("%s?_pragma=busy_timeout(%d)", path, busyTimeoutMS)
}
