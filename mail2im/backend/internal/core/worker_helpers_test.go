package core

import (
	"flymail-core/types"
	"mail2im/internal/models"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB 在 core 包内部创建测试数据库，避免循环依赖
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	// 设置全局 DB
	DB = db

	t.Cleanup(func() {
		sqlDB.Close()
	})

	return db
}

// TestIsNoSelect 验证 \Noselect 属性检测
func TestIsNoSelect(t *testing.T) {
	tests := []struct {
		name     string
		mailbox  models.Mailbox
		expected bool
	}{
		{
			name:     "空属性返回 false",
			mailbox:  models.Mailbox{Attributes: ""},
			expected: false,
		},
		{
			name:     "包含 \\Noselect 返回 true",
			mailbox:  models.Mailbox{Attributes: "\\Noselect"},
			expected: true,
		},
		{
			name:     "小写 \\noselect 返回 true",
			mailbox:  models.Mailbox{Attributes: "\\noselect"},
			expected: true,
		},
		{
			name:     "混合大小写返回 true",
			mailbox:  models.Mailbox{Attributes: "\\NoSelect"},
			expected: true,
		},
		{
			name:     "多个属性中包含返回 true",
			mailbox:  models.Mailbox{Attributes: "\\HasChildren, \\Noselect"},
			expected: true,
		},
		{
			name:     "不包含返回 false",
			mailbox:  models.Mailbox{Attributes: "\\HasChildren, \\Subscribed"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNoSelect(tt.mailbox)
			if result != tt.expected {
				t.Errorf("isNoSelect(%+v) = %v, 期望 %v", tt.mailbox, result, tt.expected)
			}
		})
	}
}

// TestGetMapKeys 验证 map 键提取
func TestGetMapKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]bool
		expected []string
	}{
		{
			name:     "空 map 返回空字符串切片",
			input:    map[string]bool{},
			expected: []string{""},
		},
		{
			name:     "单个键",
			input:    map[string]bool{"INBOX": true},
			expected: []string{"INBOX"},
		},
		{
			name:     "多个键",
			input:    map[string]bool{"INBOX": true, "Sent": true, "Drafts": true},
			expected: []string{"INBOX", "Sent", "Drafts"}, // 顺序可能不同
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMapKeys(tt.input)

			// 对于空 map，检查返回的是包含空字符串的切片
			if len(tt.input) == 0 {
				if len(result) != 1 || result[0] != "" {
					t.Errorf("getMapKeys(空map) = %v, 期望 ['']", result)
				}
				return
			}

			// 对于非空 map，检查长度和包含关系
			if len(result) != len(tt.expected) {
				t.Errorf("getMapKeys 长度 = %d, 期望 %d", len(result), len(tt.expected))
				return
			}

			// 检查所有期望的键都存在
			resultSet := make(map[string]bool)
			for _, k := range result {
				resultSet[k] = true
			}
			for _, expectedKey := range tt.expected {
				if !resultSet[expectedKey] {
					t.Errorf("getMapKeys 结果缺少键 %q, 结果: %v", expectedKey, result)
				}
			}
		})
	}
}

