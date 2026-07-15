package sync

import (
	gosync "sync"
	"time"
)

// statusStore 是同步进度的内存存储（重启丢失，见设计 §6）。
// 由 Service（手动触发）与 Manager（后台同步）共享同一实例，写入方均为账户 runner goroutine，
// 故同一账户的写天然串行；跨账户由 mu 保护。
type statusStore struct {
	mu       gosync.Mutex
	statuses map[uint]*Status
}

func newStatusStore() *statusStore {
	return &statusStore{statuses: map[uint]*Status{}}
}

// begin 开启/重置某账户的同步进度。
func (s *statusStore) begin(accountID uint, phase Phase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.statuses[accountID] = &Status{
		AccountID: accountID,
		Phase:     phase,
		StartedAt: now,
		UpdatedAt: now,
	}
}

// update 在既有状态上施加变更并刷新 UpdatedAt（无既有状态则忽略）。
func (s *statusStore) update(accountID uint, fn func(*Status)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.statuses[accountID]; st != nil {
		fn(st)
		st.UpdatedAt = time.Now()
	}
}

// markPhase 更新阶段（无既有状态则忽略）。
func (s *statusStore) markPhase(accountID uint, p Phase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.statuses[accountID]; st != nil {
		st.Phase = p
		st.UpdatedAt = time.Now()
	}
}

// markDone 标记完成并写入总数。
func (s *statusStore) markDone(accountID uint, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.statuses[accountID]; st != nil {
		st.Phase = PhaseDone
		st.Total = total
		st.Processed = total
		st.UpdatedAt = time.Now()
	}
}

// fail 标记失败并记录错误。
func (s *statusStore) fail(accountID uint, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.statuses[accountID]; st != nil {
		st.Phase = PhaseError
		st.Error = errMsg
		st.UpdatedAt = time.Now()
	}
}

// get 返回快照。
func (s *statusStore) get(accountID uint) (Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.statuses[accountID]
	if !ok {
		return Status{}, false
	}
	return *st, true
}
