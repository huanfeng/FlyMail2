package main

import (
	"github.com/energye/systray"
)

// runTray 启动系统托盘。energye/systray 自带消息循环，可安全运行在
// goroutine（不与 Wails 主线程冲突，这正是选它而非 getlantern/systray 的原因）。
func (d *desktop) runTray() {
	systray.Run(d.onTrayReady, nil)
}

func (d *desktop) onTrayReady() {
	systray.SetIcon(trayIconICO)
	systray.SetTooltip("FlyMail")

	// 左键/双击：唤起主窗口；右键：弹菜单（energye 需显式绑定）。
	systray.SetOnClick(func(systray.IMenu) { d.showWindow() })
	systray.SetOnDClick(func(systray.IMenu) { d.showWindow() })
	systray.SetOnRClick(func(menu systray.IMenu) {
		if menu != nil {
			_ = menu.ShowMenu()
		}
	})

	show := systray.AddMenuItem("显示主窗口", "打开 FlyMail 主界面")
	show.Click(d.showWindow)
	systray.AddSeparator()
	quit := systray.AddMenuItem("退出", "停止后台收信并退出 FlyMail")
	quit.Click(d.quit)
}

// stopTray 在应用关停时移除托盘图标。
func (d *desktop) stopTray() {
	systray.Quit()
}
