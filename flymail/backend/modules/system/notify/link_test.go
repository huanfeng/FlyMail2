package notify_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"flymail/modules/system/notify"
)

// 通知要带上「打开邮件」的直达链接。
//
// ── 为什么需要 ───────────────────────────────────────────────────────────────
//
// 外发通知是在别的应用里看到的（飞书、webhook 转发到手机）。光有标题和正文，
// 想处理还得自己切回来、翻到那封信。带一条直达链接，点一下就到。
//
// 链接由 Service 在 Emit 时拼，事件源（sync / rule）不必知道它的存在——
// 拼链接要读设置、还要查邮件属于哪个文件夹，那两件事都不该渗进同步逻辑。
func TestEmitAttachesLinkToOutbound(t *testing.T) {
	svc, _ := newSvc(t)
	ensureChannel(t, svc, notify.EventMailNew)
	svc.SetLinkBuilder(func(accountID, messageID uint) string {
		if messageID == 0 {
			return ""
		}
		return "https://mail.example.com/?account=1&folder=7&message=42"
	})

	got := emitAndCapture(t, svc, notify.Event{
		Type: notify.EventMailNew, AccountID: 1, MessageID: 42,
		Title: "新邮件 · Alice", Body: "主题\n正文摘要",
	})
	if got.URL != "https://mail.example.com/?account=1&folder=7&message=42" {
		t.Errorf("外发事件没带链接：URL=%q", got.URL)
	}
}

// ⚠ 没配对外访问地址时不带链接，而不是带一条半截的。
//
// 这条是主要的反向：一个「拼不出就用相对路径」或者「拿请求里的 Host 凑」的实现
// 会发出点了打不开的死链，而用户要到点击那一刻才发现——比不带链接更糟。
func TestEmitWithoutBaseURLSendsNoLink(t *testing.T) {
	svc, _ := newSvc(t)
	ensureChannel(t, svc, notify.EventMailNew)

	// 完全没设构造器（等价于 app 没装配）
	got := emitAndCapture(t, svc, notify.Event{
		Type: notify.EventMailNew, AccountID: 1, MessageID: 42, Title: "新邮件",
	})
	if got.URL != "" {
		t.Errorf("没装配构造器却带了链接：%q", got.URL)
	}

	// 构造器返回空串（用户把地址清空了）
	svc.SetLinkBuilder(func(uint, uint) string { return "" })
	got = emitAndCapture(t, svc, notify.Event{
		Type: notify.EventMailNew, AccountID: 1, MessageID: 42, Title: "新邮件",
	})
	if got.URL != "" {
		t.Errorf("地址为空却带了链接：%q", got.URL)
	}
}

// 链接按**发通知那一刻**的设置拼，不是启动时算一次。
//
// 用户改完地址之后发出的通知必须用新地址；缓存的话会继续发旧链接，
// 而且「改了设置没生效」这种现象极难排查。
func TestLinkUsesCurrentSettingAtEmitTime(t *testing.T) {
	svc, _ := newSvc(t)
	ensureChannel(t, svc, notify.EventMailNew)
	base := "https://old.example.com"
	svc.SetLinkBuilder(func(uint, uint) string { return base + "/?message=1" })

	got := emitAndCapture(t, svc, notify.Event{Type: notify.EventMailNew, MessageID: 1, Title: "t"})
	if !strings.HasPrefix(got.URL, "https://old.example.com") {
		t.Fatalf("前提不成立：%q", got.URL)
	}

	base = "https://new.example.com"
	got = emitAndCapture(t, svc, notify.Event{Type: notify.EventMailNew, MessageID: 1, Title: "t"})
	if !strings.HasPrefix(got.URL, "https://new.example.com") {
		t.Errorf("改了地址之后仍在发旧链接：%q", got.URL)
	}
}

