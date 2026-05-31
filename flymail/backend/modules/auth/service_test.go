package auth_test

import (
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
