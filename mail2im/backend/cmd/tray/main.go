package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"fyne.io/systray"
	"go.uber.org/zap"

	"flymail-core/logger"
	"mail2im/internal/app"
	appconfig "mail2im/internal/config"
)

var (
	server     *app.Server
	serverLock sync.Mutex
)

func main() {
	if _, err := appconfig.Load(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetIcon(generateIcon())
	systray.SetTitle("Mail2IM")
	systray.SetTooltip("Mail2IM - Email to IM Forwarder")

	mOpen := systray.AddMenuItem("打开 Mail2IM", "在浏览器中打开")
	systray.AddSeparator()
	mStart := systray.AddMenuItem("启动服务", "启动邮件服务")
	mStop := systray.AddMenuItem("停止服务", "停止邮件服务")
	mStop.Disable()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出应用")

	// Click tray icon to open browser
	systray.SetOnTapped(func() {
		openBrowser()
	})

	// Auto-start server
	go startServer(mStart, mStop)

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openBrowser()

			case <-mStart.ClickedCh:
				go startServer(mStart, mStop)

			case <-mStop.ClickedCh:
				go stopServer(mStart, mStop)

			case <-mQuit.ClickedCh:
				stopServer(mStart, mStop)
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	serverLock.Lock()
	defer serverLock.Unlock()
	if server != nil && server.Running() {
		server.Stop()
	}
}

func startServer(mStart, mStop *systray.MenuItem) {
	serverLock.Lock()
	defer serverLock.Unlock()

	if server != nil && server.Running() {
		return
	}

	cfg := app.DefaultServerConfig()
	server = app.NewServer(cfg)

	if err := server.Start(); err != nil {
		logger.Error("启动服务失败", zap.Error(err))
		systray.SetTooltip("Mail2IM - 启动失败")
		return
	}

	mStart.Disable()
	mStop.Enable()
	systray.SetTooltip(fmt.Sprintf("Mail2IM - 运行中 (:%s)", server.Port()))
}

func stopServer(mStart, mStop *systray.MenuItem) {
	serverLock.Lock()
	defer serverLock.Unlock()

	if server == nil || !server.Running() {
		return
	}

	server.Stop()
	mStart.Enable()
	mStop.Disable()
	systray.SetTooltip("Mail2IM - 已停止")
}

func openBrowser() {
	// In dev mode (MAIL2IM_FRONTEND_PORT set), open frontend; otherwise open backend
	port := os.Getenv("MAIL2IM_FRONTEND_PORT")
	if port == "" {
		serverLock.Lock()
		if server != nil {
			port = server.Port()
		}
		serverLock.Unlock()
	}
	if port == "" {
		port = "8080"
	}

	url := fmt.Sprintf("http://localhost:%s", port)
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}
