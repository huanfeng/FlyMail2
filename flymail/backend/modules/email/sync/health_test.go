package sync

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// captureEmit 捕获两类回调的事件，供逐用例断言。
type captureEmit struct {
	events []string // emit 捕获："type:accountID:title"
	exter  []string // emitExt 捕获，同格式
}

func (c *captureEmit) fns() (emit EmitFunc, emitExt EmitFunc) {
	return func(eventType string, accountID uint, _ uint, title, _ string) {
			c.events = append(c.events, eventType+":"+itoa(accountID)+":"+title)
		},
		func(eventType string, accountID uint, _ uint, title, _ string) {
			c.exter = append(c.exter, eventType+":"+itoa(accountID)+":"+title)
		}
}

func itoa(n uint) string { return fmt.Sprintf("%d", n) }

// mkWatcher 组装一个用例专用的 watcher。
// briefs/stats 为惰性求值：每次 runOnce 现取（模拟真实依赖的读库/读内存）。
func mkWatcher(briefs func() []AccountBrief, stats func() []RunnerStat, base time.Time) (*healthWatcher, *captureEmit, *time.Time) {
	c := &captureEmit{}
	emit, emitExt := c.fns()
	cur := base
	nowp := &cur
	w := newHealthWatcher(
		func() ([]AccountBrief, error) { return briefs(), nil },
		func() []RunnerStat { return stats() },
		emit, emitExt,
		func() time.Time { return *nowp },
	)
	return w, c, nowp
}

func TestWatcherBreakerEntryEdge(t *testing.T) {
	base := time.Unix(1700000000, 0)
	mk := func() []AccountBrief {
		return []AccountBrief{{ID: 1, Name: "Gmail", Email: "a@x.com", Status: "ok", Enabled: true}}
	}
	mkStat := func() []RunnerStat {
		return []RunnerStat{{AccountID: 1, BreakerOpen: true, BreakerFailures: 5, LastError: "dial tcp: timeout"}}
	}
	w, c, nowp := mkWatcher(mk, mkStat, base)
	w.runOnce()

	if len(c.events) != 1 {
		t.Fatalf("熔断进入沿应发 1 条落库通知，got %d", len(c.events))
	}
	if c.events[0] != "sync_failed:1:同步异常：Gmail" {
		t.Fatalf("got %q", c.events[0])
	}
	if len(c.exter) != 0 {
		t.Fatalf("进入沿不应走只外发通道，got %v", c.exter)
	}
	// 未满 24h 不重复
	*nowp = base.Add(2 * time.Hour)
	w.runOnce()
	if len(c.events) != 1 || len(c.exter) != 0 {
		t.Fatalf("24h 内不得重复推送: %v %v", c.events, c.exter)
	}

	// 满 24h：重复推送只走外发（SkipInApp 口径）
	*nowp = base.Add(25 * time.Hour)
	w.runOnce()
	if len(c.events) != 1 {
		t.Fatalf("重复推送不得落库，events=%v", c.events)
	}
	if len(c.exter) != 1 {
		t.Fatalf("重复推送应走外发，got %v", c.exter)
	}
}

func TestWatcherNeedsReauthEntrySilent(t *testing.T) {
	// needs_reauth 进入沿不推送：markNeedsReauth 已即时推送并落库（通知口径表）。
	base := time.Unix(1700000000, 0)
	mk := func() []AccountBrief {
		return []AccountBrief{{ID: 2, Name: "QQ", Email: "b@x.com", Status: "needs_reauth", Enabled: true}}
	}
	mkStat := func() []RunnerStat { return nil }
	w, c, nowp := mkWatcher(mk, mkStat, base)
	w.runOnce()

	if len(c.events) != 0 || len(c.exter) != 0 {
		t.Fatalf("needs_reauth 进入沿不得推送: events=%v exter=%v", c.events, c.exter)
	}
	// 但故障被记账：24h 后应有重复提醒
	*nowp = base.Add(25 * time.Hour)
	w.runOnce()
	if len(c.exter) != 1 {
		t.Fatalf("24h 后应外发重复提醒，got %v", c.exter)
	}
}

