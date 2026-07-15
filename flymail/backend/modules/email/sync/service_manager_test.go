package sync_test

import (
	"context"
	"path/filepath"
	gosync "sync"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	syncmod "flymail/modules/email/sync"

	"flymail-core/types"

	imapv2 "github.com/emersion/go-imap/v2"
)

// fakeLister 实现 syncmod.AccountLister（Manager 调度所需）。
type fakeLister struct {
	mu  gosync.Mutex
	ids []uint
}

func (l *fakeLister) ListEnabledIDs() ([]uint, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]uint, len(l.ids))
	copy(out, l.ids)
	return out, nil
}
func (l *fakeLister) IMAPConfig(uint) (types.IMAPConfig, error) {
	return types.IMAPConfig{Host: "h"}, nil
}
func (l *fakeLister) TouchLastSync(uint, time.Time) error { return nil }

// nopPublisher 丢弃事件。
type nopPublisher struct{}

func (nopPublisher) Publish([]byte) {}

// TestWritebackPersistsAndExecutes 回写持久队列：SetRead 落库 writeback_ops，runner 执行后回写 IMAP 并删行。
func TestWritebackPersistsAndExecutes(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := syncmod.MigrateWriteback(db); err != nil {
		t.Fatalf("migrate wb: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	fsvc := folder.NewService(folder.NewRepository(db))
	msvc := message.NewService(message.NewRepository(db), message.NewBodyRepository(db))
	sess := &fakeSession{}
	dial := func(types.IMAPConfig) (syncmod.Session, error) { return sess, nil }

	svc := syncmod.NewService(&fakeAccounts{enabled: true}, fsvc, msvc)
	svc.SetDial(dial)
	mgr := syncmod.NewManager(&fakeLister{ids: []uint{1}}, fsvc, msvc, nopPublisher{})
	mgr.SetDial(dial)
	mgr.SetPollIntervalProvider(func() int { return 3600 })
	mgr.EnableWriteback(db)
	svc.SetManager(mgr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Stop()

	// 先同步出邮件（uid=1,2 入库）。
	if err := svc.Trigger(1); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := svc.StatusOf(1); ok && st.Phase == syncmod.PhaseDone {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 标已读 → 落库 + runner 执行回写。
	if err := svc.SetRead(1, true); err != nil {
		t.Fatalf("SetRead: %v", err)
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var n int64
		db.Model(&syncmod.WritebackOp{}).Count(&n)
		if sess.hasMarkRead(imapv2.UID(1)) && n == 0 {
			return // 回写已执行且行已删除
		}
		time.Sleep(20 * time.Millisecond)
	}
	var n int64
	db.Model(&syncmod.WritebackOp{}).Count(&n)
	t.Fatalf("回写未在预期内完成: markRead=%v 剩余行=%d", sess.hasMarkRead(imapv2.UID(1)), n)
}

// TestTriggerViaManagerReachesDone 手动触发经 Manager 投递到 runner 执行全量同步，状态到 done。
func TestTriggerViaManagerReachesDone(t *testing.T) {
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
	fsvc := folder.NewService(folder.NewRepository(db))
	msvc := message.NewService(message.NewRepository(db), message.NewBodyRepository(db))
	sess := &fakeSession{}
	dial := func(types.IMAPConfig) (syncmod.Session, error) { return sess, nil }

	svc := syncmod.NewService(&fakeAccounts{enabled: true}, fsvc, msvc)
	svc.SetDial(dial)

	mgr := syncmod.NewManager(&fakeLister{ids: []uint{1}}, fsvc, msvc, nopPublisher{})
	mgr.SetDial(dial)
	// 大轮询间隔，避免后台轮询与手动触发交叠干扰断言。
	mgr.SetPollIntervalProvider(func() int { return 3600 })
	svc.SetManager(mgr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Stop()

	if err := svc.Trigger(1); err != nil {
		t.Fatalf("trigger: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := svc.StatusOf(1); ok && (st.Phase == syncmod.PhaseDone || st.Phase == syncmod.PhaseError) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	st, ok := svc.StatusOf(1)
	if !ok || st.Phase != syncmod.PhaseDone {
		t.Fatalf("同步未完成: %+v", st)
	}
	// FullSync 覆盖全部文件夹（fakeSession 返回 INBOX + Sent）。
	folders, _ := fsvc.List(1)
	if len(folders) != 2 {
		t.Fatalf("文件夹数 = %d，期望 2", len(folders))
	}
}
