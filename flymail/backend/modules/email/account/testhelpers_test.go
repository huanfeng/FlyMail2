package account_test

import (
	"path/filepath"
	"testing"

	"flymail/internal/crypto"
	"flymail/internal/database"
	"flymail/modules/email/account"
)

// newSvc 构建一个隔离的账户 Service（独立临时 DB），并返回其 repo 与 encryptor 以便断言落库内容。
func newSvc(t *testing.T) (*account.Service, *account.Repository, *crypto.Encryptor) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if sqlDB, e := db.DB(); e == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := account.NewRepository(db)
	enc, err := crypto.New("a-test-encryption-key-32bytes!!")
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	return account.NewService(repo, enc), repo, enc
}
