package privacy

import (
	"errors"
	"path/filepath"
	"testing"

	coredb "flymail-core/database"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"  Alice@Example.COM ": "alice@example.com",
		"no-at":                "",
		"@x.com":               "",
		"a@":                   "",
		"a@b@c":                "",
		"Alice <a@b.c>":        "",
		"":                     "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTrustedSenders(t *testing.T) {
	db, err := coredb.OpenSQLite(coredb.Options{Path: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&TrustedSender{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	s := NewService(db)
	if s.IsTrusted("a@b.c") {
		t.Fatal("empty list must trust nobody")
	}
	e, err := s.Add(" A@B.C ")
	if err != nil || e.Address != "a@b.c" {
		t.Fatalf("add: %v %+v", err, e)
	}
	if _, err := s.Add("a@b.c"); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate: %v", err)
	}
	if _, err := s.Add("bad"); !errors.Is(err, ErrInvalid) {
		t.Errorf("invalid: %v", err)
	}
	if !s.IsTrusted("A@b.c") || s.IsTrusted("x@b.c") {
		t.Errorf("IsTrusted mismatch")
	}
	if err := s.Delete(e.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(e.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete twice: %v", err)
	}
	if s.IsTrusted("a@b.c") {
		t.Errorf("deleted sender must not be trusted")
	}
}
