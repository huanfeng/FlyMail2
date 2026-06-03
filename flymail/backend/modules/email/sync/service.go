package sync

import (
	"bytes"
	"errors"
	"fmt"
	gosync "sync"
	"time"

	coreimap "flymail-core/imap"
	coreparser "flymail-core/parser"
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
	// 删除/移动（P0 日常动词）：
	Delete(uids ...imapv2.UID) error
	Move(mailbox string, uids ...imapv2.UID) error
	// IDLE 能力（M6）：
	CanIDLE() bool
	StartIDLE() (*coreimap.IdleHandle, error)
	SetIDLEHandler(func(coreimap.IDLEEvent))
	// FetchRawMessage 获取指定 UID 的整封原始 RFC 5322 字节（M7 附件下载使用）。
	FetchRawMessage(uid imapv2.UID) ([]byte, error)
	Close() error
}

// AccountConfigProvider 由 account.Service 实现。
type AccountConfigProvider interface {
	IMAPConfig(id uint) (types.IMAPConfig, error)
	TouchLastSync(id uint, t time.Time) error
	IsEnabled(id uint) (bool, error)
}

// Event 是推送给前端的同步事件。
type Event struct {
	Type      string `json:"type"` // "new_mail"
	AccountID uint   `json:"account_id"`
	FolderID  uint   `json:"folder_id"`
	// NewCount 为本次同步「本地新增行数」，非服务器新邮件精确数（UIDVALIDITY
	// 重建时等于整库行数）。仅作为「有变化、需刷新」的提示，前端据此失效查询；
	// 若将来要展示精确数字，应改为基于 UID>prevUIDNext 的计数。
	NewCount int `json:"new_count"`
}

// Publisher 由 sse.Hub 适配实现（Manager 只依赖发布能力）。
type Publisher interface {
	Publish(payload []byte)
}

// AccountLister 是 Manager 调度所需的账户能力。account.Service 满足之。
type AccountLister interface {
	ListEnabledIDs() ([]uint, error)
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

// ErrAccountDisabled 表示账户已停用，拒绝同步。
var ErrAccountDisabled = errors.New("account is disabled")

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

// Trigger 启动一次后台首同步；账户已停用返回 ErrAccountDisabled；同账户已在运行则返回 ErrSyncRunning。
func (s *Service) Trigger(accountID uint) error {
	enabled, err := s.accounts.IsEnabled(accountID)
	if err != nil {
		return err
	}
	if !enabled {
		return ErrAccountDisabled
	}

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

// AttachmentResult 是附件下载的结果载体。
type AttachmentResult struct {
	Filename    string
	ContentType string
	Data        []byte
}

// ErrAttachmentNotFound 表示请求的附件索引超出范围。
var ErrAttachmentNotFound = errors.New("attachment not found")

// AttachmentContent 按需从 IMAP 取整封邮件，解析出第 idx 个附件
// （顺序同 MessageDetail.attachments，含内联图）。
func (s *Service) AttachmentContent(messageID uint, idx int) (*AttachmentResult, error) {
	m, err := s.messages.GetByID(messageID)
	if err != nil {
		return nil, err
	}
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
	raw, err := sess.FetchRawMessage(imapv2.UID(m.UID))
	if err != nil {
		return nil, err
	}
	atts, err := coreparser.ExtractAttachments(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if idx < 0 || idx >= len(atts) {
		return nil, ErrAttachmentNotFound
	}
	a := atts[idx]
	if a.Err != nil {
		return nil, fmt.Errorf("read attachment content: %w", a.Err)
	}
	ct := a.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	return &AttachmentResult{Filename: a.Filename, ContentType: ct, Data: a.Content}, nil
}

// AccountStats 汇总账户下的邮件数与文件夹数。
type AccountStats struct {
	MessageCount int64 `json:"message_count"`
	FolderCount  int64 `json:"folder_count"`
}

func (s *Service) AccountStats(accountID uint) (AccountStats, error) {
	mc, err := s.messages.CountByAccount(accountID)
	if err != nil {
		return AccountStats{}, err
	}
	fc, err := s.folders.CountByAccount(accountID)
	if err != nil {
		return AccountStats{}, err
	}
	return AccountStats{MessageCount: mc, FolderCount: fc}, nil
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