func TestWatcherRecoveryEdge(t *testing.T) {
	base := time.Unix(1700000000, 0)
	briefs := []AccountBrief{{ID: 1, Name: "Gmail", Email: "a@x.com", Status: "ok", Enabled: true}}
	mkBriefs := func() []AccountBrief { return briefs }
	statOpen := []RunnerStat{{AccountID: 1, BreakerOpen: true, BreakerFailures: 5}}
	statOK := []RunnerStat{{AccountID: 1, BreakerOpen: false, BreakerFailures: 0}}
	mkStat := func() []RunnerStat { return statOpen }
	w, c, nowp := mkWatcher(mkBriefs, mkStat, base)

	w.runOnce() // 进入故障
	*nowp = base.Add(1 * time.Hour)

	// 恢复：熔断复位（直接替换注入的 stats 依赖）
	w.stats = func() []RunnerStat { return statOK }
	w.runOnce()

	if len(c.events) != 2 {
		t.Fatalf("进入沿+恢复沿应共 2 条落库通知，got %v", c.events)
	}
	if c.events[1] != "account_status:1:同步已恢复：Gmail" {
		t.Fatalf("恢复沿 got %q", c.events[1])
	}
}

func TestWatcherRestartReplay(t *testing.T) {
	// 重启补推：faults 初始为空，第一个 tick 观察到存量熔断故障 → 按新翻转沿补推。
	base := time.Unix(1700000000, 0)
	mk := func() []AccountBrief {
		return []AccountBrief{{ID: 1, Name: "Gmail", Email: "a@x.com", Status: "ok", Enabled: true}}
	}
	mkStat := func() []RunnerStat {
		return []RunnerStat{{AccountID: 1, BreakerOpen: true, BreakerFailures: 7, LastError: "x"}}
	}
	w, c, _ := mkWatcher(mk, mkStat, base)
	w.runOnce()
	if len(c.events) != 1 {
		t.Fatalf("重启后首个 tick 应补推熔断故障，got %v", c.events)
	}
}

func TestWatcherDisabledRemovedSilently(t *testing.T) {
	// 故障期间账户被禁用：移出故障集合且不发恢复通知（SetEnabled 已有自己的通知）。
	base := time.Unix(1700000000, 0)
	on := []AccountBrief{{ID: 1, Name: "Gmail", Email: "a@x.com", Status: "ok", Enabled: true}}
	off := []AccountBrief{{ID: 1, Name: "Gmail", Email: "a@x.com", Status: "ok", Enabled: false}}
	mkBriefs := func() []AccountBrief { return on }
	mkStat := func() []RunnerStat {
		return []RunnerStat{{AccountID: 1, BreakerOpen: true, BreakerFailures: 5}}
	}
	w, c, nowp := mkWatcher(mkBriefs, mkStat, base)
	w.runOnce() // 记账
	*nowp = base.Add(1 * time.Hour)
	w.statuses = func() ([]AccountBrief, error) { return off, nil }
	w.stats = func() []RunnerStat { return nil }
	w.runOnce() // 被禁用 → 移出，无任何推送

	if len(c.events) != 1 || len(c.exter) != 0 {
		t.Fatalf("禁用边界不得产生恢复通知: %v %v", c.events, c.exter)
	}
}

func TestWatcherPriorityNeedsReauth(t *testing.T) {
	// needs_reauth 优先于 breaker：同一账户两者同时成立时按 needs_reauth 判定，进入沿静默。
	base := time.Unix(1700000000, 0)
	mk := func() []AccountBrief {
		return []AccountBrief{{ID: 3, Name: "G", Email: "c@x.com", Status: "needs_reauth", Enabled: true}}
	}
	mkStat := func() []RunnerStat {
		return []RunnerStat{{AccountID: 3, BreakerOpen: true, BreakerFailures: 5}}
	}
	w, c, _ := mkWatcher(mk, mkStat, base)
	w.runOnce()

	if len(c.events) != 0 || len(c.exter) != 0 {
		t.Fatalf("needs_reauth 优先 → 进入沿静默: %v %v", c.events, c.exter)
	}
}

func TestWatcherStatusReadErrorNoFalseRecovery(t *testing.T) {
	// 读账户事实失败时不动故障集合：读库抖动不得被误判成「全部恢复」。
	base := time.Unix(1700000000, 0)
	okBriefs := []AccountBrief{{ID: 1, Name: "Gmail", Email: "a@x.com", Status: "ok", Enabled: true}}
	c := &captureEmit{}
	w := newHealthWatcher(
		func() ([]AccountBrief, error) { return okBriefs, nil },
		func() []RunnerStat { return []RunnerStat{{AccountID: 1, BreakerOpen: true, BreakerFailures: 5}} },
		func(string, uint, uint, string, string) { c.events = append(c.events, "x") },
		func(string, uint, uint, string, string) {},
		func() time.Time { return base },
	)
	w.runOnce() // 记账
	w.statuses = func() ([]AccountBrief, error) { return nil, errors.New("db busy") }
	w.runOnce() // 读取失败 → 跳过本轮
	if len(c.events) != 1 {
		t.Fatalf("读取失败不得触发恢复推送: %v", c.events)
	}
}
