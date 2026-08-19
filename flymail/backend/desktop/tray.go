package main

import (
	"github.com/energye/systray"
)

// setupTray 在主 goroutine（main 函数内、wails.Run 之前）注册托盘。
//
// 必须用 RunWithExternalLoop 而非 Run：systray 的消息窗口在「调用注册的那个
// OS 线程」上创建，而 Windows 消息队列按线程隔离。Wails 占用主线程消息循环，
// 只有让托盘窗口也创建在主线程上，Wails 自己的消息泵才能把托盘点击分发到
// systray 的 wndProc（此前在普通 goroutine 里调 Run，窗口线程与泵线程分离，
// 图标显示但事件全部丢失）。
//
// 返回的 start/end 分别在 OnStartup/OnShutdown 中调用。
func (d *desktop) setupTray() (start, end func()) {
	return systray.RunWithExternalLoop(d.onTrayReady, nil)
}

func (d *desktop) onTrayReady() {
	systray.SetIcon(trayIconICO)
	systray.SetTooltip("FlyMail")

	// 回调经 wndProc 在主线程触发；用 go 脱离 wndProc 调用栈再调 Wails runtime，
	// 避免与主线程消息泵互相等待。
	systray.SetOnClick(func(systray.IMenu) { go d.showWindow() })
	systray.SetOnDClick(func(systray.IMenu) { go d.showWindow() })
	systray.SetOnRClick(func(menu systray.IMenu) {
		if menu != nil {
			_ = menu.ShowMenu()
		}
	})

	show := systray.AddMenuItem("显示主窗口", "打开 FlyMail 主界面")
	show.Click(func() { go d.showWindow() })
	systray.AddSeparator()
	quit := systray.AddMenuItem("退出", "停止后台收信并退出 FlyMail")
	quit.Click(func() { go d.quit() })
}
