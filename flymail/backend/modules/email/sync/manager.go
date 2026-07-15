package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	gosync "sync"
	"time"

	"flymail-core/logger"
	"flymail-core/types"
	"go.uber.org/zap"

	"flymail/modules/email/folder"
	"flymail/modules/email/message"
)

const (
	defaultPollInterval = 180 * time.Second
	minPollInterval     = 30 * time.Second
	idleRefreshInterval = 29 * time.Minute
	reconcileInterval   = 30 * time.Second

	defaultMaxConcurrent = 8   // 同时执行全量同步的 runner 数上限（sync_max_concurrent）
	defaultMaxIdleConns  = 100 // 常驻 IDLE 连接数上限（sync_max_idle_conns）
)

// errSyncSlotBusy 表示全局同步名额已满，本轮全量同步让路，稍后重试（非连接故障）。
var errSyncSlotBusy = errors.New("global sync slot busy")

// Manager 是同步调度器：为每个启用账户维护一个 AccountRunner（单连接持有者），
// 并提供全局资源闸门（同步并发信号量 + 常驻 IDLE 名额 + 轮询错峰）。
// Manager 自身实现 runnerHost，把 runner 的同步动作路由到 folders/messages 服务。
type Manager struct {
	accounts AccountLister
	folders  *folder.Service
	messages *message.Service
	pub      Publisher

	dial         func(types.IMAPConfig) (Session, error)
	pollInterval func() time.Duration
	emit         EmitFunc

	maxConcurrent func() int
	maxIdle       func() int

	mu          gosync.Mutex
	runners     map[uint]*runner
	idleAllowed map[uint]bool // 获得常驻 IDLE 名额的账户集合（reconcile 时按 id 排序重算）
	rootCtx     context.Context

	syncMu     gosync.Mutex
	syncActive int // 当前正在执行全量同步的 runner 数

	status *statusStore // 与 Service 共享；FullSync 借此上报进度（可能为 nil）
	wb     *wbStore     // 持久化回写队列（EnableWriteback 装配，可能为 nil）
}

// setStatusStore 由 Service.SetManager 调用，共享同步进度存储。
func (m *Manager) setStatusStore(s *statusStore) { m.status = s }

func NewManager(accounts AccountLister, folders *folder.Service, messages *message.Service, pub Publisher) *Manager {
	return &Manager{
		accounts:      accounts,
		folders:       folders,
		messages:      messages,
		pub:           pub,
		dial:          defaultDial,
		pollInterval:  func() time.Duration { return defaultPollInterval },
		maxConcurrent: func() int { return defaultMaxConcurrent },
		maxIdle:       func() int { return defaultMaxIdleConns },
		runners:       map[uint]*runner{},
		idleAllowed:   map[uint]bool{},
	}
}

// SetDial 测试注入。
func (m *Manager) SetDial(d func(types.IMAPConfig) (Session, error)) { m.dial = d }

// SetEmitter 注入通知回调（新邮件等事件）。
func (m *Manager) SetEmitter(fn EmitFunc) { m.emit = fn }

// SetPollIntervalProvider 注入轮询间隔（秒，<minPollInterval 取下限）。
func (m *Manager) SetPollIntervalProvider(fn func() int) {
	if fn == nil {
		return
	}
	m.pollInterval = func() time.Duration {
		d := time.Duration(fn()) * time.Second
		if d < minPollInterval {
			return minPollInterval
		}
		return d
	}
}

// SetMaxConcurrentProvider 注入全局同步并发上限（<1 取默认）。
func (m *Manager) SetMaxConcurrentProvider(fn func() int) {
	if fn == nil {
		return
	}
	m.maxConcurrent = func() int {
		if n := fn(); n >= 1 {
			return n
		}
		return defaultMaxConcurrent
	}
}

// SetMaxIdleProvider 注入常驻 IDLE 名额上限（<0 取默认）。
func (m *Manager) SetMaxIdleProvider(fn func() int) {
	if fn == nil {
		return
	}
	m.maxIdle = func() int {
		if n := fn(); n >= 0 {
			return n
		}
		return defaultMaxIdleConns
	}
}

