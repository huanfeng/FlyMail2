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
	"flymail/internal/server"
	"flymail/modules/auth"
	"flymail/modules/email/account"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	"flymail/modules/email/send"
	syncmod "flymail/modules/email/sync"
	"flymail/modules/system/setting"
)

type App struct {
	cfg  *config.Config
	srv  *http.Server
	addr string
}

// New 构建 App：开库、迁移、装配 handler。
func New(cfg *config.Config) (*App, error) {
	db, err := database.Open(cfg.DBPath())
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(db); err != nil {
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
	handler := server.New(server.Deps{
		Auth:    authSvc,
		Account: accountSvc,
		Folder:  folderSvc,
		Message: messageSvc,
		Sync:    syncSvc,
		Setting: settingSvc,
		Send:    sendSvc,
	})
	return &App{cfg: cfg, srv: &http.Server{Handler: handler}}, nil
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
	return a.addr, nil
}

func (a *App) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.srv.Shutdown(ctx)
}
