package draft_test

import (
	"testing"

	"flymail/modules/email/draft"

	coredb "flymail-core/database"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := coredb.OpenSQLite(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&draft.Draft{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestRepository_CreateAndGetByID(t *testing.T) {
	db := setupTestDB(t)
	repo := draft.NewRepository(db)

	d := &draft.Draft{
		AccountID: 1,
		ToStr:     "a@example.com,b@example.com",
		Subject:   "Hello",
		BodyHTML:  "<p>hi</p>",
	}
	if err := repo.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.ID == 0 {
		t.Fatal("ID should be set after Create")
	}

	got, err := repo.GetByID(d.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Subject != d.Subject {
		t.Errorf("Subject mismatch: got %q want %q", got.Subject, d.Subject)
	}
	if got.ToStr != d.ToStr {
		t.Errorf("ToStr mismatch: got %q want %q", got.ToStr, d.ToStr)
	}
}

func TestRepository_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := draft.NewRepository(db)

	_, err := repo.GetByID(999)
	if err != draft.ErrDraftNotFound {
		t.Errorf("expected ErrDraftNotFound, got %v", err)
	}
}

func TestRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	repo := draft.NewRepository(db)

	d := &draft.Draft{AccountID: 1, Subject: "Original"}
	if err := repo.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}

	d.Subject = "Updated"
	if err := repo.Update(d); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := repo.GetByID(d.ID)
	if got.Subject != "Updated" {
		t.Errorf("expected Updated, got %q", got.Subject)
	}
}

func TestRepository_ListByAccount(t *testing.T) {
	db := setupTestDB(t)
	repo := draft.NewRepository(db)

	// account 1: 2 drafts
	_ = repo.Create(&draft.Draft{AccountID: 1, Subject: "A"})
	_ = repo.Create(&draft.Draft{AccountID: 1, Subject: "B"})
	// account 2: 1 draft (should not appear)
	_ = repo.Create(&draft.Draft{AccountID: 2, Subject: "C"})

	list, err := repo.ListByAccount(1)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 drafts for account 1, got %d", len(list))
	}
	for _, item := range list {
		if item.AccountID != 1 {
			t.Errorf("unexpected account_id %d", item.AccountID)
		}
	}
}

func TestRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := draft.NewRepository(db)

	d := &draft.Draft{AccountID: 1, Subject: "ToDelete"}
	_ = repo.Create(d)

	if err := repo.Delete(d.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.GetByID(d.ID)
	if err != draft.ErrDraftNotFound {
		t.Errorf("expected ErrDraftNotFound after Delete, got %v", err)
	}
}

func TestRepository_Delete_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := draft.NewRepository(db)

	err := repo.Delete(999)
	if err != draft.ErrDraftNotFound {
		t.Errorf("expected ErrDraftNotFound, got %v", err)
	}
}
