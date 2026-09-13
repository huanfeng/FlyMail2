package sync

import (
	gosync "sync"
	"time"
)

// statusStore 是同步进度的内存存储（重启丢失，见设计 §6）。
// 由 Service（手动触发）与 Manager（后台同步）共享同一实例，写入方均为账户 runner goroutine，
// 故同一账户的写天然串行；跨账户由 mu 保护。
//
// onChange 让「状态变了」这件事只有一个出口：所有变更方法都经由 mutate/begin 通知，
// 没有哪条写路径能绕过它。前端的后台同步进度就靠这个出口推送——
// 若改成在各个调用点手动发事件，漏掉一处的表现是"某种同步不显示进度"，
// 而那种漏几乎不可能被测试发现。
type statusStore struct {
	mu       gosync.Mutex
	statuses map[uint]*Status
	onChange func(Status)
}

func newStatusStore() *statusStore {
	return &statusStore{statuses: map[uint]*Status{}}
}

// setOnChange 注册状态变更回调（Manager 用它发布 SSE）。只在装配期调用一次。
//
// ⚠ 回调里**不能**反过来调 statusStore 的写方法：那会无限递归发布。
// 这是「所有变更走同一个出口」这个设计唯一能自伤的地方——出口越统一，
// 从出口再绕回去的代价越大。现在的 publishStatus 只做序列化与广播，没有这个问题。
func (s *statusStore) setOnChange(fn func(Status)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

// mutate 在既有状态上施加变更、刷新 UpdatedAt 并通知订阅者（无既有状态则忽略）。
// ⚠ 回调在**锁外**调用：它会走到 SSE 发布，那一侧的耗时不该把状态存储锁住，
// 更不该因为回调里再读状态而自死锁。
func (s *statusStore) mutate(accountID uint, fn func(*Status)) {
	s.mu.Lock()
	st := s.statuses[accountID]
	if st == nil {
		s.mu.Unlock()
		return
	}
	fn(st)
	st.UpdatedAt = time.Now()
	snap, cb := *st, s.onChange
	s.mu.Unlock()
	if cb != nil {
		cb(snap)
	}
}

// begin 开启/重置某账户的同步进度。
func (s *statusStore) begin(accountID uint, phase Phase) {
	s.mu.Lock()
	now := time.Now()
	st := &Status{
		AccountID: accountID,
		Phase:     phase,
		StartedAt: now,
		UpdatedAt: now,
	}
	s.statuses[accountID] = st
	snap, cb := *st, s.onChange
	s.mu.Unlock()
	if cb != nil {
		cb(snap)
	}
}

// update 在既有状态上施加变更（保留旧名，供 Service 的直连路径使用）。
func (s *statusStore) update(accountID uint, fn func(*Status)) {
	s.mutate(accountID, fn)
}

// markPhase 更新阶段（无既有状态则忽略）。
func (s *statusStore) markPhase(accountID uint, p Phase) {
	s.mutate(accountID, func(st *Status) { st.Phase = p })
}

// beginFolders 记下这一轮要过多少个文件夹，并把已完成数归零。
func (s *statusStore) beginFolders(accountID uint, total int) {
	s.mutate(accountID, func(st *Status) {
		st.FoldersTotal = total
		st.FoldersDone = 0
		st.CurrentFolder = ""
		st.CurrentFolderType = ""
	})
}

// enterFolder 标记正在同步哪个文件夹（名字与类型一起给，见 Status 的字段注释）。
func (s *statusStore) enterFolder(accountID uint, name, folderType string) {
	s.mutate(accountID, func(st *Status) {
		st.CurrentFolder = name
		st.CurrentFolderType = folderType
	})
}

// finishFolder 完成一个文件夹。
func (s *statusStore) finishFolder(accountID uint) {
	s.mutate(accountID, func(st *Status) { st.FoldersDone++ })
}

// markDone 标记完成。
func (s *statusStore) markDone(accountID uint) {
	s.mutate(accountID, func(st *Status) {
		st.Phase = PhaseDone
		st.CurrentFolder = ""
		st.CurrentFolderType = ""
	})
}

// beginBodies 记下这一轮要回补多少封正文（0 表示没有可补的，清空这条弱表达）。
func (s *statusStore) beginBodies(accountID uint, total int) {
	s.mutate(accountID, func(st *Status) {
		st.BodiesTotal = total
		st.BodiesDone = 0
	})
}

// advanceBodies 推进已回补封数。
func (s *statusStore) advanceBodies(accountID uint, done int) {
	s.mutate(accountID, func(st *Status) { st.BodiesDone = done })
}

// endBodies 收起正文回补的表达。
func (s *statusStore) endBodies(accountID uint) {
	s.mutate(accountID, func(st *Status) {
		st.BodiesTotal = 0
		st.BodiesDone = 0
	})
}

// fail 标记失败并记录错误。
func (s *statusStore) fail(accountID uint, errMsg string) {
	s.mutate(accountID, func(st *Status) {
		st.Phase = PhaseError
		st.Error = errMsg
		st.CurrentFolder = ""
		st.CurrentFolderType = ""
	})
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