// TestClassifyMailboxType 验证邮箱分类逻辑
func TestClassifyMailboxType(t *testing.T) {
	// 初始化测试数据库
	setupTestDB(t)

	tests := []struct {
		name       string
		providerID string
		mailbox    string
		path       string
		expected   string
	}{
		{
			name:       "Gmail INBOX 映射到 primary",
			providerID: "gmail",
			mailbox:    "INBOX",
			path:       "INBOX",
			expected:   "primary",
		},
		{
			name:       "Gmail Sent 映射到 sent",
			providerID: "gmail",
			mailbox:    "[Gmail]/Sent Mail",
			path:       "[Gmail]/Sent Mail",
			expected:   "sent",
		},
		{
			name:       "Gmail Drafts 映射到 draft",
			providerID: "gmail",
			mailbox:    "[Gmail]/Drafts",
			path:       "[Gmail]/Drafts",
			expected:   "draft",
		},
		{
			name:       "Gmail Trash 映射到 trash",
			providerID: "gmail",
			mailbox:    "[Gmail]/Trash",
			path:       "[Gmail]/Trash",
			expected:   "trash",
		},
		{
			name:       "Gmail Spam 映射到 spam",
			providerID: "gmail",
			mailbox:    "[Gmail]/Spam",
			path:       "[Gmail]/Spam",
			expected:   "spam",
		},
		{
			name:       "Outlook INBOX 映射到 primary",
			providerID: "outlook",
			mailbox:    "INBOX",
			path:       "INBOX",
			expected:   "primary",
		},
		{
			name:       "Outlook Sent Items 映射到 sent",
			providerID: "outlook",
			mailbox:    "Sent Items",
			path:       "Sent Items",
			expected:   "sent",
		},
		{
			name:       "未知 Provider 返回 unknown",
			providerID: "unknown-provider",
			mailbox:    "CustomFolder",
			path:       "CustomFolder",
			expected:   "unknown",
		},
		{
			name:       "空 Provider 返回 unknown",
			providerID: "",
			mailbox:    "CustomFolder",
			path:       "CustomFolder",
			expected:   "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyMailboxType(tt.providerID, tt.mailbox, tt.path)
			if result != tt.expected {
				t.Errorf("classifyMailboxType(%q, %q, %q) = %q, 期望 %q",
					tt.providerID, tt.mailbox, tt.path, result, tt.expected)
			}
		})
	}
}

// TestNewWorker 验证 Worker 创建
func TestNewWorker(t *testing.T) {
	tests := []struct {
		name    string
		account models.Account
	}{
		{
			name: "基本账户",
			account: models.Account{
				Email:      "test@example.com",
				IMAPServer: "imap.example.com",
				IMAPPort:   993,
			},
		},
		{
			name: "带代理的账户",
			account: models.Account{
				Email:      "test@example.com",
				IMAPServer: "imap.example.com",
				IMAPPort:   993,
				Proxy: &models.Proxy{
					Type: "socks5",
					Host: "proxy.example.com",
					Port: 1080,
				},
			},
		},
		{
			name: "空账户",
			account: models.Account{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := NewWorker(tt.account)

			// 验证 Worker 不为 nil
			if worker == nil {
				t.Fatal("NewWorker() 返回 nil")
			}

			// 验证账户信息正确
			if worker.Account.Email != tt.account.Email {
				t.Errorf("Worker.Account.Email = %q, 期望 %q", worker.Account.Email, tt.account.Email)
			}

			// 验证 StopChan 不为 nil
			if worker.StopChan == nil {
				t.Error("Worker.StopChan 不应该为 nil")
			}

			// 验证初始状态
			if worker.IsRunning {
				t.Error("新创建的 Worker 不应该在运行")
			}

			if worker.Client != nil {
				t.Error("新创建的 Worker.Client 应该为 nil")
			}

			if worker.session != nil {
				t.Error("新创建的 Worker.session 应该为 nil")
			}
		})
	}
}

