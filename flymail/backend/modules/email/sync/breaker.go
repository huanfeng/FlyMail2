package sync

import (
	gosync "sync"
	"time"
)

const (
	// breakerThreshold 连续失败达到即打开熔断。
	breakerThreshold = 5
	// breakerCooldown 熔断打开后的降频窗口：其间后台轮询/自动重连暂停。
	breakerCooldown = 10 * time.Minute
)

// BreakerState 是熔断器对外的只读快照（监控用）。
type BreakerState struct {
	Open      bool      `json:"open"`
	Failures  int       `json:"failures"`
	OpenUntil time.Time `json:"open_until,omitempty"`
}

// breaker 是账户级熔断器：连续失败 ≥ breakerThreshold 打开 breakerCooldown；
// 成功即清零关闭。后台任务受 AllowBackground 门控，前台任务由 runner 直接穿透
// （不查此器），穿透成功后 RecordSuccess 清零，实现「主动操作立即恢复」。
//
// 并发安全：State 可能被监控 goroutine 读取，故全程加锁。
type breaker struct {
	mu        gosync.Mutex
	failures  int
	openUntil time.Time
	now       func() time.Time // 可注入时钟，测试用
	onChange  func(open bool)  // 打开/关闭翻转回调（诊断事件用，可为 nil）
}

func newBreaker() *breaker { return &breaker{now: time.Now} }

// RecordSuccess 记一次成功：清零失败计数并关闭熔断。
func (b *breaker) RecordSuccess() {
	b.mu.Lock()
	wasOpen := b.openLocked()
	b.failures = 0
	b.openUntil = time.Time{}
	cb := b.onChange
	b.mu.Unlock()
	if wasOpen && cb != nil {
		cb(false)
	}
}

// RecordFailure 记一次失败：累计达阈值则打开熔断（或在半开后再次失败时重新打开）。
func (b *breaker) RecordFailure() {
	b.mu.Lock()
	wasOpen := b.openLocked()
	b.failures++
	if b.failures >= breakerThreshold {
		b.openUntil = b.now().Add(breakerCooldown)
	}
	nowOpen := b.openLocked()
	cb := b.onChange
	b.mu.Unlock()
	if !wasOpen && nowOpen && cb != nil {
		cb(true)
	}
}

// AllowBackground 报告后台任务此刻是否可执行：
// 未熔断，或冷却期已过（半开，放行一次试探）返回 true；冷却窗口内返回 false。
func (b *breaker) AllowBackground() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.openLocked()
}

// openLocked 报告当前是否处于熔断打开（冷却窗口内）状态。调用方须持锁。
func (b *breaker) openLocked() bool {
	if b.openUntil.IsZero() {
		return false
	}
	return !b.now().After(b.openUntil)
}

// State 返回只读快照。
func (b *breaker) State() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := BreakerState{Failures: b.failures, Open: b.openLocked()}
	if st.Open {
		st.OpenUntil = b.openUntil
	}
	return st
}
