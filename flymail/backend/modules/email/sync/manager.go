package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	gosync "sync"
	"time"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	"flymail/modules/email/folder"
	"flymail/modules/email/message"
)

const (
	defaultPollInterval = 180 * time.Second
	minPollInterval     = 30 * time.Second
	idleRefreshInterval = 29 * time.Minute
	reconcileInterval   = 30 * time.Second
	maxReconnectBackoff = 60 * time.Second
)

// Manager 后台调度：每启用账户一个 worker（单连接串行驱动），IDLE 优先 + 轮询兜底。
type Manager struct {
	accounts AccountLister
	folders  *folder.Service
	messages *message.Service
	pub      Publisher

	dial         func(types.IMAPConfig) (Session, error)
	pollInterval func() time.Duration
	emit         EmitFunc

	mu      gosync.Mutex
	workers map[uint]context.CancelFunc
	wg      gosync.WaitGroup
	rootCtx context.Context
}

func NewManager(accounts AccountLister, folders *folder.Service, messages *message.Service, pub Publisher) *Manager {
	return &Manager{
		accounts:     accounts,
		folders:      folders,
		messages:     messages,
		pub:          pub,
		dial:         defaultDial,
		pollInterval: func() time.Duration { return defaultPollInterval },
		workers:      map[uint]context.CancelFunc{},
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

// Start 启动调度：立即调和一次，并起 reconcile 循环。
func (m *Manager) Start(ctx context.Context) {
	m.rootCtx = ctx
	m.reconcile()
	go m.reconcileLoop(ctx)
}

// Stop 取消所有 worker 并等待退出。
func (m *Manager) Stop() {
	m.mu.Lock()
	for id, cancel := range m.workers {
		cancel()
		delete(m.workers, id)
	}
	m.mu.Unlock()
	m.wg.Wait()
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

func (m *Manager) reconcile() {
	ids, err := m.accounts.ListEnabledIDs()
	if err != nil {
		return
	}
	want := make(map[uint]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range want {
		if _, ok := m.workers[id]; !ok {
			wctx, cancel := context.WithCancel(m.rootCtx)
			m.workers[id] = cancel
			m.wg.Add(1)
			go m.worker(wctx, id)
		}
	}
	for id, cancel := range m.workers {
		if !want[id] {
			cancel()
			delete(m.workers, id)
		}
	}
}

func (m *Manager) worker(ctx context.Context, accountID uint) {
	defer m.wg.Done()
	log.Printf("sync-manager: 账户 %d worker 启动", accountID)
	defer log.Printf("sync-manager: 账户 %d worker 退出", accountID)
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := m.runSession(ctx, accountID)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("sync-manager: 账户 %d 会话结束(%v)，%v 后重连", accountID, err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxReconnectBackoff {
				backoff = maxReconnectBackoff
			}
			continue
		}
		backoff = time.Second
	}
}

// runSession 建立一条连接并驱动「轮询 ↔ IDLE」循环，直到出错或 ctx 取消。
func (m *Manager) runSession(ctx context.Context, accountID uint) error {
	cfg, err := m.accounts.IMAPConfig(accountID)
	if err != nil {
		return err
	}
	sess, err := m.dial(cfg)
	if err != nil {
		return err
	}
	defer sess.Close()

	log.Printf("sync-manager: 账户 %d 已连接 (IDLE 支持=%v)", accountID, sess.CanIDLE())

	// 初始全文件夹增量同步。
	if err := m.pollAll(accountID, sess); err != nil {
		return err
	}

	// 非 IDLE 服务商（如网易 163）：不持有空闲连接——这类服务器会在数分钟空闲后
	// 静默断开连接，导致后续 FETCH 卡死或失败而无法察觉。改为「轮询一轮 → 等待间隔 →
	// 关闭返回」，由 worker 重新建连进行下一轮，每轮都是新鲜连接，最稳健。
	if !sess.CanIDLE() {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(m.pollInterval()):
			return nil
		}
	}

	// 以下为 IDLE 路径：持有连接，IDLE（仅 INBOX）↔ 轮询（所有文件夹）循环。
	idleCh := make(chan struct{}, 1)
	sess.SetIDLEHandler(func(coreimap.IDLEEvent) {
		select {
		case idleCh <- struct{}{}:
		default:
		}
	})

	pollTicker := time.NewTicker(m.pollInterval())
	defer pollTicker.Stop()

	// idleRefresh timer 在循环外创建一次、每次进入 IDLE 时 Reset，
	// 避免在长生命周期 for 循环内反复 NewTimer/defer 累积（资源泄漏）。
	refreshTimer := time.NewTimer(idleRefreshInterval)
	defer refreshTimer.Stop()
	stopTimer(refreshTimer) // 起始停掉，仅在成功进入 IDLE 后 Reset

	for {
		var handle *coreimap.IdleHandle
		var idleDone <-chan error
		var idleRefresh <-chan time.Time
		if sess.CanIDLE() {
			if inbox, _ := m.folders.FindInbox(accountID); inbox != nil {
				if _, err := sess.SelectFolder(inbox.Path); err == nil {
					if h, err := sess.StartIDLE(); err == nil {
						handle = h
						idleDone = h.Done()
						refreshTimer.Reset(idleRefreshInterval)
						idleRefresh = refreshTimer.C
					}
				}
			}
		}

		select {
		case <-ctx.Done():
			if handle != nil {
				_ = handle.Stop("shutdown")
			}
			return nil
		case <-idleCh:
			stopTimer(refreshTimer)
			// Stop 失败说明 IDLE 未干净结束，连接状态不确定：返回触发重连，
			// 不要在脏连接上继续 FETCH。
			if handle != nil {
				if err := handle.Stop("new-mail"); err != nil {
					return fmt.Errorf("stop idle (new-mail): %w", err)
				}
			}
			if err := m.pollInbox(accountID, sess); err != nil {
				return err
			}
		case <-idleDone:
			stopTimer(refreshTimer)
			return fmt.Errorf("idle connection closed")
		case <-idleRefresh:
			if handle != nil {
				_ = handle.Stop("refresh")
			}
			// timer 已触发并被 select 读空，下一轮进入 IDLE 时再 Reset。
		case <-pollTicker.C:
			stopTimer(refreshTimer)
			if handle != nil {
				if err := handle.Stop("poll"); err != nil {
					return fmt.Errorf("stop idle (poll): %w", err)
				}
			}
			// 轮询出错（多为连接被服务器断开）返回触发重连，否则会在死连接上空转。
			if err := m.pollAll(accountID, sess); err != nil {
				return err
			}
			// 重新读取设置中的轮询间隔，使其变更能及时生效。
			pollTicker.Reset(m.pollInterval())
		}
	}
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

// pollAll 重列文件夹（发现新文件夹）后对所有可选文件夹做增量同步。
// 返回遇到的第一个文件夹同步错误（多为连接断开）；会先尝试完所有文件夹，
// 以保证 INBOX 等仍能在本轮被同步，再由调用方据错误决定是否重连。
func (m *Manager) pollAll(accountID uint, sess Session) error {
	if err := m.folders.SyncFolders(accountID, sess); err != nil {
		log.Printf("sync-manager: 账户 %d 列文件夹失败: %v", accountID, err)
		return err
	}
	fs, err := m.folders.List(accountID)
	if err != nil {
		return err
	}
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
	}
	_ = m.accounts.TouchLastSync(accountID, time.Now())
	// 仅当所有尝试的文件夹都失败时判定为连接故障，返回错误触发重连；
	// 个别文件夹失败（良性，如某特殊文件夹不可 SELECT）不强制重连。
	if attempted > 0 && failed == attempted {
		return firstErr
	}
	return nil
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
		log.Printf("sync-manager: 账户 %d 文件夹 %q 增量同步失败: %v", accountID, f.Path, err)
		return err
	}
	if err := m.folders.UpdateSyncState(f.ID, state.UIDValidity, state.UIDNext, state.Total, state.Unread, time.Now()); err != nil {
		log.Printf("sync-manager: 账户 %d 文件夹 %q 回写状态失败: %v", accountID, f.Path, err)
	}
	// 始终输出一行同步结果，便于诊断（本地总数/未读/锚点 uidNext/本次新增）。
	log.Printf("sync-manager: 账户 %d 文件夹 %q 同步完成 本地=%d 未读=%d uidNext=%d 新增=%d",
		accountID, f.Path, state.Total, state.Unread, state.UIDNext, newCount)
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

// workerCount 返回当前运行的 worker 数（测试与诊断用）。
func (m *Manager) workerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.workers)
}
