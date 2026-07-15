package sync

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"flymail-core/types"
)

// rnFake 复用 mgrFakeSession 满足 Session 接口，附带 Close 回调用于断言连接关闭。
type rnFake struct {
	*mgrFakeSession
	onClose func()
}

func (f *rnFake) Close() error {
	if f.onClose != nil {
		f.onClose()
	}
	return nil
}

// newDialCounter 返回一个 dial 函数与其调用计数、关闭计数。
func newDialCounter() (func(types.IMAPConfig) (Session, error), *int32, *int32) {
	var dials, closes int32
	dial := func(types.IMAPConfig) (Session, error) {
		atomic.AddInt32(&dials, 1)
		return &rnFake{mgrFakeSession: &mgrFakeSession{}, onClose: func() { atomic.AddInt32(&closes, 1) }}, nil
	}
	return dial, &dials, &closes
}

func okCfg() (types.IMAPConfig, error) { return types.IMAPConfig{Host: "h"}, nil }

// TestRunner_LazyConnectAndSerial 懒建连（无任务不 dial）+ 任务串行（任意时刻至多一个在执行）。
func TestRunner_LazyConnectAndSerial(t *testing.T) {
	dial, dials, _ := newDialCounter()
	r := newRunner(1, okCfg, dial)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	defer r.stop()

	if atomic.LoadInt32(dials) != 0 {
		t.Fatalf("未投递任务前不应 dial，实际 %d", *dials)
	}

	var active, maxActive int32
	var doneCount int32
	done := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		r.submitBackground(func(Session) error {
			cur := atomic.AddInt32(&active, 1)
			if cur > atomic.LoadInt32(&maxActive) {
				atomic.StoreInt32(&maxActive, cur)
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&active, -1)
			atomic.AddInt32(&doneCount, 1)
			done <- struct{}{}
			return nil
		})
	}
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("任务未在预期时间内完成，仅完成 %d", atomic.LoadInt32(&doneCount))
		}
	}
	if atomic.LoadInt32(&maxActive) != 1 {
		t.Fatalf("并发执行数 = %d，期望串行(1)", maxActive)
	}
	if atomic.LoadInt32(dials) != 1 {
		t.Fatalf("dial 次数 = %d，期望复用单连接(1)", *dials)
	}
}

// TestRunner_ForegroundPriority 前台任务优先于同时排队的后台任务执行。
func TestRunner_ForegroundPriority(t *testing.T) {
	dial, _, _ := newDialCounter()
	r := newRunner(1, okCfg, dial)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	defer r.stop()

	var order []string
	var recCh = make(chan string, 3)
	gate := make(chan struct{})

	// 先投一个前台"闸门"任务占住 runner，直到我们把 bg/fg 都排好队再放行。
	go func() {
		_ = r.submitForeground(context.Background(), func(Session) error {
			<-gate
			recCh <- "gate"
			return nil
		})
	}()
	// 等闸门任务真正开始执行（runner 已 busy）。
	time.Sleep(50 * time.Millisecond)

	// runner 忙时排入 bg 再排入 fg：期望放行后 fg 先于 bg。
	r.submitBackground(func(Session) error { recCh <- "bg"; return nil })
	go func() {
		_ = r.submitForeground(context.Background(), func(Session) error { recCh <- "fg"; return nil })
	}()
	time.Sleep(50 * time.Millisecond) // 确保 fg 已入队

	close(gate)
	for i := 0; i < 3; i++ {
		select {
		case s := <-recCh:
			order = append(order, s)
		case <-time.After(2 * time.Second):
			t.Fatalf("超时，已收到 %v", order)
		}
	}
	if len(order) != 3 || order[0] != "gate" || order[1] != "fg" || order[2] != "bg" {
		t.Fatalf("执行顺序 = %v，期望 [gate fg bg]", order)
	}
}

// TestRunner_IdleClose 空闲超过 idleClose 后关闭连接，下一个任务重新建连。
func TestRunner_IdleClose(t *testing.T) {
	dial, dials, closes := newDialCounter()
	r := newRunner(1, okCfg, dial)
	r.idleClose = 40 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	defer r.stop()

	done := make(chan struct{}, 1)
	r.submitBackground(func(Session) error { done <- struct{}{}; return nil })
	<-done
	// 等待超过空闲窗口，连接应被关闭。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(closes) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(closes) != 1 {
		t.Fatalf("空闲后关闭次数 = %d，期望 1", *closes)
	}

	// 再投任务应重新建连。
	r.submitBackground(func(Session) error { done <- struct{}{}; return nil })
	<-done
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(dials) < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(dials) != 2 {
		t.Fatalf("重建连接后 dial 次数 = %d，期望 2", *dials)
	}
}

// TestRunner_DialBackoff 连接失败后进入退避窗口，其间后台任务被跳过不再 dial；
// 时钟推进过退避窗口后恢复 dial。
func TestRunner_DialBackoff(t *testing.T) {
	var dials int32
	dialErr := errors.New("dial failed")
	fail := func(types.IMAPConfig) (Session, error) {
		atomic.AddInt32(&dials, 1)
		return nil, dialErr
	}
	r := newRunner(1, okCfg, fail)
	base := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	clock := base
	r.now = func() time.Time { return clock }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	defer r.stop()

	// 第一个后台任务触发 dial 失败，设置退避。
	if err := r.submitForeground(context.Background(), func(Session) error { return nil }); !errors.Is(err, dialErr) {
		t.Fatalf("首个连接应失败返回 dialErr，实际 %v", err)
	}
	if atomic.LoadInt32(&dials) != 1 {
		t.Fatalf("dial 次数 = %d，期望 1", dials)
	}

	// 退避窗口内的后台任务应被跳过（不 dial）。
	reply := make(chan error, 1)
	r.bg <- task{run: func(Session) error { return nil }, reply: reply}
	select {
	case err := <-reply:
		if !errors.Is(err, errDialBackoff) {
			t.Fatalf("退避窗口内应返回 errDialBackoff，实际 %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("后台任务未在退避内快速返回")
	}
	if atomic.LoadInt32(&dials) != 1 {
		t.Fatalf("退避内不应再 dial，dial 次数 = %d", dials)
	}
}
