package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
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

	// notifyDedupTTL 是新邮件通知的去重窗口：同一封（组）邮件在窗口内只提醒一次。
	// Gmail 把标签映射成 IMAP 文件夹，一封新邮件会在 INBOX 与各标签文件夹里
	// 分别被识别为「新增未读」，各文件夹的同步间隔可能相差数十秒，故留足窗口。
	notifyDedupTTL = 10 * time.Minute
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
	rules        RuleRunner // 规则引擎（可能为 nil），见 syncFolder

	maxConcurrent func() int
	maxIdle       func() int

	// 正文预取配置（见 bodyprefetch.go）；未注入时取默认 new / 30 天。
	bodyMode       func() string
	bodyRecentDays func() int

	mu          gosync.Mutex
	runners     map[uint]*runner
	idleAllowed map[uint]bool // 获得常驻 IDLE 名额的账户集合（reconcile 时按 id 排序重算）
	rootCtx     context.Context

	syncMu     gosync.Mutex
	syncActive int // 当前正在执行全量同步的 runner 数

	notifyMu   gosync.Mutex
	notifySeen map[string]time.Time // 新邮件通知去重：dedupKey → 上次提醒时间

	status *statusStore // 与 Service 共享；FullSync 借此上报进度（可能为 nil）
	wb     *wbStore     // 持久化回写队列（EnableWriteback 装配，可能为 nil）
}

// setStatusStore 由 Service.SetManager 调用，共享同步进度存储，
// 并把状态变更接到 SSE 上——前端的同步进度（含**后台自动同步**）由此推送。
//
// 接在 store 的统一出口上而不是各个调用点：漏掉一处的表现是"某种同步不显示进度"，
// 那种漏几乎不可能被测试发现。
func (m *Manager) setStatusStore(s *statusStore) {
	m.status = s
	s.setOnChange(m.publishStatus)
}

// publishStatus 把一次状态变更推给所有 SSE 订阅者。
func (m *Manager) publishStatus(st Status) {
	if m.pub == nil {
		return
	}
	payload, err := json.Marshal(StatusEvent{Type: "sync_status", Status: st})
	if err != nil {
		return
	}
	// 判据：**会被后续快照取代的可丢，终态不可丢。**
	//
	// 中间帧（queued / folders / messages / 文件夹推进 / 正文回补）丢了无所谓——
	// 每条事件都携带完整快照，后到的天然覆盖先到的。而丢掉 done / error 是致命的：
	// 前端会永远停在"同步中"，因为再没有后续事件来纠正它（账户行只观察缓存，
	// 手动触发那一路也已经收手）。这正是"粘住"与"短暂倒退"的分界。
	if st.Phase == PhaseDone || st.Phase == PhaseError {
		m.pub.Publish(payload)
		return
	}
	m.pub.PublishProgress(payload)
}

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

// RuleRunner 是规则引擎对同步侧暴露的唯一入口，由 rule.Service 满足。
// 定义在 sync 而不是直接依赖 rule：rule 需要 sync.Service 的批量操作（经接口注入），
// 两边都用接口才不会成环。
type RuleRunner interface {
	// Apply 对本轮新到收件箱的邮件执行黑名单与规则；返回是否改动过任何邮件。
	Apply(accountID uint, f *folder.Folder, msgs []message.Message) (bool, error)
	// Handled 返回这批邮件里已被规则/黑名单处理过的 id，供非收件箱文件夹抑制重复提醒。
	Handled(accountID uint, msgs []message.Message) map[uint]bool
}

// ruleBatchCap 是送进规则引擎的分页大小：本轮新邮件按 id 游标分页跑完，不截断——
// 截断会让第 N+1 封起因锚点前移而永远不再进引擎。
const ruleBatchCap = 500

// runRules 把本轮新邮件分页交给规则引擎，返回是否改动过任何邮件。
func (m *Manager) runRules(accountID uint, f *folder.Folder, afterID uint) (bool, error) {
	changed := false
	for {
		rows, err := m.messages.ListAfterID(f.ID, afterID, ruleBatchCap)
		if err != nil {
			return changed, err
		}
		if len(rows) == 0 {
			return changed, nil
		}
		c, err := m.rules.Apply(accountID, f, rows)
		changed = changed || c
		if err != nil {
			return changed, err
		}
		if len(rows) < ruleBatchCap {
			return changed, nil
		}
		afterID = rows[len(rows)-1].ID
	}
}