// Start 启动调度：立即调和一次，并起 reconcile 循环。
func (m *Manager) Start(ctx context.Context) {
	m.rootCtx = ctx
	m.reconcile()
	go m.reconcileLoop(ctx)
}

// Stop 取消所有 runner 并等待退出。
func (m *Manager) Stop() {
	m.mu.Lock()
	runners := make([]*runner, 0, len(m.runners))
	for id, r := range m.runners {
		runners = append(runners, r)
		delete(m.runners, id)
	}
	m.mu.Unlock()
	for _, r := range runners {
		r.stop()
	}
}

func (m *Manager) reconcileLoop(ctx context.Context) {
	t := time.NewTicker(reconcileInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.reconcile()
		}
	}
}

// reconcile 对齐「启用账户集合 ↔ runner 集合」，并按 id 排序重算 IDLE 名额。
// 停止 runner 会等待其 goroutine 退出，故必须在释放 m.mu 之后进行（runner 可能正回调 IDLEAllowed 持锁）。
func (m *Manager) reconcile() {
	ids, err := m.accounts.ListEnabledIDs()
	if err != nil {
		return
	}
	want := make(map[uint]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}

	var toStop []*runner
	m.mu.Lock()
	m.recomputeIdleQuotaLocked(ids)
	for id := range want {
		if _, ok := m.runners[id]; !ok {
			r := m.newAccountRunner(id)
			r.start(m.rootCtx)
			m.runners[id] = r
			m.recoverWriteback(id, r) // 启动恢复该账户遗留的待回写
			logger.Info("sync-manager: runner 启动", zap.Uint("account_id", id))
		}
	}
	for id, r := range m.runners {
		if !want[id] {
			toStop = append(toStop, r)
			delete(m.runners, id)
			logger.Info("sync-manager: runner 停止", zap.Uint("account_id", id))
		}
	}
	m.mu.Unlock()

	for _, r := range toStop {
		r.stop()
	}
}

// newAccountRunner 构建一个绑定该账户配置的 runner，host 指向 Manager。
func (m *Manager) newAccountRunner(accountID uint) *runner {
	r := newRunner(accountID, func() (types.IMAPConfig, error) {
		return m.accounts.IMAPConfig(accountID)
	}, m.dial)
	r.host = m
	return r
}

// ensureRunner 返回账户 runner，不存在则即时创建并启动（Trigger/详情/附件/回写按需拉起）。
func (m *Manager) ensureRunner(accountID uint) *runner {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.runners[accountID]; ok {
		return r
	}
	r := m.newAccountRunner(accountID)
	ctx := m.rootCtx
	if ctx == nil {
		ctx = context.Background()
	}
	r.start(ctx)
	m.runners[accountID] = r
	return r
}

// ── orchestrator 实现（供 Service 投递任务）─────────────────────────────────────

// TriggerSync 前台优先执行一次全量同步，阻塞至完成或 ctx 取消。
func (m *Manager) TriggerSync(ctx context.Context, accountID uint) error {
	r := m.ensureRunner(accountID)
	return r.submitForeground(ctx, func(sess Session) error {
		return m.triggeredFullSync(ctx, accountID, sess)
	})
}

// ForegroundOp 投递前台任务并等待结果（详情/附件）。
func (m *Manager) ForegroundOp(ctx context.Context, accountID uint, run func(Session) error) error {
	r := m.ensureRunner(accountID)
	return r.submitForeground(ctx, run)
}

// BackgroundOp 非阻塞投递后台任务（回写）。
func (m *Manager) BackgroundOp(accountID uint, run func(Session) error) bool {
	r := m.ensureRunner(accountID)
	return r.submitBackground(run)
}

// triggeredFullSync 是手动触发的全量同步：抢不到全局名额则置 queued 并短延迟重试。
func (m *Manager) triggeredFullSync(ctx context.Context, accountID uint, sess Session) error {
	for {
		err := m.FullSync(accountID, sess, nil)
		if errors.Is(err, errSyncSlotBusy) {
			m.statusPhase(accountID, PhaseQueued)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(syncSlotRetry):
			}
			continue
		}
		return err
	}
}

