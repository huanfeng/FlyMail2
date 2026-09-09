package auth

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	coredb "flymail-core/database"
)

func newLimiter(t *testing.T) *Limiter {
	t.Helper()
	db, err := coredb.OpenSQLite(coredb.Options{Path: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateLimiter(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return NewLimiter(db)
}

func TestLimiterBlocksAfterTenFailures(t *testing.T) {
	l := newLimiter(t)
	base := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	now := base
	l.now = func() time.Time { return now }

	for i := 1; i <= 10; i++ {
		if err := l.Check("1.2.3.4"); err != nil {
			t.Fatalf("attempt %d should be allowed: %v", i, err)
		}
		if err := l.Fail("1.2.3.4"); err != nil {
			t.Fatal(err)
		}
	}
	// 第 11 次：429，剩余等待 = 窗口结束 - 现在
	now = base.Add(5 * time.Minute)
	var tooMany *ErrTooManyAttempts
	if err := l.Check("1.2.3.4"); !errors.As(err, &tooMany) || tooMany.RetryAfter != 10*time.Minute {
		t.Fatalf("11th attempt should be blocked with 10m retry-after, got %v", err)
	}
	// 其它 IP 不受影响
	if err := l.Check("5.6.7.8"); err != nil {
		t.Errorf("other ip: %v", err)
	}
	// 窗口过期自动解封，且计数重新开始
	now = base.Add(16 * time.Minute)
	if err := l.Check("1.2.3.4"); err != nil {
		t.Errorf("after window: %v", err)
	}
	if err := l.Fail("1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	if err := l.Check("1.2.3.4"); err != nil {
		t.Errorf("one failure in new window must not block: %v", err)
	}
	// 成功登录清零
	if err := l.Reset("1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 9; i++ {
		_ = l.Fail("1.2.3.4")
	}
	if err := l.Check("1.2.3.4"); err != nil {
		t.Errorf("9 failures after reset must not block: %v", err)
	}
}

// TestLimiterConcurrentFailures：并发失败必须逐次累加——读改写的实现会互相覆盖，40 个并发只记到 1 次。
func TestLimiterConcurrentFailures(t *testing.T) {
	l := newLimiter(t)
	const n = 40
	done := make(chan error, n)
	for range n {
		go func() { done <- l.Fail("7.7.7.7") }()
	}
	for range n {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	var a LoginAttempt
	if err := l.db.Where("ip = ?", "7.7.7.7").First(&a).Error; err != nil {
		t.Fatal(err)
	}
	if a.Failures != n {
		t.Errorf("failures = %d, want %d", a.Failures, n)
	}
	var tooMany *ErrTooManyAttempts
	if err := l.Check("7.7.7.7"); !errors.As(err, &tooMany) {
		t.Errorf("should be blocked: %v", err)
	}
}

// TestLimiterSurvivesRestart：记录落库，新建一个 Limiter（模拟重启）后仍处于封禁。
func TestLimiterSurvivesRestart(t *testing.T) {
	l := newLimiter(t)
	for i := 0; i < 10; i++ {
		_ = l.Fail("9.9.9.9")
	}
	restarted := NewLimiter(l.db)
	var tooMany *ErrTooManyAttempts
	if err := restarted.Check("9.9.9.9"); !errors.As(err, &tooMany) {
		t.Errorf("block state must persist across restart: %v", err)
	}
}
