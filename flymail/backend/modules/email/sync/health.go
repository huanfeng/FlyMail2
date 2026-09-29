package sync

import (
	"context"
	"fmt"
	gosync "sync"
	"time"
)

// 账户健康 watcher：差分账户故障态并推送通知。
//
// 故障判据的两半都在 sync 包内——熔断器是 runner 的内存态（RunnerStat），
// needs_reauth 是账户表的状态列（AccountBrief.Status）。差分出翻转沿与 24h 重复，
// 通知经注入的 emit / emitExt 出口发出去；依赖注入维持 sync 不 import account/notify
// 的既有格局（事件类型字符串镜像、AccountBrief 由 app 层从 account.Service 组装）。
//
// 通知口径（见 spec「站内落库口径」）：
//   - 熔断进入沿：emit（落库+外发）
//   - needs_reauth 进入沿：不发（markNeedsReauth 已即时推送并落库）
//   - 24h 重复：emitExt（只外发，不落库——防通知中心刷屏）
//   - 恢复：emit（落库+外发）
//   - 重启补推：faults 集合初始为空，第一个 tick 的差分天然实现；仅熔断会补推
//     （needs_reauth 沿本来就不由 watcher 发，存量自然不重复）。
const (
	healthTickInterval = time.Minute
	healthRepeatEvery  = 24 * time.Hour

	faultNeedsReauth = "needs_reauth"
	faultBreaker     = "breaker"
)

// accountStatusNeedsReauth 镜像 account.StatusNeedsReauth（避免反向依赖）。
const accountStatusNeedsReauth = "needs_reauth"

// AccountBrief 是 watcher 所需的账户事实快照。app 层从 account.Service.List 组装注入。
type AccountBrief struct {
	ID      uint
	Name    string
	Email   string
	Status  string
	Enabled bool
}

// healthFault 记录一个正在故障中的账户。
type healthFault struct {
	kind      string    // faultNeedsReauth / faultBreaker
	since     time.Time // 本进程内首次观察到故障的时刻（「已持续 N 天」的起点；重启后从观察点重计）
	lastNotif time.Time // 上次推送（进入沿或重复沿）的时刻
}

// healthWatcher 每 tick 差分一次故障态集合，翻转沿即时推送、持续故障每 24h 重复。
type healthWatcher struct {
	statuses func() ([]AccountBrief, error)
	stats    func() []RunnerStat
	emit     EmitFunc // 落库+外发（熔断进入沿/恢复）
	emitExt  EmitFunc // 只外发（24h 重复）
	now      func() time.Time
	repeat   time.Duration // 24h，测试可注入更短值

	mu     gosync.Mutex
	faults map[uint]healthFault
}

func newHealthWatcher(
	statuses func() ([]AccountBrief, error),
	stats func() []RunnerStat,
	emit, emitExt EmitFunc,
	now func() time.Time,
) *healthWatcher {
	return &healthWatcher{
		statuses: statuses,
		stats:    stats,
		emit:     emit,
		emitExt:  emitExt,
		now:      now,
		repeat:   healthRepeatEvery,
		faults:   map[uint]healthFault{},
	}
}

// loop 以 healthTickInterval 为周期反复 runOnce（Manager.Start 起协程调用）。
func (w *healthWatcher) loop(ctx context.Context) {
	w.runOnce() // 启动即查一次：重启补推
	t := time.NewTicker(healthTickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.runOnce()
		}
	}
}

// runOnce 差分一轮。单次失败不进熔断（breaker 需要 5 次连续失败）、
// needs_reauth 是终态，因此这里没有额外防抖。
func (w *healthWatcher) runOnce() {
	briefs, err := w.statuses()
	if err != nil {
		// 读账户事实失败时保守处理：不动故障集合。读库抖动若被误判成
		// 「全部恢复」，用户会收到成串的假恢复通知。
		return
	}

	byStat := map[uint]RunnerStat{}
	for _, st := range w.stats() {
		byStat[st.AccountID] = st
	}
	now := w.now()

	w.mu.Lock()
	defer w.mu.Unlock()

	for _, b := range briefs {
		if !b.Enabled {
			// 故障期间被禁用：移出故障集合且不发恢复通知（SetEnabled 已有自己的通知）。
			delete(w.faults, b.ID)
			continue
		}
		// 故障判据：needs_reauth（DB 终态）优先于 breaker（内存态，5 次连续失败天然防抖）。
		kind := ""
		if b.Status == accountStatusNeedsReauth {
			kind = faultNeedsReauth
		} else if st, ok := byStat[b.ID]; ok && st.BreakerOpen {
			kind = faultBreaker
		}
		if kind == "" {
			// 健康。之前在故障中 → 恢复沿。
			if _, was := w.faults[b.ID]; was {
				delete(w.faults, b.ID)
				w.emit(notifyAccountStatus, b.ID, 0,
					"同步已恢复："+b.Name,
					fmt.Sprintf("账户 %s 恢复正常同步。", b.Email))
			}
			continue
		}
		if f, was := w.faults[b.ID]; was {
			// 已在故障中：只在满 repeat 后发一次只外发的重复提醒。
			if now.Sub(f.lastNotif) >= w.repeat {
				w.emitExt(notifySyncFailed, b.ID, 0,
					"同步异常持续："+b.Name,
					fmt.Sprintf("已持续 %d 天无法同步——%s",
						int(now.Sub(f.since).Hours()/24), w.reason(kind, byStat[b.ID])))
				f.lastNotif = now
				w.faults[b.ID] = f
			}
			continue
		}
		// 新故障（含重启后的存量发现）。
		f := healthFault{kind: kind, since: now, lastNotif: now}
		if kind == faultBreaker {
			// 熔断进入沿：落库+外发。
			w.emit(notifySyncFailed, b.ID, 0,
				"同步异常："+b.Name,
				fmt.Sprintf("连续 %d 次失败——%s", byStat[b.ID].BreakerFailures, w.reason(kind, byStat[b.ID])))
		}
		// needs_reauth 进入沿：不发（markNeedsReauth 已即时推送并落库）。
		w.faults[b.ID] = f
	}

	// briefs 未覆盖的存量故障（runner 被外部移除等）：保持记账不动，等下一个周期自然消化；
	// 不主动清除——误删会让重复提醒与恢复判断失真。
}

// reason 返回给用户的故障原因文本（重复提醒与进入沿共用，保证口径一致）。
func (w *healthWatcher) reason(kind string, st RunnerStat) string {
	if kind == faultNeedsReauth {
		return "授权已失效，请重新授权账户"
	}
	if st.LastError != "" {
		return st.LastError
	}
	return "连接邮箱服务器持续失败"
}
