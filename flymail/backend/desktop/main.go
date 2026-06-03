// Package main 是 FlyMail 的桌面（Wails）入口。
//
// 设计：复用 server 形态的同一 gin 引擎作为 Wails 的 AssetServer.Handler，
// 这样 SPA 与 /api/v1 全部走同源 wails:// 协议，无需 TCP 监听、无 CORS、零业务重写。
// 数据目录沿用 CLI 默认（config.Load 空选项 → ./data 或 FLYMAIL_DATA_DIR）。
package main

import (
	"context"
	"log"
	"os"

	"flymail/internal/app"
	"flymail/internal/config"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

func main() {
	// 数据目录与配置：与 CLI server 形态完全一致。
	cfg, err := config.Load(config.LoadOptions{})
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}

	a, err := app.New(cfg)
	if err != nil {
		log.Fatalf("初始化应用失败: %v", err)
	}

	err = wails.Run(&options.App{
		Title:     "FlyMail",
		Width:     1280,
		Height:    820,
		MinWidth:  960,
		MinHeight: 600,
		// 复用 gin 引擎：SPA + /api/v1 同源处理。
		AssetServer: &assetserver.Options{
			Handler: a.Handler(),
		},
		// 启动后开后台同步；关闭时优雅停机（停同步 + 关 HTTP + 关日志）。
		OnStartup: func(_ context.Context) {
			a.StartBackground()
		},
		OnShutdown: func(_ context.Context) {
			_ = a.Shutdown()
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
	if err != nil {
		log.Fatalf("Wails 运行失败: %v", err)
	}
}
