package sync

import (
	gosync "sync"
	"time"
)

// runner 运行时模式（当前连接/活动状态）。
const (
	modeDisconnected = "disconnected" // 无连接、空闲等待任务
	modeConnecting   = "connecting"   // 正在建连
	modeIdle         = "idle"         // 持有连接并处于 IMAP IDLE 等待
	modePolling      = "polling"      // 正在全量同步（轮询）
	modeInboxSync    = "inbox_sync"   // IDLE 唤醒后同步收件箱
	modeTask         = "task"         // 正在执行前台/后台任务
	modeBackoff      = "backoff"      // 连接失败退避中
	modeBreakerOpen  = "breaker_open" // 熔断打开、后台暂停
)

// diagRingSize 每账户保留的最近事件条数（内存环形缓冲，重启丢失）。
const diagRingSize = 50

// DiagEvent 是一条诊断事件（供前端时间线展示）。
type DiagEvent struct {
	At     time.Time `json:"at"`
	Type   string    `json:"type"`
	Detail string    `json:"detail,omitempty"`
}

// RunnerDiag 是某账户 runner 的运行时诊断快照（监控 API 返回）。
type RunnerDiag struct {
	AccountID       uint        `json:"account_id"`
	Mode            string      `json:"mode"`
	ModeSince       time.Time   `json:"mode_since"`
	ModeSeconds     int         `json:"mode_seconds"` // 处于当前模式的秒数
	IdleCapable     bool        `json:"idle_capable"` // 服务器支持 IDLE
	IdleAllowed     bool        `json:"idle_allowed"` // 获得常驻 IDLE 名额
	IdleActive      bool        `json:"idle_active"`  // 当前确实在 IDLE
	Connected       bool        `json:"connected"`
	BreakerOpen     bool        `json:"breaker_open"`
	BreakerFailures int         `json:"breaker_failures"`
	QueueDepth      int         `json:"queue_depth"`
	LastSyncAt      *time.Time  `json:"last_sync_at,omitempty"`
	LastError       string      `json:"last_error,omitempty"`
	LastErrorAt     *time.Time  `json:"last_error_at,omitempty"`
	Events          []DiagEvent `json:"events"`
}

// runnerDiag 是 runner 的内存诊断记录器：单 goroutine 更新、监控 goroutine 读，全程加锁。
type runnerDiag struct {
	mu          gosync.Mutex
	now         func() time.Time
	mode        string
	modeSince   time.Time
	idleCapable bool
	idleActive  bool
	connected   bool
	lastSyncAt  time.Time
	lastErr     string
	lastErrAt   time.Time
	events      []DiagEvent
}

func newRunnerDiag(now func() time.Time) *runnerDiag {
	return &runnerDiag{
		now:       now,
		mode:      modeDisconnected,
		modeSince: now(),
		events:    make([]DiagEvent, 0, diagRingSize),
	}
}

// setMode 切换当前模式；仅在模式变化时刷新 modeSince。
func (d *runnerDiag) setMode(m string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.mode != m {
		d.mode = m
		d.modeSince = d.now()
	}
}

// setConnected 更新连接状态；断开时清 idleActive，连接时记录服务器 IDLE 能力。
func (d *runnerDiag) setConnected(connected, idleCapable bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connected = connected
	if connected {
		d.idleCapable = idleCapable
	} else {
		d.idleActive = false
	}
}

func (d *runnerDiag) setIdleActive(b bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.idleActive = b
}

func (d *runnerDiag) markSync() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastSyncAt = d.now()
}

func (d *runnerDiag) markErr(msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastErr = msg
	d.lastErrAt = d.now()
}

// event 追加一条事件，超出环形容量时丢弃最旧。
func (d *runnerDiag) event(typ, detail string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, DiagEvent{At: d.now(), Type: typ, Detail: detail})
	if len(d.events) > diagRingSize {
		d.events = d.events[len(d.events)-diagRingSize:]
	}
}

// modeOnly 轻量读取当前模式（列表概览用，不复制事件）。
func (d *runnerDiag) modeOnly() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.mode
}

// diagSnapshot 是 runner 侧诊断字段的一致快照（Manager 据此组装 RunnerDiag）。
type diagSnapshot struct {
	Mode        string
	ModeSince   time.Time
	IdleCapable bool
	IdleActive  bool
	Connected   bool
	LastSyncAt  time.Time
	LastErrAt   time.Time
	LastErr     string
	Events      []DiagEvent
}

func (d *runnerDiag) snapshot() diagSnapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	ev := make([]DiagEvent, len(d.events))
	copy(ev, d.events)
	return diagSnapshot{
		Mode: d.mode, ModeSince: d.modeSince,
		IdleCapable: d.idleCapable, IdleActive: d.idleActive, Connected: d.connected,
		LastSyncAt: d.lastSyncAt, LastErrAt: d.lastErrAt, LastErr: d.lastErr,
		Events: ev,
	}
}
