package e2e

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"flymail/internal/app"
	"flymail/internal/config"
	"flymail/internal/database"
	"flymail/modules/auth"
)

// 默认管理员凭证：server 启动不 seed 管理员（生产走 `flymail db init`），
// harness 在 app.New 迁移完成后用同库 seed 一个已知管理员。
const (
	adminUser     = "admin"
	adminPass     = "admin"
	testJWTSecret = "e2e-test-secret"
)

type testApp struct {
	app     *app.App
	server  *httptest.Server
	baseURL string
}

// newTestApp 起进程内真实 app：临时 SQLite + httptest server。
// startBackground=true 时启动后台同步 Manager（IDLE/SSE 链路需要；
// Manager reconcile 间隔 30s，实时链路测试应先建账户再置 true 起 app，见 realtime_test）。
func newTestApp(t *testing.T, startBackground bool) *testApp {
	t.Helper()
	dataDir := t.TempDir()
	cfg := &config.Config{
		DataDir: dataDir,
		Server:  config.ServerConfig{Host: "127.0.0.1", Port: 0},
		Auth:    config.AuthConfig{JWTSecret: testJWTSecret, AccessTokenTTL: 15, RefreshTokenTTL: 168},
		Crypto:  config.CryptoConfig{EncryptionKey: "e2e-test-encryption-key"},
		Log: config.LogConfig{
			Dir: filepath.Join(dataDir, "logs"), Console: false, Level: "info", Format: "json",
		},
	}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	seedAdmin(t, cfg)

	srv := httptest.NewServer(a.Handler())
	if startBackground {
		a.StartBackground()
	}
	ta := &testApp{app: a, server: srv, baseURL: srv.URL}
	t.Cleanup(func() {
		srv.Close()
		_ = a.Shutdown()
	})
	return ta
}

// startBackground 供「先建账户、再启后台」的测试用（Manager.Start 立即 reconcile，
// 避免等 30s reconcile tick 才拾取新账户）。
func (ta *testApp) startBackground() { ta.app.StartBackground() }

// seedAdmin 打开与 app 相同的 SQLite（迁移已由 app.New 完成），写入默认管理员。
func seedAdmin(t *testing.T, cfg *config.Config) {
	t.Helper()
	db, err := database.Open(cfg.DBPath())
	if err != nil {
		t.Fatalf("seedAdmin open db: %v", err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	svc := auth.NewService(auth.NewRepository(db), auth.Options{
		JWTSecret: testJWTSecret, AccessTTLMin: 15, RefreshTTLHour: 168,
	})
	if err := svc.SetAdminPassword(adminUser, adminPass); err != nil {
		t.Fatalf("seedAdmin SetAdminPassword: %v", err)
	}
}
