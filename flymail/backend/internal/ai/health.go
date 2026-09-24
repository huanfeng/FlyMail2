package ai

import (
	"sort"
	"sync"
	"time"
)

// State 是一个配置的运行时健康状态。
//
// 只在内存里：冷却是几十秒到一小时量级的事，重启后清零的代价只是多撞一次失败，
// 落库反倒要处理「状态过期了还挂着」。
type State struct {
	CooldownUntil time.Time `json:"cooldown_until,omitzero"`
	LastError     string    `json:"last_error,omitzero"`
	LastKind      Kind      `json:"last_kind,omitzero"`
	LastOKAt      time.Time `json:"last_ok_at,omitzero"`
	LastFailAt    time.Time `json:"last_fail_at,omitzero"`
	// Failures 是连续失败次数，成功一次清零；用于限流/故障的指数退避。
	Failures int `json:"failures"`
}

// Cooling 报告在 now 时刻是否处于冷却中。
func (s State) Cooling(now time.Time) bool { return now.Before(s.CooldownUntil) }

// Health 按配置 ID 记录健康状态，并发安全。零值不可用，用 NewHealth 构造。
type Health struct {
	mu  sync.Mutex
	m   map[uint]*State
	now func() time.Time
}

func NewHealth() *Health {
	return &Health{m: map[uint]*State{}, now: time.Now}
}

// 冷却时长。
const (
	cooldownQuota    = time.Hour        // 余额不会自己长回来；充值后可手动解除
	cooldownAuth     = 30 * time.Minute // 配置问题，反复撞只会刷日志
	cooldownRejected = 10 * time.Minute // 这家不认这个模型
	cooldownBase     = 30 * time.Second // 限流/故障的退避起点
	cooldownMax      = 10 * time.Minute // 退避上限
	rateLimitFloor   = time.Minute      // 限流没给 Retry-After 时至少等这么久
)

// cooldownFor 计算这次失败后的冷却时长；0 表示不冷却（仍然切换）。
func cooldownFor(kind Kind, err error, failures int) time.Duration {
	switch kind {
	case KindQuota:
		return cooldownQuota
	case KindAuth, KindNotConfigured:
		return cooldownAuth
	case KindRejected:
		return cooldownRejected
	case KindRateLimit:
		if ra := retryAfterOf(err); ra > 0 {
			return min(ra, cooldownMax)
		}
		return max(backoff(failures), rateLimitFloor)
	case KindUpstream:
		return backoff(failures)
	}
	return 0 // 截断、输出不可用：与服务商好坏无关
}

func backoff(failures int) time.Duration {
	d := cooldownBase
	for i := 1; i < failures && d < cooldownMax; i++ {
		d *= 2
	}
	return min(d, cooldownMax)
}

func retryAfterOf(err error) time.Duration {
	if apiErr, ok := asAPIError(err); ok {
		return apiErr.RetryAfter
	}
	return 0
}

// RecordOK 记一次成功：清掉冷却与连续失败计数。
func (h *Health) RecordOK(id uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.get(id)
	st.CooldownUntil = time.Time{}
	st.Failures = 0
	st.LastOKAt = h.now()
}

// RecordFail 记一次失败并按类别设冷却。调用方取消不记（那不是服务商的错）。
func (h *Health) RecordFail(id uint, err error) Kind {
	kind := Classify(err)
	if kind == KindCanceled {
		return kind
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.get(id)
	st.Failures++
	now := h.now()
	st.LastFailAt = now
	st.LastKind = kind
	st.LastError = err.Error()
	if d := cooldownFor(kind, err, st.Failures); d > 0 {
		st.CooldownUntil = now.Add(d)
	}
	return kind
}

// Reset 手动解除冷却（「我已经充值了」）。保留最后一次错误信息供参考。
func (h *Health) Reset(id uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.get(id)
	st.CooldownUntil = time.Time{}
	st.Failures = 0
}

// Forget 删除一个配置的状态（配置被删除时调用）。
func (h *Health) Forget(id uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.m, id)
}

// Get 返回一个配置的状态快照。
func (h *Health) Get(id uint) State {
	h.mu.Lock()
	defer h.mu.Unlock()
	if st, ok := h.m[id]; ok {
		return *st
	}
	return State{}
}

// Order 给出尝试顺序：未冷却的按原顺序在前；冷却中的排到后面，最早解冻的先试。
//
// 冷却中的**不跳过**：全员都在冷却时，用户点了翻译就该真的去试一遍，
// 而不是回一句「都在冷却」——冷却时长是估计，服务商可能早就恢复了。
func (h *Health) Order(ids []uint) []uint {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	var ready, cooling []uint
	for _, id := range ids {
		if st, ok := h.m[id]; ok && st.Cooling(now) {
			cooling = append(cooling, id)
		} else {
			ready = append(ready, id)
		}
	}
	sort.SliceStable(cooling, func(i, j int) bool {
		return h.m[cooling[i]].CooldownUntil.Before(h.m[cooling[j]].CooldownUntil)
	})
	return append(ready, cooling...)
}

// AllCooling 报告 ids 是否全部处于冷却中（空列表视为真）。
func (h *Health) AllCooling(ids []uint) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	for _, id := range ids {
		st, ok := h.m[id]
		if !ok || !st.Cooling(now) {
			return false
		}
	}
	return true
}

// Now 返回 Health 使用的时钟（测试可替换），供调用方判断 Cooling。
func (h *Health) Now() time.Time { return h.now() }

func (h *Health) get(id uint) *State {
	st, ok := h.m[id]
	if !ok {
		st = &State{}
		h.m[id] = st
	}
	return st
}
