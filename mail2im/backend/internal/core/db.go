package core

import (
	"log"
	"mail2im/internal/models"

	"flymail-core/database"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(dbPath string) {
	var err error
	DB, err = database.OpenSQLite(database.Options{Path: dbPath})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	err = models.AutoMigrate(DB)
	if err != nil {
		log.Fatal("Failed to migrate database:", err)
	}
}
