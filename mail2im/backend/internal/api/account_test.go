package api

import (
	"mail2im/internal/core"
	"mail2im/internal/models"
	"testing"
)

// TestToAccount_BasicFields 验证基本字段映射
func TestToAccount_BasicFields(t *testing.T) {
	// 初始化加密模块
	core.InitCrypto("test-secret-key-32bytes-long!!")

	tests := []struct {
		name     string
		input    CreateAccountRequest
		existing *models.Account
		encrypt  bool
		check    func(t *testing.T, account *models.Account)
	}{
		{
			name: "基本字段映射",
			input: CreateAccountRequest{
				Email:       "test@example.com",
				DisplayName: "Test User",
				Login:       "test@example.com",
				Password:    "password123",
				AuthType:    "basic",
				Provider:    "gmail",
				IMAPServer:  "imap.gmail.com",
				IMAPPort:    993,
				SSLMode:     "ssl",
			},
			existing: nil,
			encrypt:  true,
			check: func(t *testing.T, account *models.Account) {
				if account.Email != "test@example.com" {
					t.Errorf("Email = %q, 期望 %q", account.Email, "test@example.com")
				}
				if account.DisplayName != "Test User" {
					t.Errorf("DisplayName = %q, 期望 %q", account.DisplayName, "Test User")
				}
				if account.Login != "test@example.com" {
					t.Errorf("Login = %q, 期望 %q", account.Login, "test@example.com")
				}
				if account.AuthType != "basic" {
					t.Errorf("AuthType = %q, 期望 %q", account.AuthType, "basic")
				}
				if account.Provider != "gmail" {
					t.Errorf("Provider = %q, 期望 %q", account.Provider, "gmail")
				}
				if account.IMAPServer != "imap.gmail.com" {
					t.Errorf("IMAPServer = %q, 期望 %q", account.IMAPServer, "imap.gmail.com")
				}
				if account.IMAPPort != 993 {
					t.Errorf("IMAPPort = %d, 期望 %d", account.IMAPPort, 993)
				}
				if account.SSLMode != "ssl" {
					t.Errorf("SSLMode = %q, 期望 %q", account.SSLMode, "ssl")
				}
				if !account.UseSSL {
					t.Error("UseSSL 应该为 true")
				}
				if account.Status != "Active" {
					t.Errorf("Status = %q, 期望 %q", account.Status, "Active")
				}
				if !account.Enabled {
					t.Error("Enabled 应该为 true")
				}
			},
		},
		{
			name: "Login 为空时使用 Email",
			input: CreateAccountRequest{
				Email:    "user@example.com",
				Login:    "",
				Password: "pass",
			},
			existing: nil,
			encrypt:  false,
			check: func(t *testing.T, account *models.Account) {
				if account.Login != "user@example.com" {
					t.Errorf("Login = %q, 期望 %q", account.Login, "user@example.com")
				}
			},
		},
		{
			name: "SSLMode 为空且 UseSSL 为 true 时设置为 ssl",
			input: CreateAccountRequest{
				Email:  "test@example.com",
				UseSSL: true,
			},
			existing: nil,
			encrypt:  false,
			check: func(t *testing.T, account *models.Account) {
				if account.SSLMode != "ssl" {
					t.Errorf("SSLMode = %q, 期望 %q", account.SSLMode, "ssl")
				}
				if !account.UseSSL {
					t.Error("UseSSL 应该为 true")
				}
			},
		},
		{
			name: "SSLMode 为空且 UseSSL 为 false 时设置为 none",
			input: CreateAccountRequest{
				Email:  "test@example.com",
				UseSSL: false,
			},
			existing: nil,
			encrypt:  false,
			check: func(t *testing.T, account *models.Account) {
				if account.SSLMode != "none" {
					t.Errorf("SSLMode = %q, 期望 %q", account.SSLMode, "none")
				}
				if account.UseSSL {
					t.Error("UseSSL 应该为 false")
				}
			},
		},
		{
			name: "密码加密",
			input: CreateAccountRequest{
				Email:    "test@example.com",
				Password: "my-password",
			},
			existing: nil,
			encrypt:  true,
			check: func(t *testing.T, account *models.Account) {
				if account.Password == "my-password" {
					t.Error("密码应该被加密")
				}
				if account.Password == "" {
					t.Error("密码不应该为空")
				}
			},
		},
		{
			name: "密码不加密",
			input: CreateAccountRequest{
				Email:    "test@example.com",
				Password: "my-password",
			},
			existing: nil,
			encrypt:  false,
			check: func(t *testing.T, account *models.Account) {
				if account.Password != "my-password" {
					t.Errorf("Password = %q, 期望 %q", account.Password, "my-password")
				}
			},
		},
		{
			name: "更新现有账户",
			input: CreateAccountRequest{
				Email:       "updated@example.com",
				DisplayName: "Updated Name",
			},
			existing: &models.Account{
				Email:       "old@example.com",
				DisplayName: "Old Name",
				Status:      "Active",
			},
			encrypt: false,
			check: func(t *testing.T, account *models.Account) {
				if account.Email != "updated@example.com" {
					t.Errorf("Email = %q, 期望 %q", account.Email, "updated@example.com")
				}
				if account.DisplayName != "Updated Name" {
					t.Errorf("DisplayName = %q, 期望 %q", account.DisplayName, "Updated Name")
				}
				// 保留现有的 Status
				if account.Status != "Active" {
					t.Errorf("Status = %q, 期望 %q", account.Status, "Active")
				}
			},
		},
		{
			name: "Enabled 字段显式设置",
			input: CreateAccountRequest{
				Email:   "test@example.com",
				Enabled: boolPtr(false),
			},
			existing: nil,
			encrypt:  false,
			check: func(t *testing.T, account *models.Account) {
				if account.Enabled {
					t.Error("Enabled 应该为 false")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account, err := toAccount(tt.input, tt.existing, tt.encrypt)
			if err != nil {
				t.Fatalf("toAccount() 返回错误: %v", err)
			}
			tt.check(t, account)
		})
	}
}

// TestToAccount_PasswordEncryption 验证密码加密逻辑
func TestToAccount_PasswordEncryption(t *testing.T) {
	core.InitCrypto("test-secret-key-32bytes-long!!")

	input := CreateAccountRequest{
		Email:    "test@example.com",
		Password: "my-secret-password",
	}

	// 测试加密
	account1, err := toAccount(input, nil, true)
	if err != nil {
		t.Fatalf("toAccount() 返回错误: %v", err)
	}

	// 验证密码被加密
	if account1.Password == "my-secret-password" {
		t.Error("密码应该被加密")
	}

	// 验证可以解密
	decrypted, err := core.Decrypt(account1.Password)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if decrypted != "my-secret-password" {
		t.Errorf("解密后密码 = %q, 期望 %q", decrypted, "my-secret-password")
	}

	// 测试不加密
	account2, err := toAccount(input, nil, false)
	if err != nil {
		t.Fatalf("toAccount() 返回错误: %v", err)
	}

	if account2.Password != "my-secret-password" {
		t.Errorf("密码 = %q, 期望 %q", account2.Password, "my-secret-password")
	}
}

// TestToAccount_NilExisting 验证 existing 为 nil 的情况
func TestToAccount_NilExisting(t *testing.T) {
	core.InitCrypto("test-secret-key-32bytes-long!!")

	input := CreateAccountRequest{
		Email:    "test@example.com",
		Password: "password",
	}

	account, err := toAccount(input, nil, false)
	if err != nil {
		t.Fatalf("toAccount() 返回错误: %v", err)
	}

	if account == nil {
		t.Fatal("account 不应该为 nil")
	}

	// 验证默认值
	if account.Status != "Active" {
		t.Errorf("Status = %q, 期望 %q", account.Status, "Active")
	}
	if !account.Enabled {
		t.Error("Enabled 应该为 true")
	}
}

// boolPtr 返回一个指向 bool 值的指针
func boolPtr(b bool) *bool {
	return &b
}
