package setting_test

import (
	"path/filepath"
	"testing"

	"flymail/internal/database"
	"flymail/modules/system/setting"
)

// newRepo 构建隔离临时 DB，返回 Repository。
func newRepo(t *testing.T) *setting.Repository {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, e := db.DB(); e == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return setting.NewRepository(db)
}

// newSvc 构建隔离 Service。
func newSvc(t *testing.T) *setting.Service {
	t.Helper()
	return setting.NewService(newRepo(t))
}

// ---------- Repository 测试 ----------

func TestRepo_SetAndGet(t *testing.T) {
	repo := newRepo(t)

	// 初始时未命中
	_, found, err := repo.Get("foo")
	if err != nil {
		t.Fatalf("Get unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected not found")
	}

	// Set 后命中
	if err := repo.Set("foo", "bar"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	val, found, err := repo.Get("foo")
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if !found {
		t.Fatal("expected found after Set")
	}
	if val != "bar" {
		t.Fatalf("expected bar, got %s", val)
	}
}

func TestRepo_SetTwice_Upsert(t *testing.T) {
	repo := newRepo(t)

	if err := repo.Set("k", "v1"); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	if err := repo.Set("k", "v2"); err != nil {
		t.Fatalf("second Set: %v", err)
	}

	// All 只返回一行
	all, err := repo.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 row, got %d", len(all))
	}
	if all["k"] != "v2" {
		t.Fatalf("expected v2, got %s", all["k"])
	}
}

func TestRepo_All(t *testing.T) {
	repo := newRepo(t)

	_ = repo.Set("a", "1")
	_ = repo.Set("b", "2")
	_ = repo.Set("c", "3")

	all, err := repo.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(all))
	}
	if all["a"] != "1" || all["b"] != "2" || all["c"] != "3" {
		t.Fatalf("unexpected map: %v", all)
	}
}

// ---------- Service 测试 ----------

func TestService_GetInt_NotExist(t *testing.T) {
	svc := newSvc(t)
	got := svc.GetInt("missing", 42)
	if got != 42 {
		t.Fatalf("expected default 42, got %d", got)
	}
}

func TestService_GetInt_Normal(t *testing.T) {
	repo := newRepo(t)
	_ = repo.Set(setting.KeySyncDepth, "1500")
	svc := setting.NewService(repo)

	got := svc.GetInt(setting.KeySyncDepth, 1000)
	if got != 1500 {
		t.Fatalf("expected 1500, got %d", got)
	}
}

func TestService_GetInt_NonNumeric(t *testing.T) {
	repo := newRepo(t)
	_ = repo.Set("bad", "abc")
	svc := setting.NewService(repo)

	got := svc.GetInt("bad", 99)
	if got != 99 {
		t.Fatalf("expected default 99 on parse failure, got %d", got)
	}
}

func TestService_SetMany(t *testing.T) {
	svc := newSvc(t)
	err := svc.SetMany(map[string]string{
		"x": "10",
		"y": "20",
	})
	if err != nil {
		t.Fatalf("SetMany: %v", err)
	}
	all := svc.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}
}
