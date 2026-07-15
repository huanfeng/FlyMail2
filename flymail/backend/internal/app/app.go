package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"flymail/internal/config"
	"flymail/internal/crypto"
	"flymail/internal/database"
	"flymail/internal/logging"
	"flymail/internal/server"
	"flymail/internal/sse"
	"flymail/modules/auth"
	"flymail/modules/email/account"
	"flymail/modules/email/draft"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	"flymail/modules/email/send"
	syncmod "flymail/modules/email/sync"
	"flymail/modules/system/monitoring"
	"flymail/modules/system/notify"
	"flymail/modules/system/setting"

	"gorm.io/gorm"
)

// appVersion 监控/关于展示用的版本号（后续可由构建注入）。
const appVersion = "dev-preview"

type App struct {
	cfg      *config.Config
	srv      *http.Server
	addr     string
	manager  *syncmod.Manager
	cancel   context.CancelFunc
	logClose func() error
	db       *gorm.DB
}

// New 构建 App：初始化日志、开库、迁移、装配 handler。
func New(cfg *config.Config) (*App, error) {
	// 最先初始化统一日志：core zap 全局单例，标准库 log 经 RedirectStdLog 收编。
	logClose, err := logging.Setup(logging.Options{
		Dir:        cfg.LogDir(),
		MaxSizeMB:  cfg.Log.MaxSizeMB,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAgeDays: cfg.Log.MaxAgeDays,
		Compress:   cfg.Log.Compress,
		Console:    cfg.Log.Console,
		Level:      cfg.Log.Level,
		Format:     cfg.Log.Format,
	})
	if err != nil {
		return nil, err
	}

	db, err := database.Open(cfg.DBPath())
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(db); err != nil {
		return nil, err
	}
	// 回写队列表随 sync 包迁移（database 包不反向依赖 sync，避免测试期 import cycle）。
	if err := syncmod.MigrateWriteback(db); err != nil {
		return nil, err
	}
	authSvc := auth.NewService(auth.NewRepository(db), auth.Options{
		JWTSecret:      cfg.Auth.JWTSecret,
		AccessTTLMin:   cfg.Auth.AccessTokenTTL,
		RefreshTTLHour: cfg.Auth.RefreshTokenTTL,
	})
	enc, err := crypto.New(cfg.Crypto.EncryptionKey)
	if err != nil {
		return nil, err
	}
	accountSvc := account.NewService(account.NewRepository(db), enc)
	folderSvc := folder.NewService(folder.NewRepository(db))
	messageSvc := message.NewService(message.NewRepository(db), message.NewBodyRepository(db))
	syncSvc := syncmod.NewService(accountSvc, folderSvc, messageSvc)
	settingSvc := setting.NewService(setting.NewRepository(db))
	syncSvc.SetSyncDepthProvider(func() int { return settingSvc.GetInt(setting.KeySyncDepth, 1000) })
	sendSvc := send.NewService(accountSvc, folderSvc)
	draftSvc := draft.NewService(draft.NewRepository(db))

	// 通知中心：站内记录 + 外发推送。emit 回调注入到各事件源（解耦）。
	notifySvc := notify.NewService(notify.NewRepository(db))
	emit := notifySvc.EmitFunc()
	syncSvc.SetEmitter(emit)
	accountSvc.SetEmitter(emit)

	// SSE Hub + 后台同步管理器（IDLE + 轮询，新邮件经 Hub 推送）。
	hub := sse.NewHub()
	manager := syncmod.NewManager(accountSvc, folderSvc, messageSvc, hub)
	manager.SetEmitter(emit)
	manager.SetPollIntervalProvider(func() int { return settingSvc.GetInt(setting.KeySyncPollInterval, 180) })

	// 系统监控（只读聚合）
	monitoringSvc := monitoring.NewService(accountSvc, folderSvc, syncSvc, manager, time.Now(), appVersion, cfg.DBPath())
	eventsHandler := sse.NewHandler(hub, func(token string) error {
		_, err := authSvc.VerifyAccessToken(token)
		return err
	})

	handler := server.New(server.Deps{
		Auth:       authSvc,
		Account:    accountSvc,
		Folder:     folderSvc,
		Message:    messageSvc,
		Sync:       syncSvc,
		Setting:    settingSvc,
		Send:       sendSvc,
		Draft:      draftSvc,
		Notify:     notifySvc,
		Monitoring: monitoringSvc,
		Events:     eventsHandler,
		VerifyToken: func(token string) error {
			_, err := authSvc.VerifyAccessToken(token)
			return err
		},
	})
	return &App{cfg: cfg, srv: &http.Server{Handler: handler}, manager: manager, logClose: logClose, db: db}, nil
}

// Handler 返回装配好的 HTTP 处理器（gin 引擎）。
// 桌面形态（Wails）将其作为 AssetServer.Handler 复用，无需 TCP 监听。
func (a *App) Handler() http.Handler { return a.srv.Handler }

// StartBackground 启动后台同步管理器（IDLE + 轮询，Shutdown 时取消）。
// 桌面形态无需 HTTP 监听，单独调用此方法即可；server 形态由 Start 内部调用。
func (a *App) StartBackground() {
	if a.manager != nil {
		ctx, cancel := context.WithCancel(context.Background())
		a.cancel = cancel
		a.manager.Start(ctx)
	}
}

// Start 在指定地址监听（addr 为空则用配置 host:port）。返回实际监听地址。
func (a *App) Start(addr string) (string, error) {
	if addr == "" {
		addr = fmt.Sprintf("%s:%d", a.cfg.Server.Host, a.cfg.Server.Port)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	a.addr = ln.Addr().String()
	go func() { _ = a.srv.Serve(ln) }()

	a.StartBackground()
	return a.addr, nil
}

func (a *App) Shutdown() error {
	// 先停后台同步（取消 ctx 并等待 worker 退出），再关 HTTP。
	if a.cancel != nil {
		a.cancel()
	}
	if a.manager != nil {
		a.manager.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := a.srv.Shutdown(ctx)
	// 释放 SQLite 连接：否则文件句柄保持到进程退出（桌面形态/测试临时目录清理都需要）。
	if a.db != nil {
		if sqlDB, dbErr := a.db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	}
	if a.logClose != nil {
		_ = a.logClose() // 关闭日志文件句柄
	}
	return err
}
