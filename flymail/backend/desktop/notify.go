package main

import (
	"log"

	"flymail/modules/system/notify"

	toast "git.sr.ht/~jackmordaunt/go-toast/v2"
)

const (
	toastAppID = "FlyMail"
	// toastGUID 是 Windows Runtime 通知激活用的 COM class GUID，随意生成但必须固定。
	toastGUID = "{8A7D2E5F-1B3C-4D9E-A6F0-2C4B8D7E9F13}"
)

// initToast 在注册表登记应用信息（AppID 等），Windows 通知中心据此展示应用名。
// 失败不致命：仅原生通知不可用，站内通知中心不受影响。
func initToast() {
	if err := toast.SetAppData(toast.AppData{AppID: toastAppID, GUID: toastGUID}); err != nil {
		log.Printf("注册 toast 应用信息失败（原生通知不可用）: %v", err)
	}
}

// onNotifyEvent 是注入 App 通知链的观察者：新邮件事件弹 Windows 原生 toast。
// 其余事件（同步失败/账户状态）留在站内通知中心，避免打扰。
func (d *desktop) onNotifyEvent(eventType string, _ uint, title, body string) {
	if eventType != string(notify.EventMailNew) {
		return
	}
	n := toast.Notification{AppID: toastAppID, Title: title, Body: body}
	if err := n.Push(); err != nil {
		log.Printf("发送系统通知失败: %v", err)
	}
}
