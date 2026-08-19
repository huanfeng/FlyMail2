// Package main 是 FlyMail 的桌面（Wails）入口。
//
// 设计：复用 server 形态的同一 gin 引擎作为 Wails 的 AssetServer.Handler，
// 这样 SPA 与 /api/v1 全部走同源 wails:// 协议，无需 TCP 监听、无 CORS、零业务重写。
//
// 本地应用体验（区别于 server 形态）：
//   - 数据目录：默认 %APPDATA%\FlyMail；exe 旁放 portable.txt 则回到 <exe 目录>/data；
//     FLYMAIL_DATA_DIR 环境变量优先级最高（见 config.DesktopDataDir）。
//   - 首次运行自动创建默认管理员 admin/admin（开箱即用，可在设置中改密码）。
//   - 单实例：重复启动唤起已有窗口而非开第二个进程（避免 SQLite 双开）。
//   - 系统托盘：关闭窗口最小化到托盘继续后台收信，托盘菜单可显示窗口/退出。
//   - 新邮件弹 Windows 原生 toast 通知。
//   - 窗口尺寸/位置/最大化状态记忆（<数据目录>/window_state.json）。
package main

import (
	"context"
	_ "embed"
	"log"
	"os"
	"sync/atomic"

	"flymail/internal/app"
	"flymail/internal/config"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var trayIconICO []byte

// desktop 聚合桌面形态的窗口/托盘/通知状态。
type desktop struct {
	app      *app.App
	dataDir  string
	ctx      context.Context
	state    *windowState
	quitting atomic.Bool // 托盘「退出」置位：OnBeforeClose 放行真正关闭
	hidden   atomic.Bool // 窗口当前是否隐藏在托盘（隐藏时不再读取窗口几何）
}

func main() {
	// 数据目录：桌面专属解析（env > 便携标记 > %APPDATA%），与 CLI 的 ./data 默认不同。
	dataDir := config.DesktopDataDir()
	cfg, err := config.Load(config.LoadOptions{DataDir: dataDir})
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
	// 开箱即用：首次运行（库中无管理员）自动创建 admin/admin。
	if created, err := a.EnsureDefaultAdmin("admin", "admin"); err != nil {
		log.Printf("检查默认管理员失败: %v", err)
	} else if created {
		log.Printf("首次运行：已创建默认管理员 admin/admin，请登录后在设置中修改密码")
	}

	d := &desktop{app: a, dataDir: dataDir, state: loadWindowState(dataDir)}
	initToast() // Windows toast 需要注册表登记 AppID（best-effort）

	startState := options.Normal
	if d.state.Maximised {
		startState = options.Maximised
	}

	err = wails.Run(&options.App{
		Title:            "FlyMail",
		Width:            d.state.Width,
		Height:           d.state.Height,
		MinWidth:         960,
		MinHeight:        600,
		WindowStartState: startState,
		// 复用 gin 引擎：SPA + /api/v1 同源处理。
		AssetServer: &assetserver.Options{
			Handler: a.Handler(),
		},
		// 单实例：二次启动把参数发给已运行实例并触发回调（这里只负责唤起窗口）。
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "com.flymail.desktop",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { d.showWindow() },
		},
		OnStartup:     d.onStartup,
		OnBeforeClose: d.onBeforeClose,
		OnShutdown:    d.onShutdown,
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
	if err != nil {
		log.Fatalf("Wails 运行失败: %v", err)
	}
}

func (d *desktop) onStartup(ctx context.Context) {
	d.ctx = ctx
	d.restoreWindowPosition()
	d.app.StartBackground()
	// 新邮件 → Windows 原生通知（与站内通知中心并行，不替代）。
	d.app.SetEmitHook(d.onNotifyEvent)
	go d.runTray()
}

// onBeforeClose 拦截窗口关闭：默认最小化到托盘继续后台收信；
// 仅当托盘「退出」置位 quitting 后才放行真正退出。
func (d *desktop) onBeforeClose(ctx context.Context) bool {
	if d.quitting.Load() {
		return false
	}
	d.saveWindowState()
	d.hidden.Store(true)
	wailsrt.WindowHide(ctx)
	return true
}

func (d *desktop) onShutdown(context.Context) {
	d.stopTray()
	_ = d.app.Shutdown()
}

func (d *desktop) showWindow() {
	if d.ctx == nil {
		return
	}
	d.hidden.Store(false)
	wailsrt.WindowShow(d.ctx)
	wailsrt.WindowUnminimise(d.ctx)
}

// quit 由托盘「退出」触发：先记住窗口状态（若可见），再走正常退出流程。
func (d *desktop) quit() {
	d.quitting.Store(true)
	if !d.hidden.Load() {
		d.saveWindowState()
	}
	wailsrt.Quit(d.ctx)
}
