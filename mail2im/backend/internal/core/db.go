package core

import (
	"log"
	"mail2im/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(dbPath string) {
	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	err = models.AutoMigrate(DB)
	if err != nil {
		log.Fatal("Failed to migrate database:", err)
	}
}
