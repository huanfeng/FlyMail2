package sync

import (
	"context"
	"testing"
	"time"
)

// TestDiagRingBufferCap 事件环形缓冲不超过 diagRingSize，保留最新。
func TestDiagRingBufferCap(t *testing.T) {
	d := newRunnerDiag(time.Now)
	for i := 0; i < diagRingSize+20; i++ {
		d.event("e", "")
	}
	ev := d.snapshot().Events
	if len(ev) != diagRingSize {
		t.Fatalf("ring 长度 = %d，期望 %d", len(ev), diagRingSize)
	}
}

// TestDiagSetModeTimestamp setMode 仅在模式变化时刷新 modeSince。
func TestDiagSetModeTimestamp(t *testing.T) {
	base := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	clock := base
	d := newRunnerDiag(func() time.Time { return clock })
	d.setMode(modePolling)
	first := d.snapshot().ModeSince
	clock = clock.Add(time.Minute)
	d.setMode(modePolling) // 相同模式不刷新
	if d.snapshot().ModeSince != first {
		t.Fatal("相同模式不应刷新 modeSince")
	}
	d.setMode(modeIdle) // 变化则刷新
	if !d.snapshot().ModeSince.After(first) {
		t.Fatal("模式变化应刷新 modeSince")
	}
}

// TestBreaker_OnChangeFires 熔断打开/关闭翻转触发回调各一次（不重复）。
func TestBreaker_OnChangeFires(t *testing.T) {
	b, _ := newTestBreaker()
	var opens, resets int
	b.onChange = func(open bool) {
		if open {
			opens++
		} else {
			resets++
		}
	}
	for i := 0; i < breakerThreshold; i++ {
		b.RecordFailure()
	}
	if opens != 1 {
		t.Fatalf("opens = %d，期望 1", opens)
	}
	b.RecordSuccess()
	if resets != 1 {
		t.Fatalf("resets = %d，期望 1", resets)
	}
	// 未打开状态下再成功，不应触发 reset。
	b.RecordSuccess()
	if resets != 1 {
		t.Fatalf("resets = %d，期望仍为 1", resets)
	}
}

func hasEvent(ev []DiagEvent, typ string) bool {
	for _, e := range ev {
		if e.Type == typ {
			return true
		}
	}
	return false
}

// TestRunnerDiag_PollRecords host 模式一轮轮询后记录 dial_ok/poll_done 且模式为 polling。
func TestRunnerDiag_PollRecords(t *testing.T) {
	dial, _, _ := newDialCounter()
	r := newRunner(3, okCfg, dial)
	r.host = &fakeHost{pollInterval: 30 * time.Millisecond, canSelect: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.start(ctx)
	defer r.stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := r.diag.snapshot()
		if hasEvent(snap.Events, "dial_ok") && hasEvent(snap.Events, "poll_done") {
			if snap.Mode != modePolling {
				t.Fatalf("mode = %q，期望 %q", snap.Mode, modePolling)
			}
			if !snap.Connected {
				t.Fatal("应为已连接")
			}
			if snap.LastSyncAt.IsZero() {
				t.Fatal("应记录 last_sync_at")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("未在预期内记录 dial_ok/poll_done：%+v", r.diag.snapshot())
}
