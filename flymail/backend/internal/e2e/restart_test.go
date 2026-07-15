package e2e

import (
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"flymail/internal/app"
	"flymail/internal/config"
	"flymail/internal/database"
	syncmod "flymail/modules/email/sync"

	imapv2 "github.com/emersion/go-imap/v2"
)

// startAppOn 在给定 cfg 上构建并起 app（httptest server）；调用方负责 srv.Close + app.Shutdown。
func startAppOn(t *testing.T, cfg *config.Config, startBackground bool) (*app.App, *httptest.Server) {
	t.Helper()
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	seedAdmin(t, cfg)
	srv := httptest.NewServer(a.Handler())
	if startBackground {
		a.StartBackground()
	}
	return a, srv
}

// TestRestart_WritebackRecovery 验证回写持久队列的重启恢复：
// app1 同步出邮件后关闭 → 向同一 DataDir 的库注入一条待回写 op（模拟崩溃前已入队未执行）→
// app2 起于同 DataDir 并启动后台 → 启动恢复把回写应用到 IMAP，服务器端最终出现 \Seen。
func TestRestart_WritebackRecovery(t *testing.T) {
	requireE2E(t)
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

	// ── 阶段一：app1 建账户、投递邮件、同步入库 ──
	a1, srv1 := startAppOn(t, cfg, false)
	c := &apiClient{t: t, baseURL: srv1.URL}
	c.login(adminUser, adminPass)
	mb := uniqueMailbox(t)
	acctID := c.createAccount(mb)
	sendSeed(t, "seeder@localhost", mb, "restart-subj", "restart-body")
	c.triggerSyncAndWait(acctID, 60*time.Second)

	inbox := findFolder(c.listFolders(acctID), "inbox")
	if inbox == nil {
		t.Fatal("无 inbox")
	}
	msgs := c.listMessages(inbox.ID)
	if len(msgs) != 1 {
		t.Fatalf("邮件数=%d 期望 1", len(msgs))
	}
	uid := msgs[0].UID

	// 服务器端此刻应为未读。
	sess := imapConnect(t, mb)
	isRead := func() (read, ok bool) {
		if _, err := sess.SelectFolder("INBOX"); err != nil {
			return false, false
		}
		emails, err := sess.FetchByUIDs([]imapv2.UID{imapv2.UID(uid)}, coreimapFetchHeaders())
		if err != nil || len(emails) != 1 {
			return false, false
		}
		return emails[0].IsRead, true
	}
	if r, ok := isRead(); !ok || r {
		t.Fatalf("前置断言失败：应为未读（ok=%v read=%v）", ok, r)
	}

	// ── 关闭 app1（模拟进程退出，释放 SQLite）──
	srv1.Close()
	if err := a1.Shutdown(); err != nil {
		t.Fatalf("app1 shutdown: %v", err)
	}

	// ── 向持久库注入一条待回写 op（模拟崩溃前已入队、尚未执行）──
	db, err := database.Open(cfg.DBPath())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	op := &syncmod.WritebackOp{
		AccountID:     acctID,
		FolderPath:    "INBOX",
		UID:           uid,
		Op:            "read", // 对应 sync.wbOpRead
		NextAttemptAt: time.Now(),
	}
	if err := db.Create(op).Error; err != nil {
		t.Fatalf("注入待回写 op: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}

	// ── 阶段二：app2 起于同 DataDir 并启动后台，触发启动恢复 ──
	a2, srv2 := startAppOn(t, cfg, true)
	t.Cleanup(func() {
		srv2.Close()
		_ = a2.Shutdown()
	})

	// 启动恢复把回写应用到 IMAP：服务器端最终出现 \Seen。
	eventually(t, 30*time.Second, 500*time.Millisecond, `重启后持久回写恢复生效(\Seen)`, func() bool {
		read, ok := isRead()
		return ok && read
	})
}
