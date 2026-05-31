package account_test

import (
	"errors"
	"path/filepath"
	"testing"

	"flymail/internal/database"
	"flymail/modules/email/account"
)

func newTestRepo(t *testing.T) *account.Repository {
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
	return account.NewRepository(db)
}

func TestRepositoryCRUD(t *testing.T) {
	r := newTestRepo(t)

	a := &account.Account{Name: "Work", Email: "u@example.com", AuthType: "password", PasswordEnc: "enc"}
	if err := r.Create(a); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == 0 {
		t.Fatal("Create 后 ID 应非零")
	}

	got, err := r.GetByID(a.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Email != "u@example.com" {
		t.Errorf("Email = %q", got.Email)
	}

	got.Name = "Work2"
	if err := r.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	reload, _ := r.GetByID(a.ID)
	if reload.Name != "Work2" {
		t.Errorf("Update 未生效: %q", reload.Name)
	}

	list, err := r.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %d, err=%v", len(list), err)
	}

	if err := r.Delete(a.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.GetByID(a.ID); !errors.Is(err, account.ErrAccountNotFound) {
		t.Errorf("删除后应返回 ErrAccountNotFound, 得到 %v", err)
	}
}
