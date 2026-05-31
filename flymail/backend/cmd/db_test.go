package cmd

import (
	"path/filepath"
	"testing"

	"flymail/internal/database"
	"flymail/modules/auth"
)

func TestRunDBInitCreatesAdmin(t *testing.T) {
	dir := t.TempDir()
	if err := runDBInit(dir, "", "admin", "secret123"); err != nil {
		t.Fatalf("runDBInit: %v", err)
	}
	db, err := database.Open(filepath.Join(dir, "flymail.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Windows 下必须显式关闭，否则 TempDir 清理时文件被占用
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	repo := auth.NewRepository(db)
	u, err := repo.GetByUsername("admin")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if u.PasswordHash == "" {
		t.Error("管理员密码未设置")
	}
}
