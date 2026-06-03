package notify_test

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/modules/system/notify"
)

func newSvc(t *testing.T) (*notify.Service, *notify.Repository) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	repo := notify.NewRepository(db)
	return notify.NewService(repo), repo
}

func TestEmitPersistsAndDispatches(t *testing.T) {
	svc, repo := newSvc(t)

	var mu sync.Mutex
	var dispatched []notify.Event
	svc.SetDispatcher(func(c *notify.Channel, evt notify.Event) error {
		mu.Lock()
		defer mu.Unlock()
		dispatched = append(dispatched, evt)
		return nil
	})

	// 启用渠道，订阅 mail_new
	enabled := true
	if _, err := svc.CreateChannel(notify.ChannelInput{
		Name: "wh", Kind: "webhook", URL: "http://x", Events: []string{"mail_new"}, Enabled: &enabled,
	}); err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	svc.Emit(notify.Event{Type: notify.EventMailNew, Title: "新邮件", Body: "1 封"})

	// 站内通知应即时落库
	items, _ := svc.List(0, 50)
	if len(items) != 1 || items[0].Type != "mail_new" {
		t.Fatalf("站内通知未落库: %+v", items)
	}
	if n, _ := svc.UnreadCount(); n != 1 {
		t.Errorf("未读数应为 1，实际 %d", n)
	}

	// 异步投递：轮询日志
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if logs, _ := repo.ListLogs(10); len(logs) > 0 {
			if logs[0].Status != "ok" {
				t.Errorf("投递日志状态应为 ok: %+v", logs[0])
			}
			mu.Lock()
			n := len(dispatched)
			mu.Unlock()
			if n != 1 {
				t.Errorf("应投递 1 次，实际 %d", n)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("超时未见投递日志")
}

func TestEmitSkipsUnsubscribedChannel(t *testing.T) {
	svc, repo := newSvc(t)
	svc.SetDispatcher(func(c *notify.Channel, evt notify.Event) error { return nil })

	enabled := true
	// 只订阅 sync_failed，不应收到 mail_new
	_, _ = svc.CreateChannel(notify.ChannelInput{
		Name: "wh", Kind: "webhook", URL: "http://x", Events: []string{"sync_failed"}, Enabled: &enabled,
	})

	svc.Emit(notify.Event{Type: notify.EventMailNew, Title: "新邮件"})

	time.Sleep(200 * time.Millisecond)
	if logs, _ := repo.ListLogs(10); len(logs) != 0 {
		t.Errorf("未订阅渠道不应有投递日志: %+v", logs)
	}
}

func TestMarkAllRead(t *testing.T) {
	svc, _ := newSvc(t)
	svc.Emit(notify.Event{Type: notify.EventSyncFailed, Title: "失败"})
	svc.Emit(notify.Event{Type: notify.EventSyncFailed, Title: "失败2"})
	if n, _ := svc.UnreadCount(); n != 2 {
		t.Fatalf("未读应为 2，实际 %d", n)
	}
	if err := svc.MarkAllRead(); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	if n, _ := svc.UnreadCount(); n != 0 {
		t.Errorf("全部已读后未读应为 0，实际 %d", n)
	}
}