// ── 状态上报辅助（status 为 nil 时静默）──────────────────────────────────────
func (m *Manager) statusBegin(accountID uint, p Phase) {
	if m.status != nil {
		m.status.begin(accountID, p)
	}
}

func (m *Manager) statusPhase(accountID uint, p Phase) {
	if m.status != nil {
		m.status.markPhase(accountID, p)
	}
}

func (m *Manager) statusFail(accountID uint, err error) {
	if m.status != nil {
		m.status.fail(accountID, err.Error())
	}
}

func (m *Manager) statusDone(accountID uint) {
	if m.status == nil {
		return
	}
	total, _ := m.messages.CountByAccount(accountID)
	m.status.markDone(accountID, int(total))
}

// recomputeIdleQuotaLocked 取启用账户中 id 最小的前 maxIdle 个授予常驻 IDLE 名额。调用方须持 m.mu。
func (m *Manager) recomputeIdleQuotaLocked(ids []uint) {
	limit := m.maxIdle()
	sorted := make([]uint, len(ids))
	copy(sorted, ids)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	allowed := make(map[uint]bool, limit)
	for i, id := range sorted {
		if i >= limit {
			break
		}
		allowed[id] = true
	}
	m.idleAllowed = allowed
}

// ── runnerHost 实现 ──────────────────────────────────────────────────────────

// FullSync 执行一轮全文件夹增量同步（全局并发受信号量限制；文件夹边界让位前台任务）。
func (m *Manager) FullSync(accountID uint, sess Session, yield func()) error {
	if !m.acquireSyncSlot() {
		return errSyncSlotBusy
	}
	defer m.releaseSyncSlot()

	start := time.Now()
	m.statusBegin(accountID, PhaseFolders)
	if err := m.folders.SyncFolders(accountID, sess); err != nil {
		logger.Error("sync-manager: 列文件夹失败",
			zap.Uint("account_id", accountID), zap.Error(err))
		m.statusFail(accountID, err)
		return err
	}
	fs, err := m.folders.List(accountID)
	if err != nil {
		m.statusFail(accountID, err)
		return err
	}
	m.statusPhase(accountID, PhaseMessages)
	var firstErr error
	attempted, failed := 0, 0
	for i := range fs {
		f := &fs[i]
		if !f.Selectable {
			continue
		}
		attempted++
		if err := m.syncFolder(accountID, f, sess); err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
		}
		if yield != nil {
			yield() // 文件夹边界让位前台任务（详情/附件/手动触发）
		}
	}
	_ = m.accounts.TouchLastSync(accountID, time.Now())
	// 仅当所有尝试的文件夹都失败时判定为连接故障，返回错误触发重连。
	if attempted > 0 && failed == attempted {
		m.statusFail(accountID, firstErr)
		return firstErr
	}
	m.statusDone(accountID)
	logger.Info("sync-manager: 一轮同步完成",
		zap.Uint("account_id", accountID), zap.Duration("duration", time.Since(start)))
	return nil
}

// InboxSync 只增量同步收件箱（IDLE 唤醒用）。
func (m *Manager) InboxSync(accountID uint, sess Session) error {
	return m.pollInbox(accountID, sess)
}

// SelectInbox 为进入 IDLE 选中收件箱；无收件箱返回 ok=false。
func (m *Manager) SelectInbox(accountID uint, sess Session) (bool, error) {
	inbox, err := m.folders.FindInbox(accountID)
	if err != nil || inbox == nil {
		return false, err
	}
	if _, err := sess.SelectFolder(inbox.Path); err != nil {
		return false, err
	}
	return true, nil
}

// PollInterval 返回当前轮询间隔。
func (m *Manager) PollInterval() time.Duration { return m.pollInterval() }

// IDLEAllowed 报告账户是否持有常驻 IDLE 名额。
func (m *Manager) IDLEAllowed(accountID uint) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.idleAllowed[accountID]
}

