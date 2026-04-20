package testutil

import (
	"mail2im/internal/core"
	"mail2im/internal/models"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SetupTestDB creates an in-memory SQLite database for testing.
// Uses shared cache mode and single connection to ensure all queries
// hit the same in-memory database instance.
func SetupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Limit to 1 connection to ensure consistency with :memory: SQLite
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	// Set global DB for core package
	core.DB = db

	t.Cleanup(func() {
		sqlDB.Close()
	})

	return db
}
