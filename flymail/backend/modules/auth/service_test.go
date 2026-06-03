package auth_test

import (
	"errors"
	"path/filepath"
	"testing"

	"flymail/internal/database"
	"flymail/modules/auth"
)

func newTestService(t *testing.T) *auth.Service {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	})
	return auth.NewService(auth.NewRepository(db), auth.Options{JWTSecret: "test-secret", AccessTTLMin: 15, RefreshTTLHour: 168})
}

func TestChangePassword(t *testing.T) {
	s := newTestService(t)

	// 准备初始密码
	if err := s.SetAdminPassword("admin", "old-pass"); err != nil {
		t.Fatalf("SetAdminPassword: %v", err)
	}

	// 正确旧密码 → 修改成功
	if err := s.ChangePassword("admin", "old-pass", "new-pass"); err != nil {
		t.Fatalf("ChangePassword 应成功: %v", err)
	}

	// 新密码可以登录
	if _, err := s.Authenticate("admin", "new-pass"); err != nil {
		t.Fatalf("新密码应可登录: %v", err)
	}

	// 旧密码不可登录
	if _, err := s.Authenticate("admin", "old-pass"); err == nil {
		t.Error("旧密码改后应无法登录")
	}

	// 错误旧密码 → ErrInvalidCredentials
	if err := s.ChangePassword("admin", "wrong-pass", "x"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("错误旧密码应返回 ErrInvalidCredentials, 实际: %v", err)
	}
}

func TestProfileAndUpdate(t *testing.T) {
	s := newTestService(t)
	if err := s.SetAdminPassword("admin", "p@ss"); err != nil {
		t.Fatalf("SetAdminPassword: %v", err)
	}

	// 初始资料：展示名/邮箱为空
	p, err := s.Profile("admin")
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}
	if p.DisplayName != "" || p.Email != "" {
		t.Errorf("初始资料应为空, got name=%q email=%q", p.DisplayName, p.Email)
	}

	// 更新资料（含两端空白，应被去除）
	if _, err := s.UpdateProfile("admin", "  管理员  ", " me@example.com "); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	p2, err := s.Profile("admin")
	if err != nil {
		t.Fatalf("Profile after update: %v", err)
	}
	if p2.DisplayName != "管理员" {
		t.Errorf("display name = %q, want 管理员", p2.DisplayName)
	}
	if p2.Email != "me@example.com" {
		t.Errorf("email = %q, want me@example.com", p2.Email)
	}
}

func TestLoginRecordsLastLogin(t *testing.T) {
	s := newTestService(t)
	if err := s.SetAdminPassword("admin", "p@ss"); err != nil {
		t.Fatalf("SetAdminPassword: %v", err)
	}
	// 登录前 LastLoginAt 为空
	before, _ := s.Profile("admin")
	if before.LastLoginAt != nil {
		t.Error("登录前 LastLoginAt 应为 nil")
	}
	if _, err := s.Login("admin", "p@ss"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	after, _ := s.Profile("admin")
	if after.LastLoginAt == nil {
		t.Error("登录后 LastLoginAt 应被记录")
	}
}

func TestSetAdminPasswordAndAuthenticate(t *testing.T) {
	s := newTestService(t)
	if err := s.SetAdminPassword("admin", "p@ssw0rd"); err != nil {
		t.Fatalf("SetAdminPassword: %v", err)
	}
	u, err := s.Authenticate("admin", "p@ssw0rd")
	if err != nil {
		t.Fatalf("Authenticate ok case: %v", err)
	}
	if u.Username != "admin" {
		t.Errorf("username = %q", u.Username)
	}
	if _, err := s.Authenticate("admin", "wrong"); err == nil {
		t.Error("Authenticate 应对错误密码报错")
	}
}
