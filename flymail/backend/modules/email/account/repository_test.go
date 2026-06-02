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

func TestListEnabledIDs(t *testing.T) {
	r := newTestRepo(t)

	// 建 2 个 enabled、1 个 disabled 账户
	a1 := &account.Account{Name: "A1", Email: "a1@example.com", AuthType: "password"}
	a2 := &account.Account{Name: "A2", Email: "a2@example.com", AuthType: "password"}
	a3 := &account.Account{Name: "A3", Email: "a3@example.com", AuthType: "password"}
	for _, a := range []*account.Account{a1, a2, a3} {
		if err := r.Create(a); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	// 将 a3 禁用（GORM 对 bool 零值不写入，须用 SetEnabled 显式更新）
	if err := r.SetEnabled(a3.ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	ids, err := r.ListEnabledIDs()
	if err != nil {
		t.Fatalf("ListEnabledIDs: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("期望 2 个启用账户 ID，实际得到 %d 个: %v", len(ids), ids)
	}
	// 确认返回的是 a1、a2 的 ID
	idSet := map[uint]bool{a1.ID: true, a2.ID: true}
	for _, id := range ids {
		if !idSet[id] {
			t.Errorf("意外的 ID %d", id)
		}
	}
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
