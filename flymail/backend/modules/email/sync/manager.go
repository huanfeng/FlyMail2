package sync

import (
	"context"
	"encoding/json"
	"fmt"
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

	// 初始全文件夹增量同步。
	m.pollAll(accountID, sess)

	idleCh := make(chan struct{}, 1)
	if sess.CanIDLE() {
		sess.SetIDLEHandler(func(coreimap.IDLEEvent) {
			select {
			case idleCh <- struct{}{}:
			default:
			}
		})
	}

	pollTicker := time.NewTicker(m.pollInterval())
	defer pollTicker.Stop()

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
						t := time.NewTimer(idleRefreshInterval)
						defer t.Stop()
						idleRefresh = t.C
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
			if handle != nil {
				_ = handle.Stop("new-mail")
			}
			m.pollInbox(accountID, sess)
		case <-idleDone:
			return fmt.Errorf("idle connection closed")
		case <-idleRefresh:
			if handle != nil {
				_ = handle.Stop("refresh")
			}
		case <-pollTicker.C:
			if handle != nil {
				_ = handle.Stop("poll")
			}
			m.pollAll(accountID, sess)
		}
	}
}

// pollAll 重列文件夹（发现新文件夹）后对所有可选文件夹做增量同步。
func (m *Manager) pollAll(accountID uint, sess Session) {
	_ = m.folders.SyncFolders(accountID, sess)
	fs, err := m.folders.List(accountID)
	if err != nil {
		return
	}
	for i := range fs {
		f := &fs[i]
		if !f.Selectable {
			continue
		}
		m.syncFolder(accountID, f, sess)
	}
	_ = m.accounts.TouchLastSync(accountID, time.Now())
}

// pollInbox 只增量同步收件箱（IDLE 唤醒用）。
func (m *Manager) pollInbox(accountID uint, sess Session) {
	inbox, err := m.folders.FindInbox(accountID)
	if err != nil || inbox == nil {
		return
	}
	m.syncFolder(accountID, inbox, sess)
}

func (m *Manager) syncFolder(accountID uint, f *folder.Folder, sess Session) {
	state, newCount, err := m.messages.IncrementalSync(
		accountID, f.ID, f.Path, f.UIDValidity, f.UIDNext, f.TotalCount, sess,
	)
	if err != nil {
		return
	}
	_ = m.folders.UpdateSyncState(f.ID, state.UIDValidity, state.UIDNext, state.Total, state.Unread, time.Now())
	if newCount > 0 && m.pub != nil {
		payload, _ := json.Marshal(Event{
			Type:      "new_mail",
			AccountID: accountID,
			FolderID:  f.ID,
			NewCount:  newCount,
		})
		m.pub.Publish(payload)
	}
}

// workerCount 返回当前运行的 worker 数（测试与诊断用）。
func (m *Manager) workerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.workers)
}
