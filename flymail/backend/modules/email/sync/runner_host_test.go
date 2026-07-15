package sync

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	gosync "sync"
)

// fakeHost 是 runner 的同步宿主桩：记录 FullSync/InboxSync 调用，可注入 FullSync 行为。
type fakeHost struct {
	mu            gosync.Mutex
	fullSyncCalls int
	inboxCalls    int
	selectCalls   int
	pollInterval  time.Duration
	canSelect     bool
	fullSyncFn    func(yield func()) error
}

func (h *fakeHost) FullSync(_ uint, _ Session, yield func()) error {
	h.mu.Lock()
	h.fullSyncCalls++
	fn := h.fullSyncFn
	h.mu.Unlock()
	if fn != nil {
		return fn(yield)
	}
	return nil
}

func (h *fakeHost) InboxSync(_ uint, _ Session) error {
	h.mu.Lock()
	h.inboxCalls++
	h.mu.Unlock()
	return nil
}

func (h *fakeHost) SelectInbox(_ uint, _ Session) (bool, error) {
	h.mu.Lock()
	h.selectCalls++
	ok := h.canSelect
	h.mu.Unlock()
	return ok, nil
}

func (h *fakeHost) PollInterval() time.Duration { return h.pollInterval }

func (h *fakeHost) IDLEAllowed(uint) bool { return true }

func (h *fakeHost) fullSync() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.fullSyncCalls
}

func (h *fakeHost) inbox() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.inboxCalls
}

// TestRunner_PollTriggersFullSync host 模式下轮询 tick 触发全量同步。
func TestRunner_PollTriggersFullSync(t *testing.T) {
	dial, dials, _ := newDialCounter()
	r := newRunner(3, okCfg, dial)
	r.host = &fakeHost{pollInterval: 30 * time.Millisecond, canSelect: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	defer r.stop()

	h := r.host.(*fakeHost)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && h.fullSync() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if h.fullSync() == 0 {
		t.Fatal("轮询未触发 FullSync")
	}
	if atomic.LoadInt32(dials) == 0 {
		t.Fatal("轮询应建连")
	}
}

// TestRunner_IdleWakeSyncsInbox 新邮件事件（idleCh）在已连接时触发 InboxSync。
func TestRunner_IdleWakeSyncsInbox(t *testing.T) {
	dial, _, _ := newDialCounter()
	r := newRunner(1, okCfg, dial)
	r.host = &fakeHost{pollInterval: time.Hour, canSelect: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	defer r.stop()

	// 先用前台任务建连（IDLE 只在已连接时发生）。
	if err := r.submitForeground(context.Background(), func(Session) error { return nil }); err != nil {
		t.Fatalf("建连任务失败: %v", err)
	}
	// 模拟 IDLE 新邮件事件。
	r.idleCh <- struct{}{}

	h := r.host.(*fakeHost)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && h.inbox() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if h.inbox() == 0 {
		t.Fatal("新邮件事件未触发 InboxSync")
	}
}

// TestRunner_FullSyncYieldsToForeground 全量同步在文件夹边界 yield 时排干前台任务，
// 前台任务在 FullSync 返回前执行（协作让位）。
func TestRunner_FullSyncYieldsToForeground(t *testing.T) {
	dial, _, _ := newDialCounter()
	r := newRunner(1, okCfg, dial)

	started := make(chan struct{})
	release := make(chan struct{})
	rec := make(chan string, 4)
	var once int32
	h := &fakeHost{
		pollInterval: 30 * time.Millisecond,
		canSelect:    true,
		fullSyncFn: func(yield func()) error {
			if atomic.AddInt32(&once, 1) == 1 {
				close(started)
				<-release
				yield() // 应排干已排队的前台任务
				rec <- "sync-after-yield"
			}
			return nil
		},
	}
	r.host = h
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	defer r.stop()

	<-started // 首轮 FullSync 已进入并阻塞
	// 在 FullSync 阻塞期间排入前台任务（runner 未在 select 前台，发送挂起，等 yield 接收）。
	go func() {
		_ = r.submitForeground(context.Background(), func(Session) error { rec <- "fg"; return nil })
	}()
	time.Sleep(50 * time.Millisecond) // 确保前台发送已挂起
	close(release)

	var order []string
	for i := 0; i < 2; i++ {
		select {
		case s := <-rec:
			order = append(order, s)
		case <-time.After(2 * time.Second):
			t.Fatalf("超时，已收到 %v", order)
		}
	}
	if len(order) != 2 || order[0] != "fg" || order[1] != "sync-after-yield" {
		t.Fatalf("顺序 = %v，期望 [fg sync-after-yield]（前台在同步返回前被让位执行）", order)
	}
}

// TestRunner_PollPhase 错峰相位落在 [0, pollInterval) 且对同一账户稳定。
func TestRunner_PollPhase(t *testing.T) {
	dial, _, _ := newDialCounter()
	r := newRunner(5, okCfg, dial)
	r.host = &fakeHost{pollInterval: 180 * time.Second}
	p1 := r.pollPhase()
	p2 := r.pollPhase()
	if p1 != p2 {
		t.Fatalf("相位不稳定: %v vs %v", p1, p2)
	}
	if p1 < 0 || p1 >= 180*time.Second {
		t.Fatalf("相位 %v 越界 [0,180s)", p1)
	}
}