// acquireSyncSlot 尝试占用一个全局同步名额，占满返回 false（非阻塞）。
func (m *Manager) acquireSyncSlot() bool {
	m.syncMu.Lock()
	defer m.syncMu.Unlock()
	if m.syncActive >= m.maxConcurrent() {
		return false
	}
	m.syncActive++
	return true
}

func (m *Manager) releaseSyncSlot() {
	m.syncMu.Lock()
	defer m.syncMu.Unlock()
	if m.syncActive > 0 {
		m.syncActive--
	}
}

// pollInbox 只增量同步收件箱（IDLE 唤醒用）。
func (m *Manager) pollInbox(accountID uint, sess Session) error {
	inbox, err := m.folders.FindInbox(accountID)
	if err != nil || inbox == nil {
		return err
	}
	return m.syncFolder(accountID, inbox, sess)
}

func (m *Manager) syncFolder(accountID uint, f *folder.Folder, sess Session) error {
	state, newCount, err := m.messages.IncrementalSync(
		accountID, f.ID, f.Path, f.UIDValidity, f.UIDNext, f.TotalCount, sess,
	)
	if err != nil {
		logger.Error("sync-manager: 增量同步失败",
			zap.Uint("account_id", accountID), zap.String("folder", f.Path), zap.Error(err))
		return err
	}
	if err := m.folders.UpdateSyncState(f.ID, state.UIDValidity, state.UIDNext, state.Total, state.Unread, time.Now()); err != nil {
		logger.Warn("sync-manager: 回写同步状态失败",
			zap.Uint("account_id", accountID), zap.String("folder", f.Path), zap.Error(err))
	}
	// 始终输出一行同步结果，便于诊断（本地总数/未读/锚点 uidNext/本次新增）。
	logger.Info("sync-manager: 文件夹同步完成",
		zap.Uint("account_id", accountID), zap.String("folder", f.Path),
		zap.Int("local", state.Total), zap.Int("unread", state.Unread),
		zap.Uint32("uid_next", uint32(state.UIDNext)), zap.Int("new", newCount))
	if newCount > 0 {
		if m.pub != nil {
			payload, _ := json.Marshal(Event{
				Type:      "new_mail",
				AccountID: accountID,
				FolderID:  f.ID,
				NewCount:  newCount,
			})
			m.pub.Publish(payload)
		}
		// 站内/外发通知：新邮件
		if m.emit != nil {
			m.emit(string(notifyMailNew), accountID,
				"新邮件", fmt.Sprintf("收到 %d 封新邮件", newCount))
		}
	}
	return nil
}

// stopTimer 停止 timer 并清空其 channel，便于后续安全 Reset。
func stopTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}

// runnerCount 返回当前 runner 数（测试与诊断用）。
func (m *Manager) runnerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.runners)
}

// WorkerAccountIDs 返回当前有 runner 的账户 id（监控用）。
func (m *Manager) WorkerAccountIDs() []uint {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]uint, 0, len(m.runners))
	for id := range m.runners {
		ids = append(ids, id)
	}
	return ids
}

// CurrentPollSeconds 返回当前轮询间隔（秒，监控用）。
func (m *Manager) CurrentPollSeconds() int {
	return int(m.pollInterval() / time.Second)
}

// RunnerStat 是单账户 runner 的运行时快照（监控用）。
type RunnerStat struct {
	AccountID       uint `json:"account_id"`
	BreakerOpen     bool `json:"breaker_open"`
	BreakerFailures int  `json:"breaker_failures"`
	QueueDepth      int  `json:"queue_depth"` // 后台任务队列深度
}

// RunnerStats 返回各账户 runner 的熔断状态与队列深度。
func (m *Manager) RunnerStats() []RunnerStat {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RunnerStat, 0, len(m.runners))
	for id, r := range m.runners {
		st, depth := r.stats()
		out = append(out, RunnerStat{
			AccountID:       id,
			BreakerOpen:     st.Open,
			BreakerFailures: st.Failures,
			QueueDepth:      depth,
		})
	}
	return out
}

// PendingWritebackCount 返回待回写总数（监控用）。
func (m *Manager) PendingWritebackCount() int64 {
	if m.wb == nil {
		return 0
	}
	n, _ := m.wb.CountPending()
	return n
}
