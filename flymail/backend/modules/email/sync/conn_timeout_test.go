package sync

import (
	"testing"
	"time"

	coreimap "flymail-core/imap"
)

// runner 持有连接的节奏必须落在 core/imap 那套超时之内。
//
// ── 为什么要跨模块钉这一条 ───────────────────────────────────────────────────
//
// core/imap 给「等下一条响应的第一个字节」加了上限，用来发现被静默掐断的连接
// （见 core/imap/timeout.go）。代价是：连接空着的时候，go-imap 的读 goroutine
// 正阻塞在这个等待上——**上限同时限定了一条空闲连接能被静默持有多久**。
//
// 于是这两个模块之间多了一条隐性契约：
//
//	runner 持有空闲连接 60 秒  <  core 的空闲上限 5 分钟
//	runner 每 29 分钟重进 IDLE  <  core 的 IDLE 上限 35 分钟
//
// 违反它的后果很隐蔽：连接不是报错，而是被定期判死然后重连。同步表面上还在走，
// 只是每隔几分钟多一次没必要的重连，日志里看着像网络不稳。改任何一侧的时间常量
// 都可能悄悄踩到——所以把契约写成断言，而不是写在注释里。
func TestRunnerHoldTimesFitConnTimeouts(t *testing.T) {
	if got, limit := idleCloseInterval, coreimap.ConnIdleTimeout(); got >= limit {
		t.Fatalf("runner 持有空闲连接 %s，而连接层 %s 就判死——健康连接会被定期误杀",
			got, limit)
	}
	if got, limit := idleRefreshInterval, coreimap.IDLEHoldTimeout(); got >= limit {
		t.Fatalf("runner 每 %s 才重进一次 IDLE，而连接层 %s 就判死——IDLE 会被定期打断",
			got, limit)
	}
	// 探活阈值也得在空闲上限之内：连接闲置超过空闲上限的话，探活对象早就没了。
	if got, limit := staleAfter, coreimap.ConnIdleTimeout(); got >= limit {
		t.Fatalf("探活阈值 %s 不该大于连接层的空闲上限 %s", got, limit)
	}
}

// 留出的余量要够。刚好卡着线等于没有余量：一次 GC 停顿、一次慢同步就会越界。
func TestConnTimeoutsHaveHeadroom(t *testing.T) {
	const minRatio = 2 // 至少两倍

	if float64(coreimap.ConnIdleTimeout()) < minRatio*float64(idleCloseInterval) {
		t.Fatalf("空闲上限 %s 相对 runner 的 %s 余量不足 %d 倍",
			coreimap.ConnIdleTimeout(), idleCloseInterval, minRatio)
	}
	// IDLE 那侧 29 分钟 vs 35 分钟做不到两倍，按绝对余量要求：至少 5 分钟。
	if gap := coreimap.IDLEHoldTimeout() - idleRefreshInterval; gap < 5*time.Minute {
		t.Fatalf("IDLE 上限只比重进周期多 %s，余量太小", gap)
	}
}