// suppressHandled 在非收件箱文件夹里把「已被规则处理过」的邮件从本轮未读集合中剔除：
// Gmail 一封邮件在 INBOX 与各标签文件夹各一行，INBOX 那份被规则移走/标读后，
// 标签文件夹同步时同一封仍是新未读，不剔除就会照常发一条「新邮件」。
func (m *Manager) suppressHandled(accountID uint, f *folder.Folder, nm *message.NewMail) {
	if m.rules == nil || nm.UnseenTotal == 0 {
		return
	}
	rows, err := m.messages.ListAfterID(f.ID, nm.AfterID, ruleBatchCap)
	if err != nil || len(rows) == 0 {
		return
	}
	handled := m.rules.Handled(accountID, rows)
	if len(handled) == 0 {
		return
	}
	drop := 0
	for i := range rows {
		if handled[rows[i].ID] && !rows[i].Seen {
			drop++
		}
	}
	kept := nm.Unseen[:0]
	for _, u := range nm.Unseen {
		if !handled[u.ID] {
			kept = append(kept, u)
		}
	}
	nm.Unseen = kept
	if nm.UnseenTotal -= drop; nm.UnseenTotal < 0 {
		nm.UnseenTotal = 0
	}
}

// SetRuleRunner 注入规则引擎。
func (m *Manager) SetRuleRunner(r RuleRunner) { m.rules = r }

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

func (m *Manager) statusFolders(accountID uint, total int) {
	if m.status != nil {
		m.status.beginFolders(accountID, total)
	}
}

func (m *Manager) statusEnterFolder(accountID uint, f *folder.Folder) {
	if m.status != nil {
		m.status.enterFolder(accountID, folderDisplayName(f), f.Type)
	}
}

func (m *Manager) statusFinishFolder(accountID uint) {
	if m.status != nil {
		m.status.finishFolder(accountID)
	}
}

// folderDisplayName 取文件夹的展示名；缺省回落到 IMAP 路径。
// 系统文件夹的本地化在前端做（folderLabel），这里只保证非空。
func folderDisplayName(f *folder.Folder) string {
	if f.DisplayName != "" {
		return f.DisplayName
	}
	return f.Path
}

func (m *Manager) statusFail(accountID uint, err error) {
	if m.status != nil {
		m.status.fail(accountID, err.Error())
	}
}

func (m *Manager) statusDone(accountID uint) {
	if m.status != nil {
		m.status.markDone(accountID)
	}
}

func (m *Manager) statusBodies(accountID uint, total int) {
	if m.status != nil {
		m.status.beginBodies(accountID, total)
	}
}

func (m *Manager) statusBodiesDone(accountID uint, done int) {
	if m.status != nil {
		m.status.advanceBodies(accountID, done)
	}
}

