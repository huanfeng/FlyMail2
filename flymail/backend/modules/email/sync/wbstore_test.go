package sync

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"flymail/internal/database"
)

// newWBTestStore 建临时 SQLite、迁移回写表并返回仓库。
func newWBTestStore(t *testing.T) (*wbStore, *gorm.DB) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "wb.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate base: %v", err)
	}
	if err := MigrateWriteback(db); err != nil {
		t.Fatalf("migrate writeback: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return newWBStore(db), db
}

// TestWBStore_EnqueueAndDuePending 入队后立即到期，可被 DuePending 捞取；Delete 后消失。
func TestWBStore_EnqueueAndDuePending(t *testing.T) {
	s, _ := newWBTestStore(t)

	op := &WritebackOp{AccountID: 7, FolderPath: "INBOX", UID: 42, Op: wbOpRead}
	if err := s.Enqueue(op); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if op.ID == 0 {
		t.Fatal("enqueue 后 ID 应被赋值")
	}

	due, err := s.DuePending(7, time.Now())
	if err != nil {
		t.Fatalf("due: %v", err)
	}
	if len(due) != 1 || due[0].UID != 42 || due[0].Op != wbOpRead {
		t.Fatalf("DuePending = %+v，期望 1 条 uid=42 read", due)
	}
	// 别的账户捞不到。
	if other, _ := s.DuePending(8, time.Now()); len(other) != 0 {
		t.Fatalf("跨账户不应捞到：%+v", other)
	}

	if err := s.Delete(op.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if due, _ := s.DuePending(7, time.Now()); len(due) != 0 {
		t.Fatalf("删除后仍捞到：%+v", due)
	}
}

// TestWBStore_FailBackoff 失败后 attempts 递增、next_attempt_at 后移，未到期不再被 DuePending 捞取。
func TestWBStore_FailBackoff(t *testing.T) {
	s, _ := newWBTestStore(t)
	op := &WritebackOp{AccountID: 1, FolderPath: "INBOX", UID: 1, Op: wbOpStar}
	if err := s.Enqueue(op); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	now := time.Now()
	attempts, err := s.Fail(op.ID, "boom", now)
	if err != nil {
		t.Fatalf("fail: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d，期望 1", attempts)
	}

	// 第 1 次失败退避 60s：此刻不到期，60s 后到期。
	if due, _ := s.DuePending(1, now.Add(30*time.Second)); len(due) != 0 {
		t.Fatalf("退避内不应到期：%+v", due)
	}
	due, _ := s.DuePending(1, now.Add(2*time.Minute))
	if len(due) != 1 {
		t.Fatalf("退避后应到期：%+v", due)
	}
	if due[0].Attempts != 1 || due[0].LastError != "boom" {
		t.Fatalf("失败元数据未持久化：%+v", due[0])
	}
}

// TestWBStore_GiveUpAtMaxAttempts 连续失败到上限后调用方据 attempts 放弃并删除。
func TestWBStore_GiveUpAtMaxAttempts(t *testing.T) {
	s, _ := newWBTestStore(t)
	op := &WritebackOp{AccountID: 1, FolderPath: "INBOX", UID: 1, Op: wbOpUnread}
	if err := s.Enqueue(op); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	now := time.Now()
	var attempts int
	for i := 0; i < maxWritebackAttempts; i++ {
		var err error
		attempts, err = s.Fail(op.ID, "still failing", now)
		if err != nil {
			t.Fatalf("fail #%d: %v", i, err)
		}
	}
	if attempts != maxWritebackAttempts {
		t.Fatalf("attempts = %d，期望 %d", attempts, maxWritebackAttempts)
	}
	// 到上限：调用方放弃并删除。
	if err := s.Delete(op.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if p, _ := s.PendingByAccount(1); len(p) != 0 {
		t.Fatalf("放弃后应无残留：%+v", p)
	}
}

// TestWBStore_PendingRecovery PendingByAccount / PendingAccountIDs / CountPending 用于启动恢复与监控。
func TestWBStore_PendingRecovery(t *testing.T) {
	s, _ := newWBTestStore(t)
	// 未来才到期的项也算 pending（启动恢复要全量拉起）。
	future := time.Now().Add(time.Hour)
	must := func(op *WritebackOp) {
		if err := s.Enqueue(op); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
	must(&WritebackOp{AccountID: 1, FolderPath: "INBOX", UID: 1, Op: wbOpRead})
	must(&WritebackOp{AccountID: 1, FolderPath: "INBOX", UID: 2, Op: wbOpStar, NextAttemptAt: future})
	must(&WritebackOp{AccountID: 2, FolderPath: "Sent", UID: 3, Op: wbOpUnstar})

	p1, _ := s.PendingByAccount(1)
	if len(p1) != 2 {
		t.Fatalf("账户 1 pending = %d，期望 2", len(p1))
	}
	ids, _ := s.PendingAccountIDs()
	if len(ids) != 2 {
		t.Fatalf("pending 账户数 = %d，期望 2", len(ids))
	}
	n, _ := s.CountPending()
	if n != 3 {
		t.Fatalf("CountPending = %d，期望 3", n)
	}
}
