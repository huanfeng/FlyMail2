package testutil

import (
	"mail2im/internal/core"
	"mail2im/internal/models"
	"os"
	"strconv"
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

// TestAccountConfig 测试邮箱账号配置
type TestAccountConfig struct {
	Email      string
	Password   string
	IMAPServer string
	IMAPPort   int
	SSLMode    string
	Provider   string
}

// LoadTestAccountConfig 从环境变量加载测试邮箱配置
func LoadTestAccountConfig(index int) *TestAccountConfig {
	email := os.Getenv("TEST_EMAIL_" + strconv.Itoa(index))
	password := os.Getenv("TEST_PASSWORD_" + strconv.Itoa(index))
	imapServer := os.Getenv("TEST_IMAP_SERVER_" + strconv.Itoa(index))
	imapPortStr := os.Getenv("TEST_IMAP_PORT_" + strconv.Itoa(index))
	sslMode := os.Getenv("TEST_SSL_MODE_" + strconv.Itoa(index))

	if email == "" || password == "" || imapServer == "" {
		return nil
	}

	imapPort := 993
	if imapPortStr != "" {
		if port, err := strconv.Atoi(imapPortStr); err == nil {
			imapPort = port
		}
	}

	if sslMode == "" {
		sslMode = "ssl"
	}

	return &TestAccountConfig{
		Email:      email,
		Password:   password,
		IMAPServer: imapServer,
		IMAPPort:   imapPort,
		SSLMode:    sslMode,
	}
}

// LoadAllTestAccounts 加载所有配置的测试邮箱账号
func LoadAllTestAccounts() []TestAccountConfig {
	var accounts []TestAccountConfig

	for i := 1; i <= 10; i++ {
		if config := LoadTestAccountConfig(i); config != nil {
			accounts = append(accounts, *config)
		}
	}

	return accounts
}

// HasTestAccounts 检查是否配置了测试邮箱账号
func HasTestAccounts() bool {
	return len(LoadAllTestAccounts()) > 0
}
