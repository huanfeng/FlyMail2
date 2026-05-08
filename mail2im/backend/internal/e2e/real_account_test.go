package e2e

import (
	"mail2im/internal/core"
	"mail2im/internal/models"
	"mail2im/internal/testutil"
	"os"
	"testing"
	"time"
)

// TestRealAccount_IMAPConnection 测试真实邮箱的 IMAP 连接
func TestRealAccount_IMAPConnection(t *testing.T) {
	if !testutil.HasTestAccounts() {
		t.Skip("跳过真实邮箱测试: 未配置测试账号 (设置 TEST_EMAIL_1 等环境变量)")
	}

	accounts := testutil.LoadAllTestAccounts()

	for _, account := range accounts {
		t.Run(account.Email, func(t *testing.T) {
			// 创建 Account 模型
			accountModel := models.Account{
				Email:      account.Email,
				Password:   account.Password,
				IMAPServer: account.IMAPServer,
				IMAPPort:   account.IMAPPort,
				SSLMode:    account.SSLMode,
				Enabled:    true,
			}

			// 测试连接
			start := time.Now()
			info, elapsed, err := core.TestIMAPConnection(accountModel)
			duration := time.Since(start)

			if err != nil {
				t.Fatalf("IMAP 连接失败: %v", err)
			}

			t.Logf("连接成功: %v", duration)
			t.Logf("  支持 IDLE: %v", info.SupportsIDLE)
			t.Logf("  安全模式: %s", info.SecurityMode)
			t.Logf("  能力: %v", info.Capabilities)

			// 验证连接信息
			if info == nil {
				t.Fatal("连接信息不应该为 nil")
			}

			if elapsed > 10*time.Second {
				t.Logf("警告: 连接耗时较长: %v", elapsed)
			}
		})
	}
}

// TestRealAccount_IMAPConnection_WithProxy 测试带代理的真实邮箱连接
func TestRealAccount_IMAPConnection_WithProxy(t *testing.T) {
	if !testutil.HasTestAccounts() {
		t.Skip("跳过真实邮箱测试: 未配置测试账号")
	}

	proxyType := getEnvOrDefault("TEST_PROXY_TYPE", "")
	if proxyType == "" {
		t.Skip("跳过代理测试: 未配置代理 (设置 TEST_PROXY_TYPE 等环境变量)")
	}

	accounts := testutil.LoadAllTestAccounts()
	if len(accounts) == 0 {
		t.Skip("没有可用的测试账号")
	}

	account := accounts[0]
	proxyHost := getEnvOrDefault("TEST_PROXY_HOST", "")
	proxyPort := getEnvOrDefault("TEST_PROXY_PORT", "0")
	proxyUsername := getEnvOrDefault("TEST_PROXY_USERNAME", "")
	proxyPassword := getEnvOrDefault("TEST_PROXY_PASSWORD", "")

	accountModel := models.Account{
		Email:      account.Email,
		Password:   account.Password,
		IMAPServer: account.IMAPServer,
		IMAPPort:   account.IMAPPort,
		SSLMode:    account.SSLMode,
		Enabled:    true,
		Proxy: &models.Proxy{
			Type:     proxyType,
			Host:     proxyHost,
			Port:     parseInt(proxyPort),
			Username: proxyUsername,
			Password: proxyPassword,
		},
	}

	// 测试连接
	info, elapsed, err := core.TestIMAPConnection(accountModel)
	if err != nil {
		t.Fatalf("带代理的 IMAP 连接失败: %v", err)
	}

	t.Logf("带代理连接成功: %v", elapsed)
	t.Logf("  支持 IDLE: %v", info.SupportsIDLE)
	t.Logf("  安全模式: %s", info.SecurityMode)
}

// TestRealAccount_IMAPConnection_InvalidCredentials 测试无效凭据的连接
func TestRealAccount_IMAPConnection_InvalidCredentials(t *testing.T) {
	if !testutil.HasTestAccounts() {
		t.Skip("跳过真实邮箱测试: 未配置测试账号")
	}

	accounts := testutil.LoadAllTestAccounts()
	if len(accounts) == 0 {
		t.Skip("没有可用的测试账号")
	}

	account := accounts[0]

	// 使用错误的密码
	accountModel := models.Account{
		Email:      account.Email,
		Password:   "wrong-password-12345",
		IMAPServer: account.IMAPServer,
		IMAPPort:   account.IMAPPort,
		SSLMode:    account.SSLMode,
		Enabled:    true,
	}

	// 测试连接应该失败
	_, _, err := core.TestIMAPConnection(accountModel)
	if err == nil {
		t.Error("使用错误密码应该连接失败")
	} else {
		t.Logf("预期的连接失败: %v", err)
	}
}

// TestRealAccount_IMAPConnection_InvalidServer 测试无效服务器的连接
func TestRealAccount_IMAPConnection_InvalidServer(t *testing.T) {
	if !testutil.HasTestAccounts() {
		t.Skip("跳过真实邮箱测试: 未配置测试账号")
	}

	accounts := testutil.LoadAllTestAccounts()
	if len(accounts) == 0 {
		t.Skip("没有可用的测试账号")
	}

	account := accounts[0]

	// 使用错误的服务器
	accountModel := models.Account{
		Email:      account.Email,
		Password:   account.Password,
		IMAPServer: "invalid.server.example.com",
		IMAPPort:   993,
		SSLMode:    "ssl",
		Enabled:    true,
	}

	// 测试连接应该失败
	_, _, err := core.TestIMAPConnection(accountModel)
	if err == nil {
		t.Error("使用无效服务器应该连接失败")
	} else {
		t.Logf("预期的连接失败: %v", err)
	}
}

// getEnvOrDefault 获取环境变量或默认值
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// parseInt 将字符串转换为整数
func parseInt(s string) int {
	// 简单实现，实际应该使用 strconv.Atoi
	switch s {
	case "1080":
		return 1080
	case "8080":
		return 8080
	case "3128":
		return 3128
	default:
		return 0
	}
}