// 「测试」按钮发出的消息也要带链接。
//
// ── 为什么这条重要 ───────────────────────────────────────────────────────────
//
// 这个按钮是「对外访问地址」唯一的即时反馈。格式校验挡得住 `mail.example.com`
// 这种写法，挡不住「格式完全正确但主机填错了」——填成容器内网名、填错端口、
// 反代路径没带上。这几种情况下通知照发、链接照带，**只有点下去才知道是死链**。
//
// ⚠ 它绕过 Emit（测试要同步拿到投递结果，也不该在通知中心留一条站内记录），
// 而 URL 恰恰是 Emit 补的。所以这条路径必须自己补链接，很容易漏。
func TestTestChannelCarriesLink(t *testing.T) {
	svc, _ := newSvc(t)
	enabled := true
	ch, err := svc.CreateChannel(notify.ChannelInput{
		Name: "wh", Kind: "webhook", URL: "http://x",
		Events: []string{string(notify.EventMailNew)}, Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	svc.SetLinkBuilder(func(uint, uint) string { return "https://mail.example.com/" })

	var got notify.Event
	svc.SetDispatcher(func(_ *notify.Channel, e notify.Event) error {
		got = e
		return nil
	})
	if err := svc.TestChannel(ch.ID); err != nil {
		t.Fatalf("TestChannel: %v", err)
	}
	if got.URL != "https://mail.example.com/" {
		t.Errorf("测试消息没带链接（URL=%q）——用户点了「测试」也验不出地址填对没有", got.URL)
	}
}

// 反向：没配地址时测试消息也不该带一条半截的链接。
func TestTestChannelWithoutBaseURLSendsNoLink(t *testing.T) {
	svc, _ := newSvc(t)
	enabled := true
	ch, err := svc.CreateChannel(notify.ChannelInput{
		Name: "wh", Kind: "webhook", URL: "http://x",
		Events: []string{string(notify.EventMailNew)}, Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	var got notify.Event
	svc.SetDispatcher(func(_ *notify.Channel, e notify.Event) error {
		got = e
		return nil
	})
	if err := svc.TestChannel(ch.ID); err != nil {
		t.Fatalf("TestChannel: %v", err)
	}
	if got.URL != "" {
		t.Errorf("没配地址却带了链接：%q", got.URL)
	}
}

// emitAndCapture 发一条通知并等外发投递完成，返回投递出去的那个事件。
//
// 走真实的 Emit（而不是直接调内部拼接函数）：要验的正是「Emit 会替事件源把链接
// 填上」这件事，绕过它就只是在测一个自己写的辅助函数。
//
// ⚠ 必须等到**投递日志落库**才返回，不能拿到 dispatcher 回调就走。
// Emit 是异步的：worker 调完 dispatcher 之后还要写一行 notify_logs，
// 测试一结束 t.Cleanup 就关库，那行写入会撞上「database is closed」，
// 临时目录也会因为 SQLite 还没放手而删不掉（TempDir RemoveAll: directory not empty）。
// 这条竞态让本包的测试偶发报错，排查了一轮才定位到。
func emitAndCapture(t *testing.T, svc *notify.Service, evt notify.Event) notify.Event {
	t.Helper()

	var mu sync.Mutex
	var got notify.Event
	svc.SetDispatcher(func(_ *notify.Channel, e notify.Event) error {
		mu.Lock()
		got = e
		mu.Unlock()
		return nil
	})

	before, err := svc.ListLogs(1000)
	if err != nil {
		t.Fatalf("ListLogs: %v", err)
	}

	svc.Emit(evt)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		logs, err := svc.ListLogs(1000)
		if err == nil && len(logs) > len(before) {
			mu.Lock()
			defer mu.Unlock()
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("外发投递没有完成（notify_logs 一直没有新行）")
	return notify.Event{}
}

// ensureChannel 建一个订阅了该事件的启用渠道；同名重复建会失败，所以只建一次。
func ensureChannel(t *testing.T, svc *notify.Service, eventType notify.EventType) {
	t.Helper()
	enabled := true
	if _, err := svc.CreateChannel(notify.ChannelInput{
		Name: "wh", Kind: "webhook", URL: "http://x",
		Events: []string{string(eventType)}, Enabled: &enabled,
	}); err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
}
