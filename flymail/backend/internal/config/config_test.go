package config

import (
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(LoadOptions{DataDir: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("default port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("default host = %q, want 127.0.0.1", cfg.Server.Host)
	}
	if cfg.LogDir() != filepath.Join(dir, "logs") {
		t.Errorf("default LogDir = %q, want %q", cfg.LogDir(), filepath.Join(dir, "logs"))
	}
	if cfg.Auth.AccessTokenTTL != 15 {
		t.Errorf("default access_token_ttl = %d, want 15", cfg.Auth.AccessTokenTTL)
	}
	if cfg.Auth.RefreshTokenTTL != 168 {
		t.Errorf("default refresh_token_ttl = %d, want 168", cfg.Auth.RefreshTokenTTL)
	}
	if cfg.DataDir != dir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, dir)
	}
	if cfg.DBPath() != filepath.Join(dir, "flymail.db") {
		t.Errorf("DBPath = %q", cfg.DBPath())
	}
}

func TestEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLYMAIL_SERVER_PORT", "9090")
	cfg, err := Load(LoadOptions{DataDir: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("env override port = %d, want 9090", cfg.Server.Port)
	}
}

func TestLoadMissingConfigFileIgnored(t *testing.T) {
	dir := t.TempDir()
	missingFile := filepath.Join(t.TempDir(), "nope.yaml")
	cfg, err := Load(LoadOptions{DataDir: dir, ConfigFile: missingFile})
	if err != nil {
		t.Fatalf("Load with missing --config file should not error, got: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("port = %d, want default 8080", cfg.Server.Port)
	}
}

func TestCryptoKeyDefaultAndEnv(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(LoadOptions{DataDir: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Crypto.EncryptionKey == "" {
		t.Error("默认加密密钥不应为空")
	}

	t.Setenv("FLYMAIL_CRYPTO_ENCRYPTION_KEY", "my-custom-key-1234567890")
	cfg2, err := Load(LoadOptions{DataDir: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg2.Crypto.EncryptionKey != "my-custom-key-1234567890" {
		t.Errorf("env 覆盖密钥失败，得到 %q", cfg2.Crypto.EncryptionKey)
	}
}

// TestTrustedProxiesFromEnv 覆盖 Docker + 反向代理部署：可信代理只能经环境变量注入。
// 未注册默认值的 key 不会被 viper 的 AutomaticEnv 读取，而这项读不到的后果是
// 登录限流按代理 IP 计数，一个人触发就锁住整站。
func TestTrustedProxiesFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLYMAIL_SERVER_TRUSTED_PROXIES", "172.18.0.0/16,127.0.0.1")
	cfg, err := Load(LoadOptions{DataDir: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"172.18.0.0/16", "127.0.0.1"}
	if len(cfg.Server.TrustedProxies) != len(want) {
		t.Fatalf("trusted_proxies = %v，期望 %v", cfg.Server.TrustedProxies, want)
	}
	for i, w := range want {
		if cfg.Server.TrustedProxies[i] != w {
			t.Errorf("trusted_proxies[%d] = %q，期望 %q", i, cfg.Server.TrustedProxies[i], w)
		}
	}
}

// TestTrustedProxiesDefaultEmpty 默认必须是空：gin 默认信任所有代理，
// 那会让任何客户端都能用 X-Forwarded-For 伪造来源 IP 绕过限流。
func TestTrustedProxiesDefaultEmpty(t *testing.T) {
	cfg, err := Load(LoadOptions{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Server.TrustedProxies) != 0 {
		t.Errorf("默认应不信任任何代理，实际 %v", cfg.Server.TrustedProxies)
	}
}
