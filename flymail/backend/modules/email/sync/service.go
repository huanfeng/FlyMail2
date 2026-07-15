package sync

import (
	"bytes"
	"context"
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

// EmitFunc 是通知事件回调（解耦：sync 包不依赖 notify 包，由 app 装配指向 notify.Emit）。
// 参数：事件类型、相关账户 id（0 表示无）、标题、正文。
type EmitFunc func(eventType string, accountID uint, title, body string)

// 事件类型字符串（与 notify 包常量保持一致；此处镜像以避免反向依赖 notify）。
const (
	notifyMailNew    = "mail_new"
	notifySyncFailed = "sync_failed"
)

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
	PhaseQueued   Phase = "queued" // 排队等待全局同步名额（前端未识别按进行中展示）
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

// orchestrator 是 Service 投递任务到账户 runner 的门面（*Manager 实现）。
// 为 nil 时 Service 退化为旧的每操作直连路径（供不接 Manager 的单测使用）。
type orchestrator interface {
	// TriggerSync 前台优先执行一次全量同步（含 queued 重试与状态上报），阻塞至完成或 ctx 取消。
	TriggerSync(ctx context.Context, accountID uint) error
	// ForegroundOp 前台任务（详情/附件），阻塞等结果。
	ForegroundOp(ctx context.Context, accountID uint, run func(Session) error) error
	// BackgroundOp 后台任务（回写），非阻塞投递。
	BackgroundOp(accountID uint, run func(Session) error) bool
}

// Service 编排单账户的首次同步（文件夹 → 收件箱消息），并通过内存 map 对外暴露进度。
type Service struct {
	accounts AccountConfigProvider
	folders  *folder.Service
	messages *message.Service

	dial        func(types.IMAPConfig) (Session, error)
	syncDepthFn func() int
	emit        EmitFunc
	orch        orchestrator

	status  *statusStore
	mu      gosync.Mutex
	running map[uint]bool

	wbCh chan wbOp
}

// SetManager 注入 runner 编排器（Manager），并让二者共享同一份同步状态存储
// （后台同步与手动触发的进度写入同一处，唯一写入方均为账户 runner）。
func (s *Service) SetManager(m *Manager) {
	s.orch = m
	m.setStatusStore(s.status)
}

// SetEmitter 注入通知回调（同步失败等事件）。
func (s *Service) SetEmitter(fn EmitFunc) { s.emit = fn }

// NewService 创建 Sync 服务。
func NewService(accounts AccountConfigProvider, folders *folder.Service, messages *message.Service) *Service {
	s := &Service{
		accounts:    accounts,
		folders:     folders,
		messages:    messages,
		dial:        defaultDial,
		syncDepthFn: func() int { return 0 },
		status:      newStatusStore(),
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

// foregroundOpTimeout 是前台按需操作（详情/附件）的等待上限；超时返回错误（任务仍会执行完）。
const foregroundOpTimeout = 20 * time.Second

func defaultDial(cfg types.IMAPConfig) (Session, error) { return coreimap.Dial(cfg) }

// runForeground 在账户 runner 上前台执行一个 IMAP 操作（复用其连接，带超时）；
// 无 Manager 时退回旧的即建即用直连（供单测）。
func (s *Service) runForeground(accountID uint, run func(Session) error) error {
	if s.orch == nil {
		return s.withDialedSession(accountID, run)
	}
	ctx, cancel := context.WithTimeout(context.Background(), foregroundOpTimeout)
	defer cancel()
	return s.orch.ForegroundOp(ctx, accountID, run)
}

// withDialedSession 为某账户即建一条连接执行 run，结束关闭（orch 为 nil 的回退路径）。
func (s *Service) withDialedSession(accountID uint, run func(Session) error) error {
	cfg, err := s.accounts.IMAPConfig(accountID)
	if err != nil {
		return err
	}
	sess, err := s.dial(cfg)
	if err != nil {
		return err
	}
	defer sess.Close()
	return run(sess)
}

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
	s.mu.Unlock()

	// 初始置 queued：等待 runner 拾取/全局同步名额；runner 执行后转 folders→messages→done。
	s.status.begin(accountID, PhaseQueued)
	go s.runTrigger(accountID)
	return nil
}

// runTrigger 执行一次手动触发的同步：有 Manager 则投递前台全量同步任务到账户 runner，
// 否则退回旧的直连路径（单测无 Manager 时）。
func (s *Service) runTrigger(accountID uint) {
	defer func() {
		s.mu.Lock()
		s.running[accountID] = false
		s.mu.Unlock()
	}()

	if s.orch == nil {
		s.run(accountID)
		return
	}
	// runner 内部会写状态（folders/messages/done/queued）；此处仅在失败时补通知。
	if err := s.orch.TriggerSync(context.Background(), accountID); err != nil && !errors.Is(err, context.Canceled) {
		if s.emit != nil {
			s.emit(notifySyncFailed, accountID, "同步失败", err.Error())
		}
	}
}

// StatusOf 返回指定账户的同步状态快照。
func (s *Service) StatusOf(accountID uint) (Status, bool) {
	return s.status.get(accountID)
}

func (s *Service) setStatus(accountID uint, fn func(*Status)) {
	s.status.update(accountID, fn)
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
	// 站内/外发通知：同步失败
	if s.emit != nil {
		s.emit(notifySyncFailed, accountID, "同步失败", err.Error())
	}
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
	var raw []byte
	fetch := func(sess Session) error {
		if _, err := sess.SelectFolder(f.Path); err != nil {
			return err
		}
		b, err := sess.FetchRawMessage(imapv2.UID(m.UID))
		if err != nil {
			return err
		}
		raw = b
		return nil
	}
	if err := s.runForeground(m.AccountID, fetch); err != nil {
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
		fetch := func(sess Session) error {
			if _, err := sess.SelectFolder(f.Path); err != nil {
				return err
			}
			emails, err := sess.FetchByUIDs(
				[]imapv2.UID{imapv2.UID(m.UID)},
				coreimap.FetchOptions{FetchBody: true, FallbackHeaders: true},
			)
			if err != nil {
				return err
			}
			if len(emails) > 0 {
				if err := s.messages.StoreParsedBody(messageID, emails[0]); err != nil {
					return err
				}
			}
			return nil
		}
		if err := s.runForeground(m.AccountID, fetch); err != nil {
			return nil, err
		}
	}
	return s.messages.Detail(messageID)
}
