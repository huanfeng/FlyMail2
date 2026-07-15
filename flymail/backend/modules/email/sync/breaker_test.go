package sync

import (
	"testing"
	"time"
)

// newTestBreaker 返回带可控时钟的熔断器。
func newTestBreaker() (*breaker, *time.Time) {
	base := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	clock := &base
	b := newBreaker()
	b.now = func() time.Time { return *clock }
	return b, clock
}

// TestBreaker_OpensAtThreshold 连续失败达阈值才打开熔断。
func TestBreaker_OpensAtThreshold(t *testing.T) {
	b, _ := newTestBreaker()
	for i := 0; i < breakerThreshold-1; i++ {
		b.RecordFailure()
		if !b.AllowBackground() {
			t.Fatalf("第 %d 次失败不应熔断", i+1)
		}
	}
	b.RecordFailure() // 第 5 次
	if b.AllowBackground() {
		t.Fatal("达阈值后应熔断，后台被拒")
	}
	if st := b.State(); !st.Open || st.Failures != breakerThreshold {
		t.Fatalf("State = %+v，期望 open 且 failures=%d", st, breakerThreshold)
	}
}

// TestBreaker_SuccessResets 前台穿透成功（RecordSuccess）立即清零并关闭熔断。
func TestBreaker_SuccessResets(t *testing.T) {
	b, _ := newTestBreaker()
	for i := 0; i < breakerThreshold; i++ {
		b.RecordFailure()
	}
	if b.AllowBackground() {
		t.Fatal("应处于熔断")
	}
	b.RecordSuccess()
	if !b.AllowBackground() {
		t.Fatal("成功后应恢复")
	}
	if st := b.State(); st.Open || st.Failures != 0 {
		t.Fatalf("State = %+v，期望 closed 且 failures=0", st)
	}
}

// TestBreaker_CooldownExpiryAllowsRetry 冷却期过后半开放行一次后台试探。
func TestBreaker_CooldownExpiryAllowsRetry(t *testing.T) {
	b, clock := newTestBreaker()
	for i := 0; i < breakerThreshold; i++ {
		b.RecordFailure()
	}
	if b.AllowBackground() {
		t.Fatal("冷却窗口内后台应被拒")
	}
	// 推进到冷却期刚过。
	*clock = clock.Add(breakerCooldown + time.Second)
	if !b.AllowBackground() {
		t.Fatal("冷却期过后应放行半开试探")
	}
	if st := b.State(); st.Open {
		t.Fatalf("冷却期过后 State 不应为 open：%+v", st)
	}
}

// TestBreaker_ReopenOnHalfOpenFailure 半开试探再失败重新打开熔断。
func TestBreaker_ReopenOnHalfOpenFailure(t *testing.T) {
	b, clock := newTestBreaker()
	for i := 0; i < breakerThreshold; i++ {
		b.RecordFailure()
	}
	*clock = clock.Add(breakerCooldown + time.Second)
	if !b.AllowBackground() {
		t.Fatal("冷却过后应放行")
	}
	// 半开试探再次失败：failures 累加、重新熔断 cooldown。
	b.RecordFailure()
	if b.AllowBackground() {
		t.Fatal("半开失败后应重新熔断")
	}
	if st := b.State(); !st.Open || st.Failures != breakerThreshold+1 {
		t.Fatalf("State = %+v，期望 open 且 failures=%d", st, breakerThreshold+1)
	}
}