func (m *Manager) statusBodiesEnd(accountID uint) {
	if m.status != nil {
		m.status.endBodies(accountID)
	}
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

// FullSync 执行一轮全文件夹增量同步，随后回补一批历史正文（若已开启）。
// 正文回补刻意放在同步名额释放之后：它是后台补齐，不该占着并发名额挡住其他账户。
//
// ⚠ statusDone 报在正文回补**之前**，别挪到后面去。
// 前端把「刷新列表/未读数」那五个 invalidate 挂在 done 上，推迟 done 就是推迟
// 「用户点了同步之后新邮件多久出现在列表里」——回补一轮十几秒到一分钟，
// 那是实打实的倒退。此刻邮件列表确实已经完整了，缺的只是正文，
// 而正文回补的进度由 Status 的 BodiesTotal / BodiesDone 另行表达。
func (m *Manager) FullSync(accountID uint, sess Session, yield func()) error {
	if err := m.fullSyncMessages(accountID, sess, yield); err != nil {
		return err
	}
	m.statusDone(accountID)
	m.prefetchHistoryBodies(accountID, sess, yield)
	return nil
}

// fullSyncMessages 是一轮全文件夹增量同步的主体（全局并发受信号量限制；
// 文件夹边界让位前台任务）。
func (m *Manager) fullSyncMessages(accountID uint, sess Session, yield func()) error {
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
	// 文件夹进度：分母在这里才知道（列完文件夹之后）。
	// 只能到文件夹这一粒度——单个文件夹内部的分批抓取没有对外的进度出口，
	// 所以首次导入时 INBOX 那一格会停留很久。够用但不够细，记在这里免得
	// 下一个人以为进度条卡住了。
	selectable := 0
	for i := range fs {
		if fs[i].Selectable {
			selectable++
		}
	}
	m.statusFolders(accountID, selectable)

	var firstErr error
	attempted, failed := 0, 0
	for i := range fs {
		f := &fs[i]
		if !f.Selectable {
			continue
		}
		attempted++
		m.statusEnterFolder(accountID, f)
		if err := m.syncFolder(accountID, f, sess); err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
		}
		m.statusFinishFolder(accountID)
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
	logger.Info("sync-manager: 一轮同步完成",
		zap.Uint("account_id", accountID), zap.Duration("duration", time.Since(start)))
	return nil
}

// InboxSync 只增量同步收件箱（IDLE 唤醒用）。进度上报的取舍见 pollInbox。
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

// pollInbox 只增量同步收件箱，**仅 IDLE 唤醒走这条**。
//
// ⚠ 刻意**不**上报同步状态，别当成漏了：这条路径只过一个文件夹的增量，
// 通常几百毫秒就结束。给它上报的话，侧栏会为每一封到达的新邮件闪一下转圈和
// 进度条——那是噪音，不是信息。这条路径本来就有它自己的反馈：新邮件出现在列表里
// （靠 new_mail 事件刷新）。
//
// ⚠ 别把这句读成「后台自动同步不报进度」：按 pollInterval 的**定时轮询根本不走
// 这条**——它走 runner 的 doPoll → host.FullSync → fullSyncMessages，那条有完整的
// 进度上报。这里只有 IDLE 唤醒那一条通路。
func (m *Manager) pollInbox(accountID uint, sess Session) error {
	inbox, err := m.folders.FindInbox(accountID)
	if err != nil || inbox == nil {
		return err
	}
	return m.syncFolder(accountID, inbox, sess)
}

func (m *Manager) syncFolder(accountID uint, f *folder.Folder, sess Session) error {
	state, nm, err := m.messages.IncrementalSync(
		accountID, f.ID, f.Path, f.UIDValidity, f.UIDNext, f.TotalCount, sess,
	)
	if err != nil {
		logger.Error("sync-manager: 增量同步失败",
			zap.Uint("account_id", accountID), zap.String("folder", f.Path), zap.Error(err))
		return err
	}
	// 新收到的邮件顺手把正文也抓下来，点开即读不必现拉。
	m.prefetchNewBodies(accountID, f, nm, sess)
	// 规则引擎：正文预取之后（正文条件才有数据）、通知之前（被移走/屏蔽的不该再提醒）。
	// 只对收件箱的非基线新邮件跑：Gmail 一封邮件在各标签文件夹各一行，逐文件夹跑会重复动作；
	// 基线导入把整个文件夹当新邮件，对历史邮件执行动作不是用户想要的。
	if m.rules != nil && !nm.Baseline && nm.Count > 0 {
		if f.Type == "inbox" {
			changed, err := m.runRules(accountID, f, nm.AfterID)
			if err != nil {
				logger.Warn("sync-manager: 规则执行失败", zap.Uint("account_id", accountID), zap.String("folder", f.Path), zap.Error(err))
			}
			if changed {
				// 移动 / 删除 / 标已读改过行，重算计数与未读集合再往下走
				if total, err := m.messages.CountByFolder(f.ID, message.Filter{}); err == nil {
					state.Total = int(total)
				}
				if unread, err := m.messages.UnreadCountByFolder(f.ID); err == nil {
					state.Unread = int(unread)
				}
				m.messages.RefreshNewMail(f.ID, nm)
			}
		} else if f.Type == "custom" {
			m.suppressHandled(accountID, f, nm)
		}
	}
	if err := m.folders.UpdateSyncState(f.ID, state.UIDValidity, state.UIDNext, state.Total, state.Unread, time.Now()); err != nil {
		logger.Warn("sync-manager: 回写同步状态失败",
			zap.Uint("account_id", accountID), zap.String("folder", f.Path), zap.Error(err))
	}
	// 始终输出一行同步结果，便于诊断（本地总数/未读/锚点 uidNext/本次新增）。
	logger.Info("sync-manager: 文件夹同步完成",
		zap.Uint("account_id", accountID), zap.String("folder", f.Path),
		zap.Int("local", state.Total), zap.Int("unread", state.Unread),
		zap.Uint32("uid_next", uint32(state.UIDNext)), zap.Int("new", nm.Count))
	if nm.Count > 0 {
		// SSE 始终发布（含基线导入）：前端据此刷新列表/未读数。NewCount 是入库行数，不扣除
		// 随后被规则移走/屏蔽的——它只是「有变化、去重新拉」的提示，前端拉回来的列表与计数已是执行后状态。
		if m.pub != nil {
			payload, _ := json.Marshal(Event{
				Type:      "new_mail",
				AccountID: accountID,
				FolderID:  f.ID,
				NewCount:  nm.Count,
			})
			m.pub.Publish(payload)
		}
		// 站内/外发通知，三重闸门：
		//  1. 文件夹类型 —— 只有收件箱与自定义文件夹（标签）值得提醒；
		//     archive 是 Gmail「所有邮件」全库镜像，junk/trash/sent/drafts 同理不提醒。
		//  2. 非基线导入的新增未读 —— 旧账户的历史邮件与已读邮件不该触发提醒。
		//  3. 跨文件夹去重 —— 同一封新邮件会在 INBOX 与各标签文件夹分别被识别为新增，
		//     不去重就会连发多条内容相同的提醒。
		// 单封未读时带上消息 ID 与发件人/主题，前端可精准跳转。
		notifiable := f.Type == "inbox" || f.Type == "custom"
		if m.emit != nil && notifiable && !nm.Baseline && nm.UnseenTotal > 0 && m.claimNotify(accountID, nm) {
			if nm.UnseenTotal == 1 && len(nm.Unseen) > 0 {
				msg := nm.Unseen[0]
				from := msg.FromName
				if from == "" {
					from = msg.FromAddr
				}
				subject := msg.Subject
				if subject == "" {
					subject = "（无主题）"
				}
				m.emit(string(notifyMailNew), accountID, msg.ID, "新邮件 · "+from, subject)
			} else {
				m.emit(string(notifyMailNew), accountID, 0,
					"新邮件", fmt.Sprintf("收到 %d 封新邮件", nm.UnseenTotal))
			}
		}
	}
	return nil
}

// claimNotify 申领一次新邮件提醒资格：同一封（组）邮件在 notifyDedupTTL 内只放行一次。
// 去重键取「账户 + 本轮未读总数 + 未读邮件的 RFC Message-ID（排序后）」——Gmail 场景下
// 同一封邮件在 INBOX 与各标签文件夹里的 Message-ID 相同，键因此一致。
// 邮件缺 Message-ID（少数不合规发信端）时无法可靠去重，一律放行，宁可重复也不漏提醒。
func (m *Manager) claimNotify(accountID uint, nm *message.NewMail) bool {
	ids := make([]string, 0, len(nm.Unseen))
	for i := range nm.Unseen {
		if id := nm.Unseen[i].MessageID; id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return true
	}
	sort.Strings(ids)
	key := fmt.Sprintf("%d|%d|%s", accountID, nm.UnseenTotal, strings.Join(ids, ","))

	now := time.Now()
	m.notifyMu.Lock()
	defer m.notifyMu.Unlock()
	if m.notifySeen == nil {
		m.notifySeen = map[string]time.Time{}
	}
	for k, at := range m.notifySeen {
		if now.Sub(at) > notifyDedupTTL {
			delete(m.notifySeen, k)
		}
	}
	if at, ok := m.notifySeen[key]; ok && now.Sub(at) <= notifyDedupTTL {
		return false
	}
	m.notifySeen[key] = now
	return true
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

// RunnerStat 是单账户 runner 的运行时快照（监控列表用）。
type RunnerStat struct {
	AccountID       uint   `json:"account_id"`
	Mode            string `json:"mode"` // 当前模式：idle/polling/disconnected/...
	BreakerOpen     bool   `json:"breaker_open"`
	BreakerFailures int    `json:"breaker_failures"`
	QueueDepth      int    `json:"queue_depth"` // 后台任务队列深度
}

// RunnerStats 返回各账户 runner 的模式、熔断状态与队列深度。
func (m *Manager) RunnerStats() []RunnerStat {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RunnerStat, 0, len(m.runners))
	for id, r := range m.runners {
		st, depth := r.stats()
		out = append(out, RunnerStat{
			AccountID:       id,
			Mode:            r.diag.modeOnly(),
			BreakerOpen:     st.Open,
			BreakerFailures: st.Failures,
			QueueDepth:      depth,
		})
	}
	return out
}

// AccountDiagnostics 返回某账户 runner 的完整运行时诊断（含事件时间线）；无 runner 返回 false。
func (m *Manager) AccountDiagnostics(accountID uint) (RunnerDiag, bool) {
	m.mu.Lock()
	r, ok := m.runners[accountID]
	allowed := m.idleAllowed[accountID]
	m.mu.Unlock()
	if !ok {
		return RunnerDiag{}, false
	}
	snap := r.diag.snapshot()
	bs, depth := r.stats()
	d := RunnerDiag{
		AccountID:       accountID,
		Mode:            snap.Mode,
		ModeSince:       snap.ModeSince,
		ModeSeconds:     int(time.Since(snap.ModeSince).Seconds()),
		IdleCapable:     snap.IdleCapable,
		IdleAllowed:     allowed,
		IdleActive:      snap.IdleActive,
		Connected:       snap.Connected,
		BreakerOpen:     bs.Open,
		BreakerFailures: bs.Failures,
		QueueDepth:      depth,
		LastError:       snap.LastErr,
		Events:          snap.Events,
	}
	if !snap.LastSyncAt.IsZero() {
		t := snap.LastSyncAt
		d.LastSyncAt = &t
	}
	if !snap.LastErrAt.IsZero() {
		t := snap.LastErrAt
		d.LastErrorAt = &t
	}
	return d, true
}

// PendingWritebackCount 返回待回写总数（监控用）。
func (m *Manager) PendingWritebackCount() int64 {
	if m.wb == nil {
		return 0
	}
	n, _ := m.wb.CountPending()
	return n
}
