package sync

import (
	"errors"
	gosync "sync"
	"time"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	imapv2 "github.com/emersion/go-imap/v2"

	"flymail/modules/email/folder"
	"flymail/modules/email/message"
)

// Session 是同步一个账户所需的 IMAP 能力集合（一个会话串行复用）。*coreimap.Session 满足此接口。
type Session interface {
	folder.IMAPLister
	message.IMAPFetcher
	FetchByUIDs(uids []imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error)
	MarkRead(uids ...imapv2.UID) error
	MarkUnread(uids ...imapv2.UID) error
	MarkStarred(uids ...imapv2.UID) error
	MarkUnstarred(uids ...imapv2.UID) error
	Close() error
}

// AccountConfigProvider 由 account.Service 实现。
type AccountConfigProvider interface {
	IMAPConfig(id uint) (types.IMAPConfig, error)
	TouchLastSync(id uint, t time.Time) error
}

// Phase 表示同步所处阶段。
type Phase string

const (
	PhaseFolders  Phase = "folders"
	PhaseMessages Phase = "messages"
	PhaseDone     Phase = "done"
	PhaseError    Phase = "error"
)

// Status 是某账户当前同步进度的快照（内存存储，重启丢失）。
type Status struct {
	AccountID uint      `json:"account_id"`
	Phase     Phase     `json:"phase"`
	Total     int       `json:"total"`
	Processed int       `json:"processed"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ErrSyncRunning 表示该账户已有同步在运行。
var ErrSyncRunning = errors.New("sync already running for this account")

// Service 编排单账户的首次同步（文件夹 → 收件箱消息），并通过内存 map 对外暴露进度。
type Service struct {
	accounts AccountConfigProvider
	folders  *folder.Service
	messages *message.Service

	dial        func(types.IMAPConfig) (Session, error)
	syncDepthFn func() int

	mu       gosync.Mutex
	statuses map[uint]*Status
	running  map[uint]bool

	wbCh chan wbOp
}

// NewService 创建 Sync 服务。
func NewService(accounts AccountConfigProvider, folders *folder.Service, messages *message.Service) *Service {
	s := &Service{
		accounts:    accounts,
		folders:     folders,
		messages:    messages,
		dial:        defaultDial,
		syncDepthFn: func() int { return 0 },
		statuses:    map[uint]*Status{},
		running:     map[uint]bool{},
		wbCh:        make(chan wbOp, 256),
	}
	go s.writebackLoop()
	return s
}

// SetSyncDepthProvider 注入同步深度提供函数；fn 返回 0 时使用 message 默认值。
func (s *Service) SetSyncDepthProvider(fn func() int) {
	if fn != nil {
		s.syncDepthFn = fn
	}
}

func defaultDial(cfg types.IMAPConfig) (Session, error) { return coreimap.Dial(cfg) }

// SetDial 覆盖拨号函数（测试注入用）。
func (s *Service) SetDial(d func(types.IMAPConfig) (Session, error)) { s.dial = d }

// Trigger 启动一次后台首同步；同账户已在运行则返回 ErrSyncRunning。
func (s *Service) Trigger(accountID uint) error {
	s.mu.Lock()
	if s.running[accountID] {
		s.mu.Unlock()
		return ErrSyncRunning
	}
	s.running[accountID] = true
	now := time.Now()
	s.statuses[accountID] = &Status{
		AccountID: accountID,
		Phase:     PhaseFolders,
		StartedAt: now,
		UpdatedAt: now,
	}
	s.mu.Unlock()

	go s.run(accountID)
	return nil
}

// StatusOf 返回指定账户的同步状态快照。
func (s *Service) StatusOf(accountID uint) (Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.statuses[accountID]
	if !ok {
		return Status{}, false
	}
	return *st, true
}

func (s *Service) setStatus(accountID uint, fn func(*Status)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.statuses[accountID]; st != nil {
		fn(st)
		st.UpdatedAt = time.Now()
	}
}

func (s *Service) run(accountID uint) {
	defer func() {
		s.mu.Lock()
		s.running[accountID] = false
		s.mu.Unlock()
	}()

	// 1. 获取 IMAP 配置
	cfg, err := s.accounts.IMAPConfig(accountID)
	if err != nil {
		s.fail(accountID, err)
		return
	}

	// 2. 建立 IMAP 连接
	sess, err := s.dial(cfg)
	if err != nil {
		s.fail(accountID, err)
		return
	}
	defer sess.Close()

	// 2.5 注入可配置同步深度（0 表示不覆盖，保持 message.Service 默认值）
	if d := s.syncDepthFn(); d > 0 {
		s.messages.SetSyncDepth(d)
	}

	// 3. 同步文件夹列表
	if err := s.folders.SyncFolders(accountID, sess); err != nil {
		s.fail(accountID, err)
		return
	}

	// 4. 同步收件箱消息
	s.setStatus(accountID, func(st *Status) { st.Phase = PhaseMessages })

	inbox, err := s.folders.FindInbox(accountID)
	if err != nil {
		s.fail(accountID, err)
		return
	}
	if inbox != nil {
		state, _, err := s.messages.SyncFolderMessages(accountID, inbox.ID, inbox.Path, inbox.UIDValidity, sess)
		if err != nil {
			s.fail(accountID, err)
			return
		}
		if err := s.folders.UpdateSyncState(inbox.ID, state.UIDValidity, state.UIDNext, state.Total, state.Unread, time.Now()); err != nil {
			s.fail(accountID, err)
			return
		}
		s.setStatus(accountID, func(st *Status) {
			st.Total = state.Total
			st.Processed = state.Total
		})
	}

	// 5. 更新账户最后同步时间
	_ = s.accounts.TouchLastSync(accountID, time.Now())

	s.setStatus(accountID, func(st *Status) { st.Phase = PhaseDone })
}

func (s *Service) fail(accountID uint, err error) {
	s.setStatus(accountID, func(st *Status) {
		st.Phase = PhaseError
		st.Error = err.Error()
	})
}

// MessageDetail 返回邮件详情；若正文尚未同步则先从 IMAP 按需抓取后落库。
func (s *Service) MessageDetail(messageID uint) (*message.MessageDetail, error) {
	m, err := s.messages.GetByID(messageID)
	if err != nil {
		return nil, err
	}
	if !m.BodySynced {
		f, err := s.folders.GetByID(m.FolderID)
		if err != nil {
			return nil, err
		}
		cfg, err := s.accounts.IMAPConfig(m.AccountID)
		if err != nil {
			return nil, err
		}
		sess, err := s.dial(cfg)
		if err != nil {
			return nil, err
		}
		defer sess.Close()
		if _, err := sess.SelectFolder(f.Path); err != nil {
			return nil, err
		}
		emails, err := sess.FetchByUIDs(
			[]imapv2.UID{imapv2.UID(m.UID)},
			coreimap.FetchOptions{FetchBody: true, FallbackHeaders: true},
		)
		if err != nil {
			return nil, err
		}
		if len(emails) > 0 {
			if err := s.messages.StoreParsedBody(messageID, emails[0]); err != nil {
				return nil, err
			}
		}
	}
	return s.messages.Detail(messageID)
}