// TestBuildIMAPConfig 验证 IMAP 配置构建
func TestBuildIMAPConfig(t *testing.T) {
	// 初始化加密模块
	InitCrypto("test-secret-key-32bytes-long!!")

	tests := []struct {
		name     string
		account  models.Account
		check    func(t *testing.T, cfg types.IMAPConfig)
	}{
		{
			name: "基本配置",
			account: models.Account{
				Email:      "test@example.com",
				Login:      "test@example.com",
				Password:   "password123",
				IMAPServer: "imap.example.com",
				IMAPPort:   993,
				SSLMode:    "ssl",
			},
			check: func(t *testing.T, cfg types.IMAPConfig) {
				if cfg.Host != "imap.example.com" {
					t.Errorf("Host = %q, 期望 %q", cfg.Host, "imap.example.com")
				}
				if cfg.Port != 993 {
					t.Errorf("Port = %d, 期望 %d", cfg.Port, 993)
				}
				if cfg.Username != "test@example.com" {
					t.Errorf("Username = %q, 期望 %q", cfg.Username, "test@example.com")
				}
				if cfg.Password != "password123" {
					t.Errorf("Password = %q, 期望 %q", cfg.Password, "password123")
				}
				if cfg.Security != types.SecuritySSL {
					t.Errorf("Security = %q, 期望 %q", cfg.Security, types.SecuritySSL)
				}
				if cfg.ClientName != "Mail2IM" {
					t.Errorf("ClientName = %q, 期望 %q", cfg.ClientName, "Mail2IM")
				}
				if cfg.ClientVendor != "Mail2IM" {
					t.Errorf("ClientVendor = %q, 期望 %q", cfg.ClientVendor, "Mail2IM")
				}
				if cfg.Proxy != nil {
					t.Error("Proxy 应该为 nil")
				}
			},
		},
		{
			name: "Login 为空时使用 Email",
			account: models.Account{
				Email:      "user@example.com",
				Login:      "",
				Password:   "pass",
				IMAPServer: "imap.example.com",
				IMAPPort:   993,
				SSLMode:    "ssl",
			},
			check: func(t *testing.T, cfg types.IMAPConfig) {
				if cfg.Username != "user@example.com" {
					t.Errorf("Username = %q, 期望 %q", cfg.Username, "user@example.com")
				}
			},
		},
		{
			name: "SSLMode 为空且 UseSSL 为 true",
			account: models.Account{
				Email:      "test@example.com",
				Password:   "pass",
				IMAPServer: "imap.example.com",
				IMAPPort:   993,
				SSLMode:    "",
				UseSSL:     true,
			},
			check: func(t *testing.T, cfg types.IMAPConfig) {
				if cfg.Security != types.SecuritySSL {
					t.Errorf("Security = %q, 期望 %q", cfg.Security, types.SecuritySSL)
				}
			},
		},
		{
			name: "SSLMode 为空且 UseSSL 为 false",
			account: models.Account{
				Email:      "test@example.com",
				Password:   "pass",
				IMAPServer: "imap.example.com",
				IMAPPort:   993,
				SSLMode:    "",
				UseSSL:     false,
			},
			check: func(t *testing.T, cfg types.IMAPConfig) {
				if cfg.Security != types.SecurityNone {
					t.Errorf("Security = %q, 期望 %q", cfg.Security, types.SecurityNone)
				}
			},
		},
		{
			name: "带代理配置",
			account: models.Account{
				Email:      "test@example.com",
				Password:   "pass",
				IMAPServer: "imap.example.com",
				IMAPPort:   993,
				SSLMode:    "ssl",
				Proxy: &models.Proxy{
					Type:     "socks5",
					Host:     "proxy.example.com",
					Port:     1080,
					Username: "proxyuser",
					Password: "proxypass",
				},
			},
			check: func(t *testing.T, cfg types.IMAPConfig) {
				if cfg.Proxy == nil {
					t.Fatal("Proxy 不应该为 nil")
				}
				if cfg.Proxy.Type != "socks5" {
					t.Errorf("Proxy.Type = %q, 期望 %q", cfg.Proxy.Type, "socks5")
				}
				if cfg.Proxy.Host != "proxy.example.com" {
					t.Errorf("Proxy.Host = %q, 期望 %q", cfg.Proxy.Host, "proxy.example.com")
				}
				if cfg.Proxy.Port != 1080 {
					t.Errorf("Proxy.Port = %d, 期望 %d", cfg.Proxy.Port, 1080)
				}
				if cfg.Proxy.Username != "proxyuser" {
					t.Errorf("Proxy.Username = %q, 期望 %q", cfg.Proxy.Username, "proxyuser")
				}
				if cfg.Proxy.Password != "proxypass" {
					t.Errorf("Proxy.Password = %q, 期望 %q", cfg.Proxy.Password, "proxypass")
				}
			},
		},
		{
			name: "无代理配置",
			account: models.Account{
				Email:      "test@example.com",
				Password:   "pass",
				IMAPServer: "imap.example.com",
				IMAPPort:   993,
				SSLMode:    "ssl",
				Proxy:      nil,
			},
			check: func(t *testing.T, cfg types.IMAPConfig) {
				if cfg.Proxy != nil {
					t.Error("Proxy 应该为 nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := NewWorker(tt.account)
			cfg, err := worker.buildIMAPConfig()
			if err != nil {
				t.Fatalf("buildIMAPConfig failed: %v", err)
			}
			tt.check(t, cfg)
		})
	}
}

// TestClassifyMailboxType_WithFolderRules 验证使用自定义规则的分类
func TestClassifyMailboxType_WithFolderRules(t *testing.T) {
	db := setupTestDB(t)

	// 添加自定义规则
	db.Create(&models.FolderRule{
		Name:       "Custom Newsletter",
		Pattern:    "(?i)(newsletter|邮件列表)",
		TargetType: "promotion",
		Order:      5,
	})

	tests := []struct {
		name       string
		providerID string
		mailbox    string
		path       string
		expected   string
	}{
		{
			name:       "自定义规则匹配 newsletter",
			providerID: "",
			mailbox:    "Newsletter",
			path:       "Newsletter",
			expected:   "promotion",
		},
		{
			name:       "自定义规则匹配中文 邮件列表",
			providerID: "",
			mailbox:    "邮件列表",
			path:       "邮件列表",
			expected:   "promotion",
		},
		{
			name:       "默认规则匹配 Spam",
			providerID: "",
			mailbox:    "Spam",
			path:       "Spam",
			expected:   "spam",
		},
		{
			name:       "默认规则匹配 Trash",
			providerID: "",
			mailbox:    "Trash",
			path:       "Trash",
			expected:   "trash",
		},
		{
			name:       "默认规则匹配 Sent",
			providerID: "",
			mailbox:    "Sent",
			path:       "Sent",
			expected:   "sent",
		},
		{
			name:       "无匹配返回 unknown",
			providerID: "",
			mailbox:    "MyCustomFolder",
			path:       "MyCustomFolder",
			expected:   "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyMailboxType(tt.providerID, tt.mailbox, tt.path)
			if result != tt.expected {
				t.Errorf("classifyMailboxType(%q, %q, %q) = %q, 期望 %q",
					tt.providerID, tt.mailbox, tt.path, result, tt.expected)
			}
		})
	}
}

// TestGetPriorityByType 验证邮件类型到优先级的映射
func TestGetPriorityByType(t *testing.T) {
	setupTestDB(t)

	tests := []struct {
		name     string
		mailType string
		expected EventPriority
	}{
		{
			name:     "primary 映射到 PriorityNormal (10)",
			mailType: "primary",
			expected: PriorityNormal,
		},
		{
			name:     "bill 映射到 PriorityNormal (10)",
			mailType: "bill",
			expected: PriorityNormal,
		},
		{
			name:     "notification 映射到 PriorityNormal (10)",
			mailType: "notification",
			expected: PriorityNormal,
		},
		{
			name:     "important 映射到 PriorityHigh (20)",
			mailType: "important",
			expected: PriorityHigh,
		},
		{
			name:     "spam 映射到 PriorityLow (0)",
			mailType: "spam",
			expected: PriorityLow,
		},
		{
			name:     "trash 映射到 PriorityLow (0)",
			mailType: "trash",
			expected: PriorityLow,
		},
		{
			name:     "promotion 映射到 PriorityLow (0)",
			mailType: "promotion",
			expected: PriorityLow,
		},
		{
			name:     "未知类型映射到 PriorityNormal (默认)",
			mailType: "nonexistent",
			expected: PriorityNormal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPriorityByType(tt.mailType)
			if result != tt.expected {
				t.Errorf("getPriorityByType(%q) = %v, 期望 %v", tt.mailType, result, tt.expected)
			}
		})
	}
}
