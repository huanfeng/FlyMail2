package database

import (
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"flymail/modules/system/task"
	"flymail/shared/config"

	coreauth "flymail-core/auth"
	"flymail/shared/store/model"
)

type ServerDB struct {
	MainDB *gorm.DB
	LogDB  *gorm.DB
}

var serverDB *ServerDB

func Init(cfg *config.Config) error {
	var err error

	mainDB, err := gorm.Open(sqlite.Open(cfg.Database.Path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to main database: %w", err)
	}

	// Auto migrate the schema
	if err := mainDB.AutoMigrate(
		&model.User{},
		&model.EmailAccount{},
		&model.Email{},
		&model.Attachment{},
		&model.Folder{},
		&model.Setting{},
		&model.NotifyChannel{},
		&task.Config{},
	); err != nil {
		return fmt.Errorf("failed to migrate main database: %w", err)
	}

	// Create default admin user if not exists
	if err := createDefaultAdmin(mainDB, cfg); err != nil {
		return fmt.Errorf("failed to create default admin: %w", err)
	}

	// Create default settings
	if err := createDefaultSettings(mainDB); err != nil {
		return fmt.Errorf("failed to create default settings: %w", err)
	}

	logDB, err := gorm.Open(sqlite.Open(cfg.Database.LogPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to log database: %w", err)
	}

	if err := logDB.AutoMigrate(
		&model.NotifyLog{},
		&task.Log{},
	); err != nil {
		return fmt.Errorf("failed to migrate log database: %w", err)
	}

	serverDB = &ServerDB{
		MainDB: mainDB,
		LogDB:  logDB,
	}

	return nil
}

func createDefaultAdmin(mainDB *gorm.DB, cfg *config.Config) error {
	var count int64
	if err := mainDB.Model(&model.User{}).Count(&count).Error; err != nil {
		return err
	}

	if count == 0 {
		hashedPassword, err := coreauth.HashPassword(cfg.Auth.AdminDefaultPassword)
		if err != nil {
			return err
		}

		admin := model.User{
			Username: "admin",
			Email:    "admin@flaymail.local",
			Password: hashedPassword,
			IsAdmin:  true,
		}

		if err := mainDB.Create(&admin).Error; err != nil {
			return err
		}
	}

	return nil
}

func createDefaultSettings(mainDB *gorm.DB) error {
	settings := []model.Setting{
		{Key: "email_sync_interval", Value: "300"}, // Default 5 minutes
		{Key: "email_monitor_enabled", Value: "true"},
		{Key: "email_monitor_enable_idle", Value: "true"},
		{Key: "email_monitor_day_start", Value: "8"},
		{Key: "email_monitor_day_end", Value: "22"},
		{Key: "email_monitor_day_interval", Value: "1m"},
		{Key: "email_monitor_night_interval", Value: "10m"},
		{Key: "email_monitor_retry_interval", Value: "30s"},
		{Key: "email_monitor_max_retries", Value: "3"},
	}

	for _, setting := range settings {
		var existing model.Setting
		if err := mainDB.Where("key = ?", setting.Key).First(&existing).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				if err := mainDB.Create(&setting).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}

	return nil
}

func ResetAdminPassword(cfg *config.Config) error {
	if serverDB == nil {
		if err := Init(cfg); err != nil {
			return err
		}
	}

	hashedPassword, err := coreauth.HashPassword(cfg.Auth.AdminDefaultPassword)
	if err != nil {
		return err
	}

	if err := serverDB.MainDB.Model(&model.User{}).Where("username = ?", "admin").Update("password", hashedPassword).Error; err != nil {
		return err
	}

	return nil
}

func GetDB() *ServerDB {
	return serverDB
}

// Close closes both database connections
func (db *ServerDB) Close() error {
	mainDB, _ := db.MainDB.DB()
	if err := mainDB.Close(); err != nil {
		return err
	}

	logDB, _ := db.LogDB.DB()
	if err := logDB.Close(); err != nil {
		return err
	}

	return nil
}
