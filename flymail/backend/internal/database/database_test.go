package database

import (
	"path/filepath"
	"testing"

	"flymail/modules/auth"
)

func TestOpenAndMigrate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// 测试结束前关闭连接，避免 Windows 文件锁导致 TempDir 清理失败。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !db.Migrator().HasTable(&auth.AdminUser{}) {
		t.Error("admin_users 表未创建")
	}
}
