// Package monitoring 提供只读的系统监控聚合：系统概览 + 各账户健康。
package monitoring

import (
	"os"
	"time"

	"flymail/modules/email/account"
	"flymail/modules/email/folder"
	syncmod "flymail/modules/email/sync"
)

// Overview 是系统概览。
type Overview struct {
	Accounts         int    `json:"accounts"`
	Folders          int64  `json:"folders"`
	Messages         int64  `json:"messages"`
	Unread           int64  `json:"unread"`
	ActiveWorkers    int    `json:"active_workers"`
	PollIntervalSec  int    `json:"poll_interval_sec"`
	UptimeSec        int64  `json:"uptime_sec"`
	Version          string `json:"version"`
	DBSizeBytes      int64  `json:"db_size_bytes"`
	PendingWriteback int64  `json:"pending_writeback"` // 待回写操作数
}

// AccountHealth 是单账户的健康快照。
type AccountHealth struct {
	ID           uint       `json:"id"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	Enabled      bool       `json:"enabled"`
	Status       string     `json:"status"`
	LastSyncAt   *time.Time `json:"last_sync_at,omitempty"`
	MessageCount int64      `json:"message_count"`
	FolderCount  int64      `json:"folder_count"`
	HasWorker    bool       `json:"has_worker"` // 是否有后台同步 worker
	SyncPhase    string     `json:"sync_phase"` // 最近一次同步阶段（none/queued/folders/messages/done/error）
	SyncError    string     `json:"sync_error,omitempty"`
	BreakerOpen  bool       `json:"breaker_open"` // 账户熔断是否打开
	QueueDepth   int        `json:"queue_depth"`  // 后台任务队列深度
	Mode         string     `json:"mode"`         // 当前 runner 模式：idle/polling/disconnected/...
}

// Service 聚合各子系统的只读状态。
type Service struct {
	accounts  *account.Service
	folders   *folder.Service
	sync      *syncmod.Service
	manager   *syncmod.Manager
	startedAt time.Time
	version   string
	dbPath    string
}

func NewService(
	accounts *account.Service,
	folders *folder.Service,
	sync *syncmod.Service,
	manager *syncmod.Manager,
	startedAt time.Time,
	version, dbPath string,
) *Service {
	return &Service{
		accounts: accounts, folders: folders, sync: sync, manager: manager,
		startedAt: startedAt, version: version, dbPath: dbPath,
	}
}

// Overview 汇总系统级指标。
func (s *Service) Overview() (Overview, error) {
	accts, err := s.accounts.List()
	if err != nil {
		return Overview{}, err
	}
	var folders, messages, unread int64
	for _, a := range accts {
		if st, err := s.sync.AccountStats(a.ID); err == nil {
			messages += st.MessageCount
			folders += st.FolderCount
		}
		if fs, err := s.folders.List(a.ID); err == nil {
			for i := range fs {
				unread += int64(fs[i].UnreadCount)
			}
		}
	}
	var dbSize int64
	if fi, err := os.Stat(s.dbPath); err == nil {
		dbSize = fi.Size()
	}
	return Overview{
		Accounts:         len(accts),
		Folders:          folders,
		Messages:         messages,
		Unread:           unread,
		ActiveWorkers:    len(s.manager.WorkerAccountIDs()),
		PollIntervalSec:  s.manager.CurrentPollSeconds(),
		UptimeSec:        int64(time.Since(s.startedAt).Seconds()),
		Version:          s.version,
		DBSizeBytes:      dbSize,
		PendingWriteback: s.manager.PendingWritebackCount(),
	}, nil
}

// Accounts 返回各账户健康快照。
func (s *Service) Accounts() ([]AccountHealth, error) {
	accts, err := s.accounts.List()
	if err != nil {
		return nil, err
	}
	workers := map[uint]bool{}
	for _, id := range s.manager.WorkerAccountIDs() {
		workers[id] = true
	}
	rstats := map[uint]syncmod.RunnerStat{}
	for _, rs := range s.manager.RunnerStats() {
		rstats[rs.AccountID] = rs
	}
	out := make([]AccountHealth, 0, len(accts))
	for _, a := range accts {
		h := AccountHealth{
			ID: a.ID, Name: a.Name, Email: a.Email,
			Enabled: a.Enabled, Status: a.Status, LastSyncAt: a.LastSyncAt,
			HasWorker: workers[a.ID], SyncPhase: "none",
		}
		if rs, ok := rstats[a.ID]; ok {
			h.BreakerOpen = rs.BreakerOpen
			h.QueueDepth = rs.QueueDepth
			h.Mode = rs.Mode
		}
		if st, err := s.sync.AccountStats(a.ID); err == nil {
			h.MessageCount = st.MessageCount
			h.FolderCount = st.FolderCount
		}
		if st, ok := s.sync.StatusOf(a.ID); ok {
			h.SyncPhase = string(st.Phase)
			h.SyncError = st.Error
		}
		out = append(out, h)
	}
	return out, nil
}

// Diagnostics 返回某账户 runner 的运行时诊断（模式/IDLE 三态/熔断/队列/事件时间线）。
// 无 runner（账户停用或尚未拉起）返回 ok=false。
func (s *Service) Diagnostics(accountID uint) (syncmod.RunnerDiag, bool) {
	return s.manager.AccountDiagnostics(accountID)
}
