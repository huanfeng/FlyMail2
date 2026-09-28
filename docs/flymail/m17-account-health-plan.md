# M17 账户健康可观测性 — 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 账户进入故障态（熔断/OAuth 失效）时推送提醒（翻转沿 + 每 24h 重复 + 恢复通知），并在侧栏、全局横幅、账户管理页三处给出明确视觉区分。

**Architecture:** 新增 health watcher（sync 包内，60s tick 差分故障态集合）作为通知的单一出口；`GET /accounts` 响应扩展 `health` 对象（needs_reauth + breaker + last_error），前端复用现有 `['accounts']` 查询零新增轮询；SSE 复用 notify 流。事件类型复用 `sync_failed`/`account_status`，零新事件类型。

**Tech Stack:** Go + GORM + zap（backend）；React + TanStack Query + react-i18next + vitest（frontend）

**Spec:** `docs/flymail/m17-account-health.md`（计划与 spec 一起阅读；spec 是意图，计划是落地）

## Global Constraints

- 提交纪律（仓库级）：中文 commit message；`git commit -F - --only -- <路径>`，禁用 `git add -A`/`git add .`；每任务一提交。flymail/backend/CLAUDE.md 要求：**未经用户明确指示不自行提交**——执行开始前由用户确认提交授权。
- 后端每次改 go 代码后 `gofmt -w <文件>`（backend CLAUDE.md）。
- 测试命令：后端 `go test ./...`（cwd `flymail/backend`）；前端 `pnpm vitest run` 与 `pnpm tsc -b`（cwd `flymail/frontend`，包管理器必须 pnpm）。
- 测试服务只监听 127.0.0.1（0.0.0.0 会弹防火墙确认）。
- 后端通知文案硬编码中文（与现状一致）；前端文案全部走 i18n 键（zh/en 同步加）。
- 事件类型只用现有 `sync_failed`（红）/`account_status`（橙），不新增事件类型、不改 notify 的 ValidEvent 白名单。
- watcher 的 tick/now/repeatAfter 可注入；测试不 sleep 真实时间。

## Review Focus

spec 暗示但单个任务测试不直接覆盖、最可能咬人的五类输入（已配测试钉住的标 *）：

1. **watcher 差分的正确性是全功能的地基**：prev/cur 集合差分错（翻转沿误判、恢复误报、禁用误报恢复）会让用户收到矛盾通知。*Task 3 的表驱动差分测试钉住。
2. **「成功后的旧 last_error」误报**：账户恢复健康后 diag.lastErr 不清除（markSync 不清），若 health 判定带上 lastErr 会让健康账户永远显示错误。*Task 5 双测试钉住：TestHealthForMirrorsRunnerState 钉同步侧镜像（LastError 只透传不参与判定），TestServiceHealthProviderInjection 的第二段钉 app 层「健康 → health 缺省即使 LastError 非空」。
3. **sessionStorage 不可用**（隐私模式/禁存储）：横幅关闭记忆读写必须 try/catch，存储不可用时降级为「本会话不再横幅重现」（每渲染读 null → 横幅常显）。*Task 7 测试钉住存储抛异常时横幅仍正常显示。
4. **桌面通知去重窗口**：watcher 翻转沿与手动同步失败同时到达（两条 SSE），10s 窗口内同账户同事件不重弹。*Task 9 测试钉住。
5. **needs_reauth 节流与前台穿透的恢复闭环**：前台手动同步成功必须清 needsReauthUntil，否则重新授权后后台仍停 30 分钟。*Task 4 测试钉住。

---

### Task 1: notify 层外发-only 投递（Event.SkipInApp + EmitExternalFunc）

**Files:**
- Modify: `flymail/backend/modules/system/notify/dto.go:10-25`（Event 加字段）
- Modify: `flymail/backend/modules/system/notify/service.go:93-117`（Emit 判断 SkipInApp）
- Test: `flymail/backend/modules/system/notify/service_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `Event.SkipInApp bool`；`Service.EmitExternalFunc() func(eventType string, accountID uint, messageID uint, title, body string)`（与 EmitFunc 同形）

- [ ] **Step 1.1 写失败测试**（service_test.go 追加）：

```go
func TestEmitSkipInApp(t *testing.T) {
	svc, repo := newSvc(t)

	// 渠道订阅 sync_failed：SkipInApp 只影响站内落库，外发照常。
	var mu sync.Mutex
	var dispatched []notify.Event
	svc.SetDispatcher(func(c *notify.Channel, evt notify.Event) error {
		mu.Lock()
		defer mu.Unlock()
		dispatched = append(dispatched, evt)
		return nil
	})
	enabled := true
	if _, err := svc.CreateChannel(notify.ChannelInput{
		Name: "wh", Kind: "webhook", URL: "http://x", Events: []string{"sync_failed"}, Enabled: &enabled,
	}); err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}

	// 普通 Emit：站内落库 + 外发
	svc.Emit(notify.Event{Type: notify.EventSyncFailed, AccountID: 1, Title: "t", Body: "b"})
	// SkipInApp：只外发，不落库
	svc.Emit(notify.Event{Type: notify.EventSyncFailed, AccountID: 1, Title: "t2", Body: "b", SkipInApp: true})

	// 异步投递是 goroutine，轮询等待 worker 消费（参考本文件既有测试的做法）
	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(dispatched) == 2
	})
	mu.Lock()
	n := countNotifications(t, repo)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("站内通知应为 1 条（SkipInApp 不落库），got %d", n)
	}
}
```

> 说明：`waitFor`/`countNotifications` 若本文件没有现成 helper，按文件内既有测试的轮询/查询写法补一个（查 `notifications` 表计数，`repo.List` 已有分页接口可直接用）。

- [ ] **Step 1.2 运行确认失败**：`go test ./modules/system/notify/ -run TestEmitSkipInApp -v` — 预期 FAIL（`SkipInApp` 字段不存在，编译错误）。
- [ ] **Step 1.3 实现**：

dto.go Event 结构末尾追加字段：

```go
	// SkipInApp 置位时跳过站内落库，只走外发渠道与 SSE。
	//   - 站内通知中心是「历史记录」：同一条持续故障每天重复落一条会把记录淹没，
	//     所以重复提醒不落站内，只推外发渠道。
	//   - 由 EmitExternalFunc 返回的回调置位；普通 EmitFunc 回调不置位。
	SkipInApp bool
```

service.go Emit 中，把落库段包进判断：

```go
	// 站内通知落库（best-effort）。SkipInApp 的事件只走外发：见 Event.SkipInApp 注释。
	if !evt.SkipInApp {
		if err := s.repo.InsertNotification(&Notification{
			Type:      string(evt.Type),
			AccountID: evt.AccountID,
			MessageID: evt.MessageID,
			Title:     evt.Title,
			Body:      evt.Body,
		}); err != nil {
			logger.Error("notify: 写入站内通知失败", zap.Error(err))
		}
	}
```

service.go EmitFunc 之后新增对称方法：

```go
// EmitExternalFunc 与 EmitFunc 同形，但事件只走外发渠道与 SSE，不落站内通知。
// 用于「站内已有权威记录、重复推送只会刷屏」的场景（账户持续故障的每日重复提醒）。
func (s *Service) EmitExternalFunc() func(eventType string, accountID uint, messageID uint, title, body string) {
	return func(eventType string, accountID uint, messageID uint, title, body string) {
		s.Emit(Event{Type: EventType(eventType), AccountID: accountID, MessageID: messageID, Title: title, Body: body, SkipInApp: true})
	}
}
```

- [ ] **Step 1.4 运行确认通过**：`go test ./modules/system/notify/ -v` 全绿。
- [ ] **Step 1.5 gofmt + 提交**：

```bash
gofmt -w modules/system/notify/dto.go modules/system/notify/service.go modules/system/notify/service_test.go
git add modules/system/notify/dto.go modules/system/notify/service.go modules/system/notify/service_test.go
git commit -F - --only -- modules/system/notify/dto.go modules/system/notify/service.go modules/system/notify/service_test.go <<'MSG'
feat(flymail): 通知支持「只外发不落站内」投递（EmitExternalFunc）

账户持续故障的每日重复提醒只推外发渠道，不刷站内通知中心。
MSG
```

---

### Task 2: RunnerStat.LastError（watcher 的 last_error 数据源）

**Files:**
- Modify: `flymail/backend/modules/email/sync/diag.go`（新增 lastErrorOnly 访问器）
- Modify: `flymail/backend/modules/email/sync/manager.go`（RunnerStat 结构 + RunnerStats 填充）
- Test: `flymail/backend/modules/email/sync/manager_test.go`（追加）

**Interfaces:**
- Consumes: `runnerDiag.markErr/msg`、`runnerDiag.snapshot`（既有，勿动）
- Produces: `RunnerStat.LastError string`（json `last_error,omitempty`）；`runnerDiag.lastErrorOnly() (string, time.Time)`（包内访问器）

- [ ] **Step 2.1 写失败测试**（manager_test.go 追加；Manager 结构体字段在包内可直接构造，runner 只需 newRunner）：

```go
func TestRunnerStatsIncludesLastError(t *testing.T) {
	m := &Manager{runners: map[uint]*runner{
		7: newRunner(7,
			func(uint) (types.IMAPConfig, error) { return types.IMAPConfig{}, nil },
			nil,
		),
	}}
	m.runners[7].diag.markErr("dial tcp: connection refused")

	sts := m.RunnerStats()
	if len(sts) != 1 {
		t.Fatalf("应有 1 条 runner 快照，got %d", len(sts))
	}
	if sts[0].LastError != "dial tcp: connection refused" {
		t.Fatalf("LastError = %q, want %q", sts[0].LastError, "dial tcp: connection refused")
	}
}

func TestRunnerStatsLastErrorEmptyWhenHealthy(t *testing.T) {
	// 健康的 runner：markErr 从未调用，LastError 必须为空——
	// watcher 的推送文案若拿到旧错误会误导用户（Review Focus 第 2 条）。
	m := &Manager{runners: map[uint]*runner{
		9: newRunner(9,
			func(uint) (types.IMAPConfig, error) { return types.IMAPConfig{}, nil },
			nil,
		),
	}}
	sts := m.RunnerStats()
	if len(sts) != 1 {
		t.Fatalf("应有 1 条 runner 快照，got %d", len(sts))
	}
	if sts[0].LastError != "" {
		t.Fatalf("健康 runner 的 LastError 应为空，got %q", sts[0].LastError)
	}
}
```

- [ ] **Step 2.2 运行确认失败**：`go test ./modules/email/sync/ -run 'TestRunnerStatsIncludesLastError|TestRunnerStatsLastErrorEmptyWhenHealthy' -v`
  预期：FAIL，编译错误 `sts[0].LastError undefined`（RunnerStat 尚无该字段）。
- [ ] **Step 2.3 实现**：

diag.go 末尾追加访问器（与 modeOnly 同一模式：轻量读取，不复制事件）：

```go
// lastErrorOnly 轻量读取最近一次错误文本与时间（watcher/健康快照用，不复制事件）。
func (d *runnerDiag) lastErrorOnly() (string, time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lastErr, d.lastErrAt
}
```

manager.go：RunnerStat 结构加字段（放在 QueueDepth 之后）：

```go
	// LastError 供健康推送文案与账户页展示最近失败原因；
	// 只反映本进程内的最近一次失败（重启清零），成功后不回填旧错误。
	LastError string `json:"last_error,omitempty"`
```

RunnerStats 填充处补一行：

```go
	for id, r := range m.runners {
		st, depth := r.stats()
		lastErr, _ := r.diag.lastErrorOnly()
		out = append(out, RunnerStat{
			AccountID:       id,
			Mode:            r.diag.modeOnly(),
			BreakerOpen:     st.Open,
			BreakerFailures: st.Failures,
			QueueDepth:      depth,
			LastError:       lastErr,
		})
	}
```

- [ ] **Step 2.4 运行确认通过**：`go test ./modules/email/sync/ -run 'TestRunnerStats' -v` 全绿，且 `go build ./...` 通过。
- [ ] **Step 2.5 gofmt + 提交**：

```bash
gofmt -w modules/email/sync/diag.go modules/email/sync/manager.go modules/email/sync/manager_test.go
git add modules/email/sync/diag.go modules/email/sync/manager.go modules/email/sync/manager_test.go
git commit -F - --only -- modules/email/sync/diag.go modules/email/sync/manager.go modules/email/sync/manager_test.go <<'MSG'
feat(flymail): RunnerStat 带出 last_error（账户健康推送的数据源）

MSG
```

---

### Task 3: health watcher 核心 + Manager 集成 + app.go 装配

**Files:**
- Create: `flymail/backend/modules/email/sync/health.go`
- Create: `flymail/backend/modules/email/sync/health_test.go`
- Modify: `flymail/backend/modules/email/sync/service.go`（镜像常量 notifyAccountStatus）
- Modify: `flymail/backend/internal/app/app.go`（装配注入）

**Interfaces:**
- Consumes: `RunnerStat`（Task 2）、`EmitFunc`（既有）
- Produces:
  - `AccountBrief{ID uint; Name string; Email string; Status string; Enabled bool}`
  - `SetHealthRuntime(fn func() ([]AccountBrief, error))`
  - `SetEmitterExternal(fn EmitFunc)`
  - watcher 内部：`newHealthWatcher(statuses, stats, emit, emitExt EmitFunc, now func() time.Time) *healthWatcher`、`(w) runOnce()`、`(w) loop(ctx)`
  - service.go 常量组新增镜像常量 `notifyAccountStatus = "account_status"`（与 notifySyncFailed 同组同注释风格）
  - manager.go 新增 `SetHealthRuntime(fn func() ([]AccountBrief, error))`、`SetEmitterExternal(fn EmitFunc)`（挂到 watcher）、`startHealthWatcher(ctx)`（Start 内调用）
  - app.go 装配后 watcher 自动随 Manager.Start 启动

**通知口径（spec 修订后，实现必须逐行对齐）**

| 场景 | 事件类型 | 落库 | 外发 |
|---|---|---|---|
| 熔断进入故障沿 | sync_failed | ✅ | ✅ |
| needs_reauth 进入故障沿 | （不发——markNeedsReauth 已即时推送并落库） | - | - |
| 24h 重复 | sync_failed | ❌ | ✅（emitExt，SkipInApp） |
| 恢复 | account_status | ✅ | ✅ |
| 重启补推（仅熔断） | sync_failed | ✅ | needs_reauth 存量不重复推送 |

> needs_reauth 沿为什么由 markNeedsReauth 独占：OAuth 失效被发现的那一刻
> markNeedsReauth 就推送了（account_status），watcher 最快 60 秒后才能观察到同一翻转，
> 再推一次是同一件事的两个推送。watcher 对 needs_reauth 只补 24h 重复与恢复——
> 通道未配置/漏看的情况下 24h 后仍会再提醒，信息不丢。

- [ ] **Step 3.1 写失败测试**（health_test.go，全量给出）：

```go
package sync

import (
	"errors"
	"testing"
	"time"
)

// captureEmit 捕获两类回调的事件，供逐用例断言。
type captureEmit struct {
	events []string // emit 捕获："type:accountID:title"
	exter  []string // emitExt 捕获，同格式
}

func (c *captureEmit) fns() (emit EmitFunc, emitExt EmitFunc) {
	return func(eventType string, accountID uint, _ uint, title, _ string) {
			c.events = append(c.events, eventType+":"+itoa(accountID)+":"+title)
		},
		func(eventType string, accountID uint, _ uint, title, _ string) {
			c.exter = append(c.exter, eventType+":"+itoa(accountID)+":"+title)
		}
}

func itoa(n uint) string { return fmt.Sprintf("%d", n) }

// mkWatcher 组装一个用例专用的 watcher。
// briefs/stats 为惰性求值：每次 runOnce 现取（模拟真实依赖的读库/读内存）。
func mkWatcher(briefs func() []AccountBrief, stats func() []RunnerStat, base time.Time) (*healthWatcher, *captureEmit, *func() time.Time) {
	cap := &captureEmit{}
	emit, emitExt := cap.fns()
	cur := base
	nowp := &cur
	w := newHealthWatcher(
		func() ([]AccountBrief, error) { return briefs(), nil },
		func() []RunnerStat { return stats() },
		emit, emitExt,
		func() time.Time { return *nowp },
	)
	return w, cap, nowp
}

func TestWatcherBreakerEntryEdge(t *testing.T) {
	base := time.Unix(1700000000, 0)
	mk := func() []AccountBrief {
		return []AccountBrief{{ID: 1, Name: "Gmail", Email: "a@x.com", Status: "ok", Enabled: true}}
	}
	mkStat := func() []RunnerStat {
		return []RunnerStat{{AccountID: 1, BreakerOpen: true, BreakerFailures: 5, LastError: "dial tcp: timeout"}}
	}
	w, cap, nowp := mkWatcher(mk, mkStat, base)
	w.runOnce()

	if len(cap.events) != 1 {
		t.Fatalf("熔断进入沿应发 1 条落库通知，got %d", len(cap.events))
	}
	if cap.events[0] != "sync_failed:1:同步异常：Gmail" {
		t.Fatalf("got %q", cap.events[0])
	}
	if len(cap.exter) != 0 {
		t.Fatalf("进入沿不应走只外发通道，got %v", cap.exter)
	}
	// 未满 24h 不重复
	*nowp = base.Add(2 * time.Hour)
	w.runOnce()
	if len(cap.events) != 1 || len(cap.exter) != 0 {
		t.Fatalf("24h 内不得重复推送: %v %v", cap.events, cap.exter)
	}

	// 满 24h：重复推送只走外发（SkipInApp 口径）
	*nowp = base.Add(25 * time.Hour)
	w.runOnce()
	if len(cap.events) != 1 {
		t.Fatalf("重复推送不得落库，events=%v", cap.events)
	}
	if len(cap.exter) != 1 {
		t.Fatalf("重复推送应走外发，got %v", cap.exter)
	}
}

func TestWatcherNeedsReauthEntrySilent(t *testing.T) {
	// needs_reauth 进入沿不推送：markNeedsReauth 已即时推送并落库（通知口径表）。
	base := time.Unix(1700000000, 0)
	mk := func() []AccountBrief {
		return []AccountBrief{{ID: 2, Name: "QQ", Email: "b@x.com", Status: "needs_reauth", Enabled: true}}
	}
	mkStat := func() []RunnerStat { return nil }
	w, cap, _ := mkWatcher(mk, mkStat, base)
	w.runOnce()

	if len(cap.events) != 0 || len(cap.exter) != 0 {
		t.Fatalf("needs_reauth 进入沿不得推送: events=%v exter=%v", cap.events, cap.exter)
	}
	// 但故障被记账：24h 后应有重复提醒
	*nowp = base.Add(25 * time.Hour)
	w.runOnce()
	if len(cap.exter) != 1 {
		t.Fatalf("24h 后应外发重复提醒，got %v", cap.exter)
	}
}

func TestWatcherRecoveryEdge(t *testing.T) {
	base := time.Unix(1700000000, 0)
	briefs := []AccountBrief{{ID: 1, Name: "Gmail", Email: "a@x.com", Status: "ok", Enabled: true}}
	mkBriefs := func() []AccountBrief { return briefs }
	statOpen := []RunnerStat{{AccountID: 1, BreakerOpen: true, BreakerFailures: 5}}
	statOK := []RunnerStat{{AccountID: 1, BreakerOpen: false, BreakerFailures: 0}}
	mkStat := func() []RunnerStat { return statOpen }
	w, cap, nowp := mkWatcher(mkBriefs, mkStat, base)

	w.runOnce() // 进入故障
	*nowp = base.Add(1 * time.Hour)

	// 恢复：熔断复位（直接替换注入的 stats 依赖）
	w.stats = func() []RunnerStat { return statOK }
	w.runOnce()

	if len(cap.events) != 2 {
		t.Fatalf("进入沿+恢复沿应共 2 条落库通知，got %v", cap.events)
	}
	if cap.events[1] != "account_status:1:同步已恢复：Gmail" {
		t.Fatalf("恢复沿 got %q", cap.events[1])
	}
}

func TestWatcherRestartReplay(t *testing.T) {
	// 重启补推：faults 初始为空，第一个 tick 观察到存量熔断故障 → 按新翻转沿补推。
	base := time.Unix(1700000000, 0)
	mk := func() []AccountBrief {
		return []AccountBrief{{ID: 1, Name: "Gmail", Email: "a@x.com", Status: "ok", Enabled: true}}
	}
	mkStat := func() []RunnerStat {
		return []RunnerStat{{AccountID: 1, BreakerOpen: true, BreakerFailures: 7, LastError: "x"}}
	}
	w, cap, _ := mkWatcher(mk, mkStat, base)
	w.runOnce()
	if len(cap.events) != 1 {
		t.Fatalf("重启后首个 tick 应补推熔断故障，got %v", cap.events)
	}
}

func TestWatcherDisabledRemovedSilently(t *testing.T) {
	// 故障期间账户被禁用：移出故障集合且不发恢复通知（SetEnabled 已有自己的通知）。
	base := time.Unix(1700000000, 0)
	on := []AccountBrief{{ID: 1, Name: "Gmail", Email: "a@x.com", Status: "ok", Enabled: true}}
	off := []AccountBrief{{ID: 1, Name: "Gmail", Email: "a@x.com", Status: "ok", Enabled: false}}
	mkBriefs := func() []AccountBrief { return on }
	mkStat := func() []RunnerStat {
		return []RunnerStat{{AccountID: 1, BreakerOpen: true, BreakerFailures: 5}}
	}
	w, cap, nowp := mkWatcher(mkBriefs, mkStat, base)
	w.runOnce() // 记账
	*nowp = base.Add(1 * time.Hour)
	w.statuses = func() ([]AccountBrief, error) { return off, nil }
	w.stats = func() []RunnerStat { return nil }
	w.runOnce() // 被禁用 → 移出，无任何推送

	if len(cap.events) != 1 || len(cap.exter) != 0 {
		t.Fatalf("禁用边界不得产生恢复通知: %v %v", cap.events, cap.exter)
	}
}

func TestWatcherPriorityNeedsReauth(t *testing.T) {
	// needs_reauth 优先于 breaker：同一账户两者同时成立时按 needs_reauth 记账与提醒。
	base := time.Unix(1700000000, 0)
	mk := func() []AccountBrief {
		return []AccountBrief{{ID: 3, Name: "G", Email: "c@x.com", Status: "needs_reauth", Enabled: true}}
	}
	mkStat := func() []RunnerStat {
		return []RunnerStat{{AccountID: 3, BreakerOpen: true, BreakerFailures: 5}}
	}
	w, cap, _ := mkWatcher(mk, mkStat, base)
	w.runOnce()

	if len(cap.events) != 0 || len(cap.exter) != 0 {
		t.Fatalf("needs_reauth 优先 → 进入沿静默: %v %v", cap.events, cap.exter)
	}
}

func TestWatcherStatusReadErrorNoFalseRecovery(t *testing.T) {
	// 读账户事实失败时不动故障集合：读库抖动不得被误判成「全部恢复」。
	base := time.Unix(1700000000, 0)
	okBriefs := []AccountBrief{{ID: 1, Name: "Gmail", Email: "a@x.com", Status: "ok", Enabled: true}}
	w, cap, _ := newHealthWatcher(
		func() ([]AccountBrief, error) { return okBriefs, nil },
		func() []RunnerStat { return []RunnerStat{{AccountID: 1, BreakerOpen: true, BreakerFailures: 5}} },
		func(string, uint, uint, string, string) { cap.events = append(cap.events, "x") },
		func(string, uint, uint, string, string) {},
		func() time.Time { return base },
	)
	w.runOnce() // 记账
	w.statuses = func() ([]AccountBrief, error) { return nil, errors.New("db busy") }
	w.runOnce() // 读取失败 → 跳过本轮
	if len(cap.events) != 1 {
		t.Fatalf("读取失败不得触发恢复推送: %v", cap.events)
	}
}
```

> `itoa` 若与包内既有 helper 撞名，改名为 `uidStr` 并同步测试内的引用。

- [ ] **Step 3.2 运行确认失败**：`go test ./modules/email/sync/ -run TestWatcher -v`
  预期：FAIL，编译错误 `undefined: newHealthWatcher`。
- [ ] **Step 3.3 实现 health.go**（全文给出）：

```go
package sync

import (
	"context"
	"fmt"
	gosync "sync"
	"time"
)

// 账户健康 watcher：差分账户故障态并推送通知。
//
// 故障判据的两半都在 sync 包内——熔断器是 runner 的内存态（RunnerStat），
// needs_reauth 是账户表的状态列（AccountBrief.Status）。差分出翻转沿与 24h 重复，
// 通知经注入的 emit / emitExt 出口发出去；依赖注入维持 sync 不 import account/notify
// 的既有格局（事件类型字符串镜像、AccountBrief 由 app 层从 account.Service 组装）。
//
// 通知口径（见 spec「站内落库口径」）：
//   - 熔断进入沿：emit（落库+外发）
//   - needs_reauth 进入沿：不发（markNeedsReauth 已即时推送并落库）
//   - 24h 重复：emitExt（只外发，不落库——防通知中心刷屏）
//   - 恢复：emit（落库+外发）
//   - 重启补推：faults 集合初始为空，第一个 tick 的差分天然实现；仅熔断会补推
//     （needs_reauth 沿本来就不由 watcher 发，存量自然不重复）。
const (
	healthTickInterval = time.Minute
	healthRepeatEvery  = 24 * time.Hour

	faultNeedsReauth = "needs_reauth"
	faultBreaker     = "breaker"
)

// accountStatusNeedsReauth 镜像 account.StatusNeedsReauth（避免反向依赖）。
const accountStatusNeedsReauth = "needs_reauth"

// AccountBrief 是 watcher 所需的账户事实快照。app 层从 account.Service.List 组装注入。
type AccountBrief struct {
	ID      uint
	Name    string
	Email   string
	Status  string
	Enabled bool
}

// healthFault 记录一个正在故障中的账户。
type healthFault struct {
	kind      string    // faultNeedsReauth / faultBreaker
	since     time.Time // 本进程内首次观察到故障的时刻（「已持续 N 天」的起点；重启后从观察点重计）
	lastNotif time.Time // 上次推送（进入沿或重复沿）的时刻
}

// healthWatcher 每 tick 差分一次故障态集合，翻转沿即时推送、持续故障每 24h 重复。
type healthWatcher struct {
	statuses func() ([]AccountBrief, error)
	stats    func() []RunnerStat
	emit     EmitFunc      // 落库+外发（熔断进入沿/恢复）
	emitExt  EmitFunc      // 只外发（24h 重复）
	now      func() time.Time
	repeat   time.Duration // 24h，测试可注入更短值

	mu     gosync.Mutex
	faults map[uint]healthFault
}

func newHealthWatcher(
	statuses func() ([]AccountBrief, error),
	stats func() []RunnerStat,
	emit, emitExt EmitFunc,
	now func() time.Time,
) *healthWatcher {
	return &healthWatcher{
		statuses: statuses,
		stats:    stats,
		emit:     emit,
		emitExt:  emitExt,
		now:      now,
		repeat:   healthRepeatEvery,
		faults:   map[uint]healthFault{},
	}
}

// loop 以 healthTickInterval 为周期反复 runOnce（Manager.Start 起协程调用）。
func (w *healthWatcher) loop(ctx context.Context) {
	w.runOnce() // 启动即查一次：重启补推
	t := time.NewTicker(healthTickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.runOnce()
		}
	}
}

// runOnce 差分一轮。单次失败不进熔断（breaker 需要 5 次连续失败）、
// needs_reauth 是终态，因此这里没有额外防抖。
func (w *healthWatcher) runOnce() {
	briefs, err := w.statuses()
	if err != nil {
		// 读账户事实失败时保守处理：不动故障集合。读库抖动若被误判成
		// 「全部恢复」，用户会收到成串的假恢复通知。
		return
	}

	byStat := map[uint]RunnerStat{}
	for _, st := range w.stats() {
		byStat[st.AccountID] = st
	}
	now := w.now()

	w.mu.Lock()
	defer w.mu.Unlock()

	for _, b := range briefs {
		if !b.Enabled {
			// 故障期间被禁用：移出故障集合且不发恢复通知（SetEnabled 已有自己的通知）。
			delete(w.faults, b.ID)
			continue
		}
		// 故障判据：needs_reauth（DB 终态）优先于 breaker（内存态，5 次连续失败天然防抖）。
		kind := ""
		if b.Status == accountStatusNeedsReauth {
			kind = faultNeedsReauth
		} else if st, ok := byStat[b.ID]; ok && st.BreakerOpen {
			kind = faultBreaker
		}
		if kind == "" {
			// 健康。之前在故障中 → 恢复沿。
			if f, was := w.faults[b.ID]; was {
				delete(w.faults, b.ID)
				w.emit(notifyAccountStatus, b.ID, 0,
					"同步已恢复："+b.Name,
					fmt.Sprintf("账户 %s 恢复正常同步。", b.Email))
			}
			continue
		}
		if f, was := w.faults[b.ID]; was {
			// 已在故障中：只在满 repeat 后发一次只外发的重复提醒。
			if now.Sub(f.lastNotif) >= w.repeat {
				w.emitExt(notifySyncFailed, b.ID, 0,
					"同步异常持续："+b.Name,
					fmt.Sprintf("已持续 %d 天无法同步——%s",
						int(now.Sub(f.since).Hours()/24), w.reason(kind, byStat[b.ID])))
				f.lastNotif = now
				w.faults[b.ID] = f
			}
			continue
		}
		// 新故障（含重启后的存量发现）。
		f := healthFault{kind: kind, since: now, lastNotif: now}
		if kind == faultBreaker {
			// 熔断进入沿：落库+外发。
			w.emit(notifySyncFailed, b.ID, 0,
				"同步异常："+b.Name,
				fmt.Sprintf("连续 %d 次失败——%s", byStat[b.ID].BreakerFailures, w.reason(kind, byStat[b.ID])))
		}
		// needs_reauth 进入沿：不发（markNeedsReauth 已即时推送并落库）。
		w.faults[b.ID] = f
	}

	// briefs 未覆盖的存量故障（runner 被外部移除等）：保持记账不动，等下一个周期自然消化；
	// 不主动清除——误删会让重复提醒与恢复判断失真。
}

// reason 返回给用户的故障原因文本（重复提醒与进入沿共用，保证口径一致）。
func (w *healthWatcher) reason(kind string, st RunnerStat) string {
	if kind == faultNeedsReauth {
		return "授权已失效，请重新授权账户"
	}
	if st.LastError != "" {
		return st.LastError
	}
	return "连接邮箱服务器持续失败"
}
```

- [ ] **Step 3.4 实现 Manager 集成**（manager.go）：

Manager struct 在 `wb *wbStore` 字段后追加：

```go
	// health 账户健康 watcher（SetHealthRuntime 装配后才存在；nil 时 Start 跳过）。
	health *healthWatcher
```

Stop 方法之后新增：

```go
// SetHealthRuntime 注入账户事实来源（app 层从 account.Service 组装，见 AccountBrief）。
// 必须在 Start 之前调用。
func (m *Manager) SetHealthRuntime(fn func() ([]AccountBrief, error)) {
	m.health = newHealthWatcher(fn, m.RunnerStats, m.emit, m.emitExt)
}

// SetEmitterExternal 注入「只外发不落站内」的通知回调（24h 重复提醒走这条通道）。
// 须在 SetHealthRuntime 之后调用（先建 watcher 再挂回调）。
func (m *Manager) SetEmitterExternal(fn EmitFunc) {
	if m.health != nil {
		m.health.emitExt = fn
	}
}
```

Start 方法里 `go m.reconcileLoop(ctx)` 之后追加：

```go
	if m.health != nil {
		go m.health.loop(ctx)
	}
```

装配顺序约束（写进代码注释）：`SetEmitter → SetHealthRuntime → SetEmitterExternal`，
与 app.go 的调用顺序一致；不满足时 emit/emitExt 为 nil 会导致推送丢失。

- [ ] **Step 3.5 app.go 装配**（`manager.SetEmitter(emit)` 之后插入）：

```go
	// 账户健康 watcher（M17）：差分故障态并推送通知。
	// 账户事实（名称/状态/启停）来自 account.Service；通知出口与其它事件源共享同一个 emit。
	manager.SetHealthRuntime(func() ([]syncmod.AccountBrief, error) {
		list, err := accountSvc.List()
		if err != nil {
			return nil, err
		}
		briefs := make([]syncmod.AccountBrief, 0, len(list))
		for _, a := range list {
			briefs = append(briefs, syncmod.AccountBrief{
				ID:      a.ID,
				Name:    a.Name,
				Email:   a.Email,
				Status:  a.Status,
				Enabled: a.Enabled,
			})
		}
		return briefs, nil
	})
	manager.SetEmitterExternal(notifySvc.EmitExternalFunc())
```

- [ ] **Step 3.6 运行确认通过**：
  - `go test ./modules/email/sync/ -run TestWatcher -v` 全绿
  - `go test ./modules/email/sync/` 全量绿（无回归）
  - `go build ./...` 通过
- [ ] **Step 3.7 gofmt + 提交**：

```bash
gofmt -w modules/email/sync/health.go modules/email/sync/health_test.go modules/email/sync/manager.go modules/email/sync/service.go modules/email/sync/manager_test.go internal/app/app.go
git add modules/email/sync/health.go modules/email/sync/health_test.go modules/email/sync/manager.go modules/email/sync/service.go modules/email/sync/manager_test.go internal/app/app.go
git commit -F - --only -- modules/email/sync/health.go modules/email/sync/health_test.go modules/email/sync/manager.go modules/email/sync/service.go internal/app/app.go <<'MSG'
feat(flymail): 账户健康 watcher——差分故障态推送通知

熔断进入沿推 sync_failed（落库+外发）；needs_reauth 沿由 markNeedsReauth
独占，watcher 只做 24h 重复（只外发）与恢复通知。重启后首个 tick 天然补推
存量熔断故障；账户被禁用时静默移出。app 层注入账户事实与只外发出口。
MSG
```

---

### Task 4: runner needs_reauth 30 分钟节流（停止无效重试）

**Files:**
- Modify: `flymail/backend/modules/email/sync/runner.go`
- Modify: `flymail/backend/modules/email/sync/diag.go`（mode 常量）
- Modify: `flymail/frontend/src/locales/zh.json` + `en.json`（monitoring.mode.needs_reauth 键）
- Test: `flymail/backend/modules/email/sync/runner_test.go`（追加）

**Interfaces:**
- Consumes: `account.ErrNeedsReauth`（既有 sentinel；sync → account 依赖方向已核实安全）
- Produces: runner 字段 `needsReauthUntil time.Time`（仅 runner goroutine 访问，同 dialBackoff）、
  哨兵错误 `errNeedsReauthThrottled`、diag 模式 `modeNeedsReauth = "needs_reauth"`

**设计要点**
- ErrNeedsReauth 的暴露点在 `r.cfg()`（IMAPConfig→AccessToken 刷新失败），
  经 `dialConn` 返回，落在 doPoll 与 exec 两处拨号失败分支。
- 节流只挡**后台**路径：doPoll 直接跳过本轮并置 mode=needs_reauth；exec 后台任务
  回 errNeedsReauthThrottled（回写任务留在持久化队列，后续重试）。
- 前台穿透（用户手动同步/详情不受节流）——重新授权后点一次同步立即恢复的闭环。
- 清除点：`onDialSuccess`（连接成功⇒授权必然已恢复），前台/后台/轮询三条路径汇合处。

- [ ] **Step 4.1 写失败测试**（runner_test.go 追加）：

```go
func TestRunnerNeedsReauthThrottle(t *testing.T) {
	dials := 0
	now := time.Unix(1700000000, 0)
	// 拨号一律以 ErrNeedsReauth 失败（模拟 OAuth 刷新失败暴露在 r.cfg()→dialConn）。
	r := newRunner(1,
		func() (types.IMAPConfig, error) { return types.IMAPConfig{}, nil },
		func(types.IMAPConfig) (Session, error) {
			dials++
			return nil, fmt.Errorf("refresh: %w", account.ErrNeedsReauth)
		},
	)
	r.now = func() time.Time { return now }
	sess := Session(nil)
	mk := func() task {
		return task{run: func(Session) error { return nil }, reply: make(chan error, 1)}
	}

	// 1) 首个后台任务：拨号失败原因为 ErrNeedsReauth → 进入节流 + 退避
	r.exec(&sess, mk(), false)
	if r.needsReauthUntil.IsZero() {
		t.Fatal("ErrNeedsReauth 后应进入节流")
	}
	if dials != 1 {
		t.Fatalf("dials=%d", dials)
	}

	// 2) 拨号退避（1s）已过、节流（30min）未过：后台任务仍被拒绝——
	//    证明节流独立于既有 dial backoff 生效，30 分钟内不再拨号。
	now = now.Add(2 * time.Second)
	tk := mk()
	r.exec(&sess, tk, false)
	if err := <-tk.reply; !errors.Is(err, errNeedsReauthThrottled) {
		t.Fatalf("节流窗口内的后台任务应回 errNeedsReauthThrottled，got %v", err)
	}
	if dials != 1 {
		t.Fatalf("节流期间不得再拨号，dials=%d", dials)
	}

	// 3) 前台任务穿透节流：仍然拨号（用户手动操作不受 30 分钟限制）
	tkf := mk()
	r.exec(&sess, tkf, true)
	if dials != 2 {
		t.Fatalf("前台应穿透节流，dials=%d", dials)
	}

	// 4) 拨号成功清除节流（重新授权后自动恢复的闭环）
	r.dial = func(types.IMAPConfig) (Session, error) {
		dials++
		return &mgrFakeSession{}, nil
	}
	tks := mk()
	r.exec(&sess, tks, true)
	if err := <-tks.reply; err != nil {
		t.Fatalf("成功路径不应报错，got %v", err)
	}
	if !r.needsReauthUntil.IsZero() {
		t.Fatal("拨号成功后应清除节流")
	}
}
```

> import 变更：runner_test.go 追加 `"fmt"` 与 `"flymail/modules/email/account"`。

- [ ] **Step 4.2 运行确认失败**：`go test ./modules/email/sync/ -run TestRunnerNeedsReauthThrottle -v`
  预期：FAIL，编译错误 `undefined: errNeedsReauthThrottled`。
- [ ] **Step 4.3 实现**（runner.go）：

字段（`staleAfter time.Duration` 之后）：

```go
	// needsReauthUntil 授权失效后的后台拨号节流截止时刻（仅 runner goroutine 访问，同 dialBackoff）。
	// 拨号失败且 errors.Is(err, account.ErrNeedsReauth) 时置 now+30min；
	// onDialSuccess（连接成功⇒授权已恢复）清除。零值表示未节流。
	needsReauthUntil time.Time
```

常量与哨兵（needsReauthBackoff 放文件头常量区，与 idleCloseInterval 等同区）：

```go
// needsReauthBackoff 授权失效后后台拨号的节流时长：30 分钟内不再无意义地重试。
const needsReauthBackoff = 30 * time.Minute

// errNeedsReauthThrottled 回给被节流拒绝的后台任务（回写任务留在持久化队列，到期重试）。
var errNeedsReauthThrottled = errors.New("needs reauth: background throttled 30min")
```

doPoll：breaker 门控之后、ensureAlive 之前插入：

```go
	if !r.needsReauthUntil.IsZero() && r.now().Before(r.needsReauthUntil) {
		r.diag.setMode(modeNeedsReauth)
		return interval
	}
```

exec：后台门控组（breaker、dial backoff）追加第三道：

```go
		if !r.needsReauthUntil.IsZero() && r.now().Before(r.needsReauthUntil) {
			reply(t, errNeedsReauthThrottled)
			return
		}
```

两处拨号失败分支（doPoll 的 `r.dialConn()` err 分支与 exec 的 dialConn err 分支），
在 `r.onDialFailure()` 之后各追加一行：

```go
			r.markNeedsReauthThrottle(err)
```

新 helper（onDialFailure 附近）：

```go
// markNeedsReauthThrottle 拨号失败原因为授权失效时，把后台拨号节流 30 分钟，
// 并记一条诊断事件（监控页时间线可见）。
func (r *runner) markNeedsReauthThrottle(err error) {
	if !errors.Is(err, account.ErrNeedsReauth) {
		return
	}
	r.needsReauthUntil = r.now().Add(needsReauthBackoff)
	r.diag.event("needs_reauth", "授权已失效，后台暂停拨号 30 分钟")
}
```

onDialSuccess 末尾追加清除逻辑：

```go
	if !r.needsReauthUntil.IsZero() {
		r.needsReauthUntil = time.Time{}
		r.diag.event("needs_reauth_cleared", "连接成功，解除授权节流")
	}
```

diag.go 的 mode 常量组追加：

```go
	modeNeedsReauth = "needs_reauth" // 授权失效、后台拨号节流中
```

- [ ] **Step 4.4 运行确认通过**：`go test ./modules/email/sync/ -run TestRunnerNeedsReauthThrottle -v` 绿；
  `go build ./...` 通过。
- [ ] **Step 4.5 i18n**：zh.json 与 en.json 的 `settings.monitoring.mode` 下各加
  `"needs_reauth": "需重新授权"` / `"needs_reauth": "Re-auth needed"`。
- [ ] **Step 4.6 gofmt + 提交**：

```bash
gofmt -w modules/email/sync/runner.go modules/email/sync/diag.go modules/email/sync/runner_test.go
git add modules/email/sync/runner.go modules/email/sync/diag.go modules/email/sync/runner_test.go ../frontend/src/locales/zh.json ../frontend/src/locales/en.json
git commit -F - --only -- modules/email/sync/runner.go modules/email/sync/diag.go modules/email/sync/runner_test.go ../frontend/src/locales/zh.json ../frontend/src/locales/en.json <<'MSG'
feat(flymail): 授权失效后后台拨号节流 30 分钟

OAuth 账户授权失效（ErrNeedsReauth）后，后台路径 30 分钟内不再无意义重试；
前台操作穿透节流，拨号成功即解除。监控页模式显示「需重新授权」。
MSG
```

---

### Task 5: AccountResponse.health 注入链（sync → app → account → 前端）

**Files:**
- Modify: `flymail/backend/modules/email/sync/health.go`（新增 RunnerHealth + HealthFor）
- Modify: `flymail/backend/modules/email/account/dto.go`（AccountHealthInfo + AccountResponse.Health）
- Modify: `flymail/backend/modules/email/account/service.go`（health 字段 + SetHealthProvider + withHealth）
- Modify: `flymail/backend/internal/app/app.go`（SetHealthProvider 装配）
- Test: `flymail/backend/modules/email/account/service_test.go`（追加，包 account_test）

**Interfaces:**
- Consumes: `Manager.runners` map、`runner.stats()`（既有）
- Produces:
  - sync: `RunnerHealth{BreakerOpen bool; Failures int; LastError string}`、`(m *Manager) HealthFor(accountID uint) (RunnerHealth, bool)`
  - account: `AccountHealthInfo{NeedsReauth bool; BreakerOpen bool; Failures int; LastError string}`（json: needs_reauth/breaker_open/failures/last_error）
  - account: `SetHealthProvider(fn func(a *Account) *AccountHealthInfo)`、`withHealth(a *Account, resp AccountResponse) AccountResponse`
  - API 形状：`GET /accounts` 每项带 `"health": {"needs_reauth": false, "breaker_open": true, "failures": 5, "last_error": "..."}`；健康账户该字段缺省

- [ ] **Step 5.1 写失败测试**（service_test.go 追加，注意测试在包 account_test 中）：

```go
func TestServiceHealthProviderInjection(t *testing.T) {
	svc, _, _ := newSvc(t)
	created, err := svc.Create(account.CreateAccountRequest{
		Name: "T", Email: "t@x.com", Username: "t", Password: "pw",
		IMAPHost: "127.0.0.1", IMAPPort: 993, IMAPSecurity: "tls",
		SMTPHost: "127.0.0.1", SMTPPort: 587, SMTPSecurity: "starttls",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Health != nil {
		t.Fatal("未注入 provider 时 Health 应缺省")
	}

	svc.SetHealthProvider(func(a *account.Account) *account.AccountHealthInfo {
		if a.ID == created.ID {
			return &account.AccountHealthInfo{
				NeedsReauth: false, BreakerOpen: true, Failures: 5, LastError: "dial tcp: timeout",
			}
		}
		return nil
	})

	got, err := svc.Get(created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Health == nil || !got.Health.BreakerOpen || got.Health.Failures != 5 {
		t.Fatalf("Get 未带出健康快照: %+v", got.Health)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var found *account.AccountHealthInfo
	for i := range list {
		if list[i].ID == created.ID {
			found = list[i].Health
		}
	}
	if found == nil {
		t.Fatal("List 未带出健康快照")
	}

	// provider 返回 nil（健康）→ health 字段缺省
	svc.SetHealthProvider(func(a *account.Account) *account.AccountHealthInfo { return nil })
	got2, _ := svc.Get(created.ID)
	if got2.Health != nil {
		t.Fatalf("健康账户 health 应缺省，got %+v", got2.Health)
	}
}
```

- [ ] **Step 5.2 运行确认失败**：`go test ./modules/email/account/ -run TestServiceHealthProviderInjection -v`
  预期：FAIL，编译错误 `created.Health undefined`。
- [ ] **Step 5.3 实现 dto.go**（新增类型 + 字段）：

```go
// AccountHealthInfo 是账户运行时健康快照。
// nil/缺省表示健康，非 nil 即有故障或需授权（前端据此渲染标记）。
type AccountHealthInfo struct {
	NeedsReauth bool   `json:"needs_reauth"`
	BreakerOpen bool   `json:"breaker_open"`
	Failures    int    `json:"failures"`
	LastError   string `json:"last_error,omitempty"`
}
```

AccountResponse 末尾（LastSyncAt 之后）追加：

```go
	// Health 运行时健康快照；健康账户为 nil（json 缺省）。
	Health *AccountHealthInfo `json:"health,omitempty"`
```

- [ ] **Step 5.4 实现 service.go**：

Service struct 的 emit 字段后追加：

```go
	// health 运行时健康快照来源（app 层从同步引擎组装注入；nil 时响应不带 health）。
	health func(a *Account) *AccountHealthInfo
```

SetEmitter 之后新增：

```go
// SetHealthProvider 注入健康快照来源。fn 收到 *Account（可读 Status，便于
// app 层把 needs_reauth 一并装进快照），返回 nil 表示健康。
func (s *Service) SetHealthProvider(fn func(a *Account) *AccountHealthInfo) { s.health = fn }
```

- [ ] **Step 5.5 withHealth helper + 4 个调用点改造**：

```go
// withHealth 给响应补健康快照；provider 未注入时原样返回（单测/无 Manager 场景）。
func (s *Service) withHealth(a *Account, resp AccountResponse) AccountResponse {
	if s.health != nil {
		resp.Health = s.health(a)
	}
	return resp
}
```

四处 `resp := toResponse(a)` 与 List 循环改为：

```go
	resp := s.withHealth(a, toResponse(a))
```

```go
	out = append(out, s.withHealth(&list[i], toResponse(&list[i])))
```

- [ ] **Step 5.6 实现 sync 侧 HealthFor**（health.go 追加）——先写测试（manager_test.go，构造同 Task 2）：

```go
func TestHealthForMirrorsRunnerState(t *testing.T) {
	m := &Manager{runners: map[uint]*runner{
		7: newRunner(7,
			func(uint) (types.IMAPConfig, error) { return types.IMAPConfig{}, nil },
			nil,
		),
	}}
	m.runners[7].diag.markErr("dial tcp: refused")

	rh, ok := m.HealthFor(7)
	if !ok {
		t.Fatalf("7 号 runner 存在，HealthFor 不应返回 false")
	}
	if rh.BreakerOpen || rh.Failures != 0 {
		t.Fatalf("裸 runner 未熔断: %+v", rh)
	}
	if rh.LastError != "dial tcp: refused" {
		t.Fatalf("LastError = %q", rh.LastError)
	}

	// 无 runner：ok=false（app 层据此在健康时返回 nil）
	if _, ok := m.HealthFor(99); ok {
		t.Fatal("无 runner 应返回 false")
	}
}
```

```go
// RunnerHealth 是账户运行时健康快照（账户 API 的 health 字段来源）。
type RunnerHealth struct {
	BreakerOpen bool
	Failures    int
	LastError   string
}

// HealthFor 返回某账户的运行时健康快照；无 runner 时返回 false（app 层组合 DB 状态后注入 account 层）。
func (m *Manager) HealthFor(accountID uint) (RunnerHealth, bool) {
	m.mu.Lock()
	r, ok := m.runners[accountID]
	m.mu.Unlock()
	if !ok {
		return RunnerHealth{}, false
	}
	st, _ := r.stats()
	lastErr, _ := r.diag.lastErrorOnly()
	return RunnerHealth{
		BreakerOpen: st.Open,
		Failures:    st.Failures,
		LastError:   lastErr,
	}, true
}
```

- [ ] **Step 5.7 app.go 装配**（Task 3 装配代码之后插入）：

```go
	// 账户健康快照注入账户 API（M17）：熔断/错误来自同步引擎内存态，
	// needs_reauth 来自账户表状态列。健康账户返回 nil（响应缺省 health）。
	accountSvc.SetHealthProvider(func(a *account.Account) *account.AccountHealthInfo {
		rh, ok := manager.HealthFor(a.ID)
		if !ok && a.Status != account.StatusNeedsReauth {
			return nil
		}
		h := &account.AccountHealthInfo{}
		if a.Status == account.StatusNeedsReauth {
			h.NeedsReauth = true
		}
		if ok {
			h.BreakerOpen = rh.BreakerOpen
			h.Failures = rh.Failures
			h.LastError = rh.LastError
		}
		if !h.NeedsReauth && !h.BreakerOpen {
			return nil
		}
		return h
	})
```

- [ ] **Step 5.8 运行确认通过**：`go test ./modules/email/sync/ -run TestHealthForMirrorsRunnerState -v` 绿；
  `go test ./modules/email/account/ -run TestServiceHealthProviderInjection -v` 绿；
  `go test ./modules/email/sync/ ./modules/email/account/` 全量绿；`go build ./...` 通过。
- [ ] **Step 5.9 gofmt + 提交**：

```bash
gofmt -w modules/email/account/dto.go modules/email/account/service.go internal/app/app.go
git add modules/email/sync/health.go modules/email/account/dto.go modules/email/account/service.go internal/app/app.go modules/email/account/service_test.go
git commit -F - --only -- modules/email/sync/health.go modules/email/account/dto.go modules/email/account/service.go internal/app/app.go modules/email/account/service_test.go <<'MSG'
feat(flymail): /accounts 响应注入运行时健康快照 health 字段

熔断/最近错误来自同步引擎内存态，needs_reauth 来自账户表状态列；
健康账户响应缺省该字段。前端侧栏/横幅/账户页据此渲染故障标记。
MSG
```

---

### Task 6: 前端健康类型 + 侧栏账户行故障标记

**Files:**
- Create: `flymail/frontend/src/lib/account-health.ts`
- Modify: `flymail/frontend/src/lib/types.ts`（Account 加 health 字段）
- Modify: `flymail/frontend/src/components/mail/AccountSidebar.tsx`（账户行渲染）
- Modify: `flymail/frontend/src/index.css`（.acct-health 样式）
- Modify: `flymail/frontend/src/locales/zh.json` + `en.json`（account.health.* 键）
- Test: `flymail/frontend/src/lib/account-health.test.ts`

**与 spec 的偏离（有意）**：spec 侧栏设计写的是 lucide 图标（KeyRound/AlertTriangle），
但 `Icon.tsx` 的 `IconName` 是封闭联合类型（约 40 个名称，无 KeyRound/AlertTriangle）。
为避免本里程碑扩白名单，侧栏用「色边小徽标+文字」承载故障信息；文字与配色语义与 spec
一致（needs_reauth 橙/熔断红）。若后续要图标，扩 `IconName` 与 SVG 路径即可。i18n 同理：spec 建议复用 `settings.monitoring.*` 键，但侧栏徽标需要更短文案与悬停提示（tip），故新增 `account.health.*` 命名空间；账户页（Task 8）则复用既有 `settings.account.syncError`。

**Interfaces:**
- Consumes: `Account.health`（Task 5 API 形状）、侧栏账户行渲染点（acct-name 之后）
- Produces: `healthKind(a): 'needs_reauth' | 'breaker' | null`（needs_reauth 优先）；后续横幅/账户页/实时同步 hooks 复用

- [ ] **Step 6.1 写失败测试**（account-health.test.ts）：

```ts
import { describe, it, expect } from 'vitest'
import { healthKind } from '@/lib/account-health'

describe('healthKind', () => {
  it('needs_reauth 优先于 breaker（与后端口径一致）', () => {
    expect(healthKind({ status: 'needs_reauth', health: { needs_reauth: true, breaker_open: true, failures: 5 } })).toBe('needs_reauth')
  })
  it('仅熔断返回 breaker', () => {
    expect(healthKind({ status: 'ok', health: { needs_reauth: false, breaker_open: true, failures: 5 } })).toBe('breaker')
  })
  it('健康账户返回 null', () => {
    expect(healthKind({ status: 'ok', health: undefined })).toBe(null)
    expect(healthKind({ status: 'ok', health: { needs_reauth: false, breaker_open: false, failures: 0 } })).toBe(null)
  })
  it('health 字段缺省但 status 为 needs_reauth 时仍判定（兼容旧缓存数据）', () => {
    expect(healthKind({ status: 'needs_reauth', health: undefined })).toBe('needs_reauth')
  })
})
```

- [ ] **Step 6.2 运行确认失败**：`cd ../frontend && pnpm vitest run src/lib/account-health.test.ts`
  预期：FAIL（模块不存在）。

- [ ] **Step 6.3 实现 account-health.ts**（新建，判定口径的唯一出处，横幅/账户页复用）：

```ts
import type { Account } from './types'

export type HealthKind = 'needs_reauth' | 'breaker'

/** 账户故障判定：needs_reauth 优先于 breaker（与后端 watcher 口径一致）。
 * health 字段缺省时回退 status 判定（兼容旧缓存数据）。 */
export function healthKind(a: Pick<Account, 'status' | 'health'>): HealthKind | null {
  if (a.health?.needs_reauth || a.status === 'needs_reauth') return 'needs_reauth'
  if (a.health?.breaker_open) return 'breaker'
  return null
}
```

- [ ] **Step 6.4 types.ts 加 Account 接口**：在 `Account` 接口定义处（含 `status` 字段的对象）追加：

```ts
/** 运行时健康快照；健康账户 API 缺省该字段（M17）。 */
export interface AccountHealth {
  needs_reauth: boolean
  breaker_open: boolean
  failures: number
  last_error?: string
}
```

`Account` 接口内 `status` 字段之后追加：

```ts
  health?: AccountHealth
```

- [ ] **Step 6.5 运行确认通过**：`pnpm vitest run src/lib/account-health.test.ts` 预期 4 个用例 PASS。

- [ ] **Step 6.6 AccountSidebar 账户行渲染**：账户按钮行内 `acct-name` span 之后（未读徽标 `acct-unread` 之前）追加：

```tsx
                  {(() => {
                    const hk = healthKind(acc)
                    if (!hk) return null
                    const tip =
                      hk === 'needs_reauth'
                        ? t('account.health.needsReauthTip')
                        : `${t('account.health.breakerTip')}${acc.health?.last_error ?? ''}`
                    return (
                      <span
                        className={'acct-health' + (hk === 'needs_reauth' ? ' is-reauth' : ' is-breaker')}
                        title={tip}
                      >
                        {hk === 'needs_reauth' ? t('account.health.needsReauth') : t('account.health.breaker')}
                      </span>
                    )
                  })()}
```

import 区追加 `import { healthKind } from '@/lib/account-health'`。
disabled 行置灰：账户按钮 style 上叠加（故障标记不受影响，可并存）：

```tsx
style={{ flex: 1, ...(acc.enabled ? undefined : { opacity: 0.55 }) }}
```

（若该按钮已有 style 属性则合并，不要覆盖既有键。）

- [ ] **Step 6.7 index.css 追加样式**（`.acct-health` 系列与侧栏既有徽标视觉同族）：

```css
/* M17 账户健康标记：侧栏账户行的故障徽标（needs_reauth 橙 / 熔断红） */
.acct-health {
  font-size: 10px;
  line-height: 1;
  padding: 2px 5px;
  border-radius: 9999px;
  border: 1px solid;
  white-space: nowrap;
  flex-shrink: 0;
}
.acct-health.is-reauth {
  color: #d97706;
  border-color: #d97706;
  background: color-mix(in srgb, #d97706 12%, transparent);
}
.acct-health.is-breaker {
  color: var(--destructive);
  border-color: var(--destructive);
  background: color-mix(in srgb, var(--destructive) 12%, transparent);
}
```

- [ ] **Step 6.8 i18n 键**：`zh.json` 的 `account` 节内追加：

```json
    "health": {
      "needsReauth": "需重新授权",
      "breaker": "同步异常",
      "needsReauthTip": "授权已失效，点击账户进入设置页重新授权",
      "breakerTip": "连续失败已熔断："
    },
```

`en.json` 对应位置追加：

```json
    "health": {
      "needsReauth": "Reauth needed",
      "breaker": "Sync failing",
      "needsReauthTip": "Authorization expired. Open the account in settings to re-auth",
      "breakerTip": "Circuit breaker open: "
    },

      "needsReauthTip": "Authorization expired. Open the account in settings to re-auth",
      "breakerTip": "Circuit breaker open: "
    },
```

- [ ] **Step 6.9 验证**：`pnpm vitest run src/lib/account-health.test.ts` 绿；`pnpm tsc -b` 无错（solution 式 tsconfig 必须用 `tsc -b`，`--noEmit` 是空检查）；`pnpm build` 通过。
- [ ] **Step 6.10 提交**：

```bash
git add frontend/src/lib/account-health.ts frontend/src/lib/account-health.test.ts frontend/src/lib/types.ts frontend/src/components/mail/AccountSidebar.tsx frontend/src/index.css frontend/src/locales/zh.json frontend/src/locales/en.json
git commit -F - --only -- frontend/src/lib/account-health.ts frontend/src/lib/account-health.test.ts frontend/src/lib/types.ts frontend/src/components/mail/AccountSidebar.tsx frontend/src/index.css frontend/src/locales/zh.json frontend/src/locales/en.json <<'MSG'
feat(flymail): 侧栏账户行显示健康故障标记（M17）

needs_reauth 橙标/熔断红标，title 提示最近错误；healthKind 判定口径
集中一处，横幅/账户页/实时同步 hooks 复用。disabled 行置灰可叠加。
MSG
```

---

### Task 7: 全局健康横幅 HealthBanner（出现/点名/关闭记忆/恢复清理）

**Files:**
- Modify: `flymail/frontend/src/lib/account-health.ts`（追加 dismiss 记忆 helper）
- Create: `flymail/frontend/src/components/mail/HealthBanner.tsx`
- Modify: `flymail/frontend/src/pages/Shell.tsx`（conn-banner 前挂载）
- Modify: `flymail/frontend/src/index.css`（.health-banner 样式）
- Modify: `flymail/frontend/src/locales/zh.json` + `en.json`（account.healthBanner.* 键）
- Test: `flymail/frontend/src/lib/account-health.test.ts`（追加用例）+ `flymail/frontend/src/lib/health-banner.test.tsx`（组件测试，仿 account-reorder.test.tsx 挂载方式）

**Interfaces:**
- Consumes: `healthKind(a)`（Task 6）、`useAccounts()`（queryKey `['accounts']`，Task 5 的 health 字段随查询缓存更新——Task 9 会把 SSE 事件接到该缓存失效）、`requestOpenSettings(page)`（`@/lib/settings-nav`）
- Produces: `loadDismissed(): Set<number>` / `saveDismissed(s: Set<number>): void` / `pruneDismissed(dismissed: Set<number>, faultyIds: Set<number>): Set<number>`；`<HealthBanner />`（无 props）

**设计口径（spec §UI-横幅）**：点名最多 2 个账户名，其余并作「等 N 个」；「知道了」按账户 id 记忆在 sessionStorage（本会话内不再弹）；账户恢复后从记忆里清掉——同会话再出故障会重现。

- [ ] **Step 7.1 写失败测试**。lib 层（追加到 account-health.test.ts）：

```ts
import { loadDismissed, saveDismissed, pruneDismissed } from '@/lib/account-health'

describe('关闭记忆', () => {
  beforeEach(() => sessionStorage.clear())

  it('load 空存储返回空集合', () => {
    expect(loadDismissed().size).toBe(0)
  })

  it('save 后 load 往返一致', () => {
    const s = new Set([1, 3])
    saveDismissed(s)
    expect(loadDismissed()).toEqual(new Set([1, 3]))
  })

  it('损坏的存储内容不抛错，回退空集合', () => {
    sessionStorage.setItem('flymail-health-dismissed', '{bad json')
    expect(loadDismissed().size).toBe(0)
  })

  it('恢复后从记忆清除：同会话再故障可重现横幅', () => {
    const dismissed = new Set([1, 2])
    const next = pruneDismissed(dismissed, new Set([2]))
    expect(next.has(1)).toBe(false)
    expect(next.has(2)).toBe(true)
  })
})
```

（`beforeEach` 若文件顶部未 import 则从 vitest 补。）

组件测试（health-banner.test.tsx，仿 account-reorder.test.tsx 的 createRoot+act 挂载；i18n 恒等 t）：

```tsx
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { HealthBanner } from '@/components/mail/HealthBanner'
import type { Account } from '@/lib/types'

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (k: string) => k }) }))
;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function acct(id: number, name: string, health?: Account['health']): Account {
  return {
    id, name, email: `${name}@example.com`, auth_type: 'password',
    imap_host: 'i', imap_port: 993, imap_security: 'ssl',
    smtp_host: 's', smtp_port: 465, smtp_security: 'ssl',
    status: health?.needs_reauth ? 'needs_reauth' : 'ok', enabled: true,
    health,
  }
}

describe('HealthBanner', () => {
  let qc: QueryClient
  let container: HTMLDivElement
  let root: Root

  async function renderWith(accounts: Account[]) {
    qc.setQueryData(['accounts'], accounts)
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <HealthBanner />
        </QueryClientProvider>,
      )
    })
  }

  /** 换一份数据重渲染（模拟 ['accounts'] 缓存被 SSE invalidate 后的刷新） */
  async function rerenderWith(accounts: Account[]) {
    qc.setQueryData(['accounts'], accounts)
    await act(async () => {
      root.render(
        <QueryClientProvider client={qc}>
          <HealthBanner />
        </QueryClientProvider>,
      )
    })
  }

  function banner(): HTMLElement | null {
    return container.querySelector('.health-banner')
  }

  beforeEach(() => {
    qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
    sessionStorage.clear()
  })

  afterEach(async () => {
    await act(async () => { root.unmount() })
    container.remove()
  })

  it('无故障账户不渲染横幅', async () => {
    await renderWith([acct(1, 'a')])
    expect(banner()).toBe(null)
  })

  it('一个故障账户：横幅出现并点名', async () => {
    await renderWith([
      acct(1, 'a'),
      acct(2, 'b', { needs_reauth: false, breaker_open: true, failures: 5 }),
    ])
    expect(banner()).not.toBe(null)
    expect(banner()!.textContent).toContain('b')
    expect(banner()!.textContent).toContain('account.healthBanner.title')
  })

  it('三个故障账户：点名前 2 个，其余并作「等 N 个」', async () => {
    await renderWith([
      acct(1, 'x', { needs_reauth: false, breaker_open: true, failures: 5 }),
      acct(2, 'y', { needs_reauth: false, breaker_open: true, failures: 2 }),
      acct(3, 'z', { needs_reauth: false, breaker_open: true, failures: 3 }),
    ])
    const text = banner()!.textContent ?? ''
    expect(text).toContain('x')
    expect(text).toContain('y')
    expect(text).not.toContain('z')
    expect(text).toContain('account.healthBanner.more')
  })

  it('点「知道了」后横幅消失且记忆落在 sessionStorage', async () => {
    await renderWith([
      acct(1, 'a'),
      acct(2, 'b', { needs_reauth: false, breaker_open: true, failures: 5 }),
    ])
    const btn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === 'account.healthBanner.dismiss',
    )
    expect(btn).toBeTruthy()
    await act(async () => { btn!.click() })
    await flush()
    expect(banner()).toBe(null)
    expect(JSON.parse(sessionStorage.getItem('flymail-health-dismissed') ?? '[]')).toContain(2)
  })

  it('关闭后账户恢复再故障：横幅重现（恢复时已清记忆）', async () => {
    await renderWith([
      acct(2, 'b', { needs_reauth: false, breaker_open: true, failures: 5 }),
    ])
    const btn = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === 'account.healthBanner.dismiss',
    )
    await act(async () => { btn!.click() })
    await flush()
    expect(banner()).toBe(null)

    // 恢复（faultyIds 不再含 2）→ 记忆被清
    await rerenderWith([acct(2, 'b')])
    await flush()
    const stored = JSON.parse(sessionStorage.getItem('flymail-health-dismissed') ?? '[]')
    expect(stored).not.toContain(2)

    // 再故障 → 横幅重现
    await rerenderWith([
      acct(2, 'b', { needs_reauth: false, breaker_open: true, failures: 5 }),
    ])
    await flush()
    expect(banner()).not.toBe(null)
  })
})
```

flush helper（用例里用到，**放进 describe 内部、用例定义之前**；与 account-reorder.test.tsx 同款）：

```tsx
  async function flush(times = 4) {
    for (let i = 0; i < times; i++) {
      await act(async () => { await new Promise((r) => setTimeout(r, 0)) })
    }
  }
```

- [ ] **Step 7.2 运行确认失败**：`pnpm vitest run src/lib/account-health.test.ts src/lib/health-banner.test.tsx`
  预期：FAIL（pruneDismissed 未导出 / HealthBanner 模块不存在）。

- [ ] **Step 7.3 实现 lib helper**（account-health.ts 追加）：

```ts
const DISMISSED_KEY = 'flymail-health-dismissed'

/** 「知道了」的记忆：本会话内已关闭的故障账户 id。恢复时由 pruneDismissed 清除。 */
export function loadDismissed(): Set<number> {
  try {
    return new Set(JSON.parse(sessionStorage.getItem(DISMISSED_KEY) ?? '[]') as number[])
  } catch {
    return new Set()
  }
}

export function saveDismissed(s: Set<number>): void {
  try {
    sessionStorage.setItem(DISMISSED_KEY, JSON.stringify([...s]))
  } catch {
    // sessionStorage 不可用（隐私模式等）：记忆失效可接受，横幅行为回退为每次出现
  }
}

/** 恢复即清记忆：把 dismissed 里已不在 faultyIds 中的 id 移除。
 * 没有这一步，同会话内「恢复→再故障」不会重现横幅。 */
export function pruneDismissed(dismissed: Set<number>, faultyIds: Set<number>): Set<number> {
  const out = new Set<number>()
  dismissed.forEach((id) => {
    if (faultyIds.has(id)) out.add(id)
  })
  return out
}
```

- [ ] **Step 7.4 HealthBanner.tsx**（新建）：

```tsx
import { useState, useEffect, useMemo, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { useAccounts } from '@/lib/queries'
import { healthKind, loadDismissed, saveDismissed, pruneDismissed } from '@/lib/account-health'
import { requestOpenSettings } from '@/lib/settings-nav'
```

组件本体：

```tsx
export function HealthBanner() {
  const { t } = useTranslation()
  const { data: accounts = [] } = useAccounts()
  const [dismissed, setDismissed] = useState<Set<number>>(() => loadDismissed())
  const faulty = accounts.filter((a) => healthKind(a) !== null)
  const faultyIds = useMemo(() => new Set(faulty.map((a) => a.id)), [accounts])
  const visible = faulty.filter((a) => !dismissed.has(a.id))

  // 恢复即清记忆：faultyIds 缩小后把已恢复账户从 dismissed 移除，
  // 同会话「恢复→再故障」才会重现横幅。没有故障时 faultyIds 为空集，
  // prune 把记忆全清——这正是期望行为。
  useEffect(() => {
    if (dismissed.size === 0) return
    const next = pruneDismissed(dismissed, faultyIds)
    if (next.size === dismissed.size) return
    setDismissed(next)
    saveDismissed(next)
  }, [dismissed, faultyIds])
```

渲染段（useEffect 之后接）：

```tsx
  if (visible.length === 0) return null
  const named = visible.slice(0, 2)  const more = visible.length - named.length
  const dismissAll = useCallback(() => {
    const next = new Set(dismissed)
    visible.forEach((a) => next.add(a.id))
    setDismissed(next)
    saveDismissed(next)
  }, [dismissed, visible])

  return (
    <div className="health-banner" role="alert">
      <span className="health-banner-text">
        {t('account.healthBanner.title')}
        {named.map((a) => a.name).join('、')}
        {more > 0 && t('account.healthBanner.more').replace('{n}', String(more))}
      </span>
      <div className="health-banner-actions">
        <button
          type="button"
          className="health-banner-btn"
          onClick={() => requestOpenSettings('monitoring')}
        >
          {t('account.healthBanner.openSettings')}
        </button>
        <button
          type="button"
          className="health-banner-btn"
          onClick={dismissAll}
        >
          {t('account.healthBanner.dismiss')}
        </button>
      </div>
    </div>
  )
}
```

- [ ] **Step 7.5 index.css 追加**（紧挨 .conn-banner 系列之后，定位叠放关系见注释）：

```css
/* M17 账户健康横幅：固定在断线横幅上方 44px，两者同时出现时上下堆叠不重叠。
   固定定位不占布局——同 conn-banner 的取舍：故障可能持续数天，挤走内容更烦。 */
.health-banner {
  position: fixed; left: 50%; bottom: 116px; transform: translateX(-50%);
  z-index: 89;
  display: flex; align-items: center; gap: 10px;
  padding: 7px 14px; border-radius: 8px;
  background: var(--warning-wash); color: var(--warning-ink);
  border: 1px solid var(--destructive);
  border-color: color-mix(in srgb, var(--destructive) 45%, transparent);
  font-size: 12.5px;
  box-shadow: var(--shadow-sm);
  animation: toastIn 0.2s;
  max-width: calc(100vw - 32px);
}
.health-banner-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.health-banner-actions { display: flex; gap: 8px; flex-shrink: 0; }
.health-banner-btn {
  border: 1px solid var(--rule);
  background: transparent;
  color: inherit;
  padding: 2px 10px;
  border-radius: 6px;
  font-size: 12px;
  cursor: pointer;
}
```

- [ ] **Step 7.6 i18n 键**：`zh.json` 的 `account` 节 `health` 键之后追加 `healthBanner`：

```json
      "healthBanner": {
        "title": "账户同步异常：",
        "more": " 等 {n} 个",
        "openSettings": "查看",
        "dismiss": "知道了"
      },
```

`en.json` 对应位置：

```json
      "healthBanner": {
        "title": "Account sync failing: ",
        "more": " +{n} more",
        "openSettings": "View",
        "dismiss": "Dismiss"
      },
```

- [ ] **Step 7.7 Shell.tsx 挂载**：在 conn-banner 注释块（「{/\* 实时连接断了。这条必须说出来…… \*/}」）之前插入：

```tsx
      {/* M17 账户健康横幅：账户同步故障的全局出口。数据复用 ['accounts']
          查询缓存（SSE 事件经 Task 9 invalidate 它），无新轮询。
          固定定位不占布局，位于断线横幅上方，两者可同时出现。 */}
      <HealthBanner />

```

import 区追加 `import { HealthBanner } from '@/components/mail/HealthBanner'`。

- [ ] **Step 7.8 验证**：`pnpm vitest run src/lib/account-health.test.ts src/lib/health-banner.test.tsx` 绿（7+5=12 个用例）；
  `pnpm tsc -b` 无错；`pnpm build` 通过。
- [ ] **Step 7.9 提交**：

```bash
git add frontend/src/lib/account-health.ts frontend/src/lib/account-health.test.ts frontend/src/lib/health-banner.test.tsx frontend/src/components/mail/HealthBanner.tsx frontend/src/pages/Shell.tsx frontend/src/index.css frontend/src/locales/zh.json frontend/src/locales/en.json
git commit -F - --only -- frontend/src/lib/account-health.ts frontend/src/lib/account-health.test.ts frontend/src/lib/health-banner.test.tsx frontend/src/components/mail/HealthBanner.tsx frontend/src/pages/Shell.tsx frontend/src/index.css frontend/src/locales/zh.json frontend/src/locales/en.json <<'MSG'
feat(flymail): 全局健康横幅 HealthBanner（M17）

多账户故障时点名前 2 个+「等 N 个」，点「查看」打开监控设置页。
「知道了」按账户 id 记忆在 sessionStorage；账户恢复即清记忆，
同会话再故障会重现。固定底部第 116px，位于断线横幅上方。
MSG
```

---

### Task 8: 账户页徽标 + 「上次同步」相对时间

**Files:**
- Modify: `flymail/frontend/src/lib/account-health.ts`（追加 formatLastSyncRel）
- Modify: `flymail/frontend/src/components/settings/sections/AccountsSection.tsx`
- Modify: `flymail/frontend/src/locales/zh.json` + `en.json`
- Test: `flymail/frontend/src/lib/account-health.test.ts`（追加用例）
**Interfaces:**
- Consumes: `Account.health`（Task 5）、`healthKind`（Task 6）、`account.last_sync_at`（types.ts 既有字段）
- Produces: `formatLastSyncRel(iso: string | undefined, now: number, locale: string): string`——分桶：1 分钟内「刚刚」，60 分钟内「N 分钟前」，24 小时内「N 小时前」，30 天内「N 天前」，超过 30 天回退 `toLocaleDateString`；iso 缺省或非法返回空串。

- [ ] **Step 8.1 写失败测试**（account-health.test.ts 追加）。
  （spec「账户页徽标三态」的判定逻辑由 Task 6 的 healthKind 单测覆盖；chip 渲染本身走 Task 10 的 E2E 手动清单——AccountsSection 挂载成本高，不为展示层单开组件测试。）:

```ts
import { healthKind, formatLastSyncRel } from '@/lib/account-health'

describe('formatLastSyncRel', () => {
  const NOW = Date.UTC(2026, 8, 28, 12, 0, 0)

  it('缺省/非法 iso 返回空串', () => {
    expect(formatLastSyncRel(undefined, NOW, 'zh-CN')).toBe('')
    expect(formatLastSyncRel('not-a-date', NOW, 'zh-CN')).toBe('')
  })
  it('1 分钟内归为「刚刚」', () => {
    const iso = new Date(NOW - 30_000).toISOString()
    expect(formatLastSyncRel(iso, NOW, 'zh-CN')).toBe('刚刚')
  })
  it('分钟桶', () => {
    const iso = new Date(NOW - 5 * 60_000).toISOString()
    expect(formatLastSyncRel(iso, NOW, 'zh-CN')).toBe('5 分钟前')
  })
  it('小时桶', () => {
    const iso = new Date(NOW - 3 * 3_600_000).toISOString()
    expect(formatLastSyncRel(iso, NOW, 'zh-CN')).toBe('3 小时前')
  })
  it('天桶', () => {
    const iso = new Date(NOW - 2 * 86_400_000).toISOString()
    expect(formatLastSyncRel(iso, NOW, 'zh-CN')).toBe('2 天前')
  })
})
```

- [ ] **Step 8.2 运行确认失败**：`pnpm vitest run src/lib/account-health.test.ts`
  预期：formatLastSyncRel 未导出 → FAIL。

- [ ] **Step 8.3 实现 helper**（account-health.ts 追加）：

```ts
/** 「上次同步」相对时间：纯展示。iso 缺省或非法返回空串。
 * 分桶：1 分钟内「刚刚」，60 分钟内分钟桶，24 小时内小时桶，
 * 30 天内天桶，更久回退本地日期。 */
export function formatLastSyncRel(
  iso: string | undefined,
  now: number,
  locale: string,
): string {
  if (!iso) return ''
  const ms = new Date(iso).getTime()
  if (Number.isNaN(ms)) return ''
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
  const minutes = Math.floor((now - ms) / 60000)
  if (minutes < 1) return rtf.format(0, 'minute')
  if (minutes < 60) return rtf.format(-minutes, 'minute')
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return rtf.format(-hours, 'hour')
  const days = Math.floor(hours / 24)
  if (days < 30) return rtf.format(-days, 'day')
  return new Date(iso).toLocaleDateString(locale)
}
```

- [ ] **Step 8.4 AccountsSection 徽标与相对时间**：
  1. import 区追加：

```tsx
import { healthKind, formatLastSyncRel } from '@/lib/account-health'
```

  2. AccountCardRow 内 `const stats = statsQuery.data` 之后追加——文件已有 `const { t } = useTranslation()`，把该行改为：

```tsx
  const { t, i18n } = useTranslation()
```

  3. 状态徽标 span（ac-status）之后追加健康徽标（复用监控页 chip 风格）：

```tsx
        {(() => {
          const hk = healthKind(account)
          if (!hk) return null
          const isReauth = hk === 'needs_reauth'
          return (
            <span
              className="chip"
              style={{
                fontSize: 11,
                padding: '1px 8px',
                color: isReauth ? '#d97706' : 'var(--destructive)',
                borderColor: isReauth ? '#d97706' : 'var(--destructive)',
              }}
              title={account.health?.last_error}
            >
              {isReauth ? t('settings.account.needsReauth') : t('settings.account.syncError')}
            </span>
          )
        })()}
```
  4. stats 行（messages/folders 那个 div）内追加相对时间（`locale` 从 `i18n.language` 取）：

```tsx
            {account.last_sync_at && (
              <>
                &nbsp;·&nbsp;
                {t('settings.account.lastSync')}: {formatLastSyncRel(account.last_sync_at, Date.now(), locale)}
              </>
            )}
```

（`locale` 变量：`const locale = i18n.language`，在 `const { t, i18n } = useTranslation()` 后定义。）

- [ ] **Step 8.5 运行确认通过**：`pnpm vitest run src/lib/account-health.test.ts` 绿（4+4+5=13 用例）；
  `pnpm tsc -b` 无错；`pnpm build` 通过。

- [ ] **Step 8.6 i18n 键**：`settings.account.lastSync`（最后同步/Last sync）与
  `settings.account.syncError`（同步异常/Sync failing）**已存在，直接复用不重复添加**。
  仅需在 `zh.json` 的 `settings.account` 节追加：

```json
      "needsReauth": "需重新授权",
```

`en.json` 对应位置追加：

```json
      "needsReauth": "Reauth needed",
```

- [ ] **Step 8.7 提交**：

```bash
git add frontend/src/lib/account-health.ts frontend/src/lib/account-health.test.ts frontend/src/components/settings/sections/AccountsSection.tsx frontend/src/locales/zh.json frontend/src/locales/en.json
git commit -F - --only -- frontend/src/lib/account-health.ts frontend/src/lib/account-health.test.ts frontend/src/components/settings/sections/AccountsSection.tsx frontend/src/locales/zh.json frontend/src/locales/en.json <<'MSG'
feat(flymail): 账户设置页显示健康徽标与上次同步相对时间（M17）

健康账户无徽标；needs_reauth 橙 chip / 熔断红 chip，title 展示
最近错误。stats 行追加「上次同步」相对时间，分桶口径与通知一致。
MSG
```

---

### Task 9: 实时事件接线（SSE → 账户缓存失效 + 桌面通知）

**Files:**
- Modify: `flymail/frontend/src/lib/browser-notify.ts`（追加 showAccountNotice + 去重 helper）
- Modify: `flymail/frontend/src/hooks/useRealtimeSync.ts`（notify 分支扩展）
- Test: `flymail/frontend/src/lib/account-health.test.ts`（追加 shouldAccountNotice 用例）
- Test: `flymail/frontend/src/hooks/useRealtimeSync.test.tsx`（追加用例，仿既有结构）

**Interfaces:**
- Consumes: SSE notify 事件（ev.event: 'sync_failed' | 'account_status'，含 title/body/account_id——watcher 与 markNeedsReauth 已在后端 emit）、`getNotifyPrefs`、`pageHidden`
- Produces: `shouldAccountNotice(accountId: number, event: string, now?: number): boolean`（10s 去重）；`showAccountNotice(n: { title: string; body: string; accountId: number }): Notification | null`
- [ ] **Step 9.1 写失败测试**。lib 层（account-health.test.ts 追加；文件顶部补 import）：

```ts
import { shouldAccountNotice } from '@/lib/browser-notify'

describe('shouldAccountNotice 10 秒去重', () => {
  it('同账户同事件 10s 内折叠', () => {
    const t0 = 1_000_000
    expect(shouldAccountNotice(7, 'sync_failed', t0)).toBe(true)
    expect(shouldAccountNotice(7, 'sync_failed', t0 + 9_999)).toBe(false)
    expect(shouldAccountNotice(7, 'sync_failed', t0 + 10_001)).toBe(true)
  })
  it('不同账户互不折叠', () => {
    const t0 = 2_000_000
    expect(shouldAccountNotice(7, 'sync_failed', t0)).toBe(true)
    expect(shouldAccountNotice(8, 'sync_failed', t0)).toBe(true)
  })
  it('同账户不同事件互不折叠', () => {
    const t0 = 3_000_000
    expect(shouldAccountNotice(7, 'sync_failed', t0)).toBe(true)
    expect(shouldAccountNotice(7, 'account_status', t0)).toBe(true)
  })
})
```

组件级（useRealtimeSync.test.tsx 追加；注意：该文件对 `@/lib/browser-notify` 的
vi.mock 工厂是穷举式的——新导出 `showAccountNotice`/`shouldAccountNotice` 必须补进
工厂，否则 hook 一 import 就 undefined 调用炸掉。工厂追加两行：

```ts
const showAccountNotice = vi.fn()
```

工厂对象内（pageHidden 之后）追加：

```ts
  showAccountNotice: (...a: unknown[]) => showAccountNotice(...a),
  shouldAccountNotice: () => true,
```

追加用例（文件尾部、顶层 describe 内）：

```ts
const ACCT_EVENT: RealtimeEvent = {
  type: 'notify',
  event: 'sync_failed',
  account_id: 9,
  title: '同步异常：a',
  body: '连续 5 次失败——dial tcp',
}

const RECOVER_EVENT: RealtimeEvent = {
  type: 'notify',
  event: 'account_status',
  account_id: 9,
  title: '同步已恢复：a',
  body: '账户 a 恢复正常同步。',
}

  it('sync_failed/account_status：失效 accounts 缓存并弹桌面通知', async () => {
    const qc = await mount()
    const inv = vi.spyOn(qc, 'invalidateQueries')
    await fire(ACCT_EVENT)
    expect(inv).toHaveBeenCalledWith({ queryKey: ['accounts'] })
    expect(showAccountNotice).toHaveBeenCalledTimes(1)
    expect(showAccountNotice).toHaveBeenCalledWith(
      expect.objectContaining({ accountId: 9, title: '同步异常：a' }),
    )
  })

  it('恢复事件同样失效缓存并通知（恢复推送也是推送）', async () => {
    const qc = await mount()
    const inv = vi.spyOn(qc, 'invalidateQueries')
    await fire(RECOVER_EVENT)
    expect(inv).toHaveBeenCalledWith({ queryKey: ['accounts'] })
    expect(showAccountNotice).toHaveBeenCalledTimes(1)
  })
})
```

- [ ] **Step 9.2 运行确认失败**：`pnpm vitest run src/lib/account-health.test.ts src/hooks/useRealtimeSync.test.tsx`
  预期：shouldAccountNotice 未导出 → FAIL。

- [ ] **Step 9.3 实现 browser-notify.ts**（追加到 showMailNotice 之后）：

```ts
export interface AccountNotice {
  title: string
  body: string
  accountId: number
}

/** 同账户同事件的 10 秒去重：watcher 60s tick 与手动「立即同步」失败
 * 可能在短时间内重复触发同一事实，弹两条一样的通知只是噪音。
 * 内存态即可——重启浏览器后置位自然丢失，代价可忽略。 */
const lastAcctNotice = new Map<string, number>()

export function shouldAccountNotice(accountId: number, event: string, now = Date.now()): boolean {
  const key = `${event}-${accountId}`
  const last = lastAcctNotice.get(key) ?? 0
  if (now - last < 10_000) return false
  lastAcctNotice.set(key, now)
  return true
}

/** 弹一条账户健康通知。tag 按账户：同账户连续事件替换而非堆叠；
 * 无提示音（与 mail_new 不同，账户故障不需要唤起用户立即处理）。 */
export function showAccountNotice(n: AccountNotice): Notification | null {
  if (notifyPermission() !== 'granted') return null
  try {
    const notice = new Notification(n.title, {
      body: n.body,
      tag: `flymail-acct-${n.accountId}`,
    })
    notice.onclick = () => {
      window.focus()
      notice.close()
    }
    return notice
  } catch {
    return null
  }
}
```

- [ ] **Step 9.4 useRealtimeSync.ts notify 分支扩展**：在 `if (ev.event !== 'mail_new') return` 之前插入：

```ts
        // M17 账户健康事件：账户列表缓存必须失效，侧栏标记/横幅/账户页才跟得上；
        // 桌面通知走独立函数（tag 按账户分 tag、10s 去重防双弹，无提示音）。
        if (ev.event === 'sync_failed' || ev.event === 'account_status') {
          void qc.invalidateQueries({ queryKey: ['accounts'] })
          if (!pageHidden()) return
          const prefs = getNotifyPrefs()
          if (prefs.desktop && shouldAccountNotice(ev.account_id, ev.event)) {
            showAccountNotice({ title: ev.title, body: ev.body, accountId: ev.account_id })
          }
          return
        }
```

import 区追加：

```ts
import { showAccountNotice, shouldAccountNotice } from '@/lib/browser-notify'
```

（既有 import 行 `import { showMailNotice, playChime, claimChime, pageHidden, getNotifyPrefs } from '@/lib/browser-notify'`——以文件实际为准合并进同一行或新增一行均可。）

- [ ] **Step 9.5 验证**：`pnpm vitest run src/lib/account-health.test.ts src/hooks/useRealtimeSync.test.tsx` 绿；
  `pnpm tsc -b` 无错；`pnpm build` 通过。
- [ ] **Step 9.6 提交**：

```bash
git add frontend/src/lib/browser-notify.ts frontend/src/lib/account-health.test.ts frontend/src/hooks/useRealtimeSync.ts frontend/src/hooks/useRealtimeSync.test.tsx
git commit -F - --only -- frontend/src/lib/browser-notify.ts frontend/src/lib/account-health.test.ts frontend/src/hooks/useRealtimeSync.ts frontend/src/hooks/useRealtimeSync.test.tsx <<'MSG'
feat(flymail): 账户健康事件接 SSE 与桌面通知（M17）

sync_failed/account_status 到达即失效 accounts 查询缓存，侧栏/横幅/账户页
跟随刷新；标签页不可见时弹桌面通知（tag 按账户、10s 去重、无提示音）。
MSG
```

---

### Task 10: 收尾——全量验证与真机验收清单

**Files:**
- Modify: `flymail/docs/flymail/m17-account-health.md`（状态行更新为「已实现」）
- Test: 全量回归（无新文件）

**Interfaces:**
- Consumes: Task 1-9 的全部交付物
- Produces: 绿灯证据 + 验收记录

- [ ] **Step 10.1 后端全量回归**：

```bash
cd flymail/backend
gofmt -l . && go build ./... && go test ./... 2>&1 | tail -30
```

预期：gofmt 无输出、build 通过、全量测试 PASS（若有失败逐个修掉再进下一步）。

- [ ] **Step 10.2 前端全量回归**：

```bash
cd flymail/frontend
pnpm vitest run 2>&1 | tail -15
pnpm tsc -b
pnpm build
```

预期：vitest 全绿、tsc -b 无错、build 通过。

- [ ] **Step 10.3 真机验收清单**（按 flymail 开发惯例执行 `./e2e.ps1` 基线 9/9 PASS 后逐项人工核对；需真机/真实账户的部分记录到 `docs/flymail/m17-account-health.md` 末尾验收记录节）：
  1. **熔断进入沿**：GreenMail 账户密码改错 → 5 次连续失败后熔断 → 通知中心出现
     「同步异常：{账户名}」，webhook 渠道收到外发 JSON（若配了测试渠道）；
  2. **UI 三处**：侧栏账户行出现红色「同步异常」标记、底部横幅出现并点名该账户、
     账户设置页出现红 chip；
  3. **24h 重复**：把 healthRepeatEvery 临时改为 1 分钟重编译（仅验收用，勿提交），
     观察到「同步异常持续：{账户名}」外发推送且通知中心**不**新增落库（SkipInApp 生效）；
  4. **恢复沿**：密码改回 → 下次同步成功 → 「同步已恢复：{账户侧栏名}」通知 + 横幅消失 +
     侧栏标记消失 + 账户页 chip 消失；
  5. **needs_reauth 沿**（真机 Google 账户，若 M14 OAuth 真机凭据仍未配齐则此项挂起，
     依据 M14 验收结论决定是否补做）：撤销授权后 → markNeedsReauth 即时推送
     （account_status 事件）→ 60s 内 watcher 观察到翻转但**不**重复推送（双推送防线）→
     侧栏橙标 + 横幅点名；
  6. **重启补推**：故障持续期间重启后端 → 第一个 tick 补推存量熔断（通知中心落库+外发）。

- [ ] **Step 10.4 spec 状态行更新**：`docs/flymail/m17-account-health.md` 顶部状态行
  （若有「状态：设计」类字样）改为「已实现（2026-09-28）」，并在文档末尾追加验收记录节：

```markdown
## 验收记录

- 2026-09-28：M17 实现（Task 1-9）+ 真机验收清单 6 项核对完成。
```

- [ ] **Step 10.5 文档提交**：

```bash
git add docs/flymail/m17-account-health.md docs/flymail/m17-account-health-plan.md
git commit -F - --only -- docs/flymail/m17-account-health.md docs/flymail/m17-account-health-plan.md <<'MSG'
docs(flymail): M17 实现计划与验收记录（账户健康可观测性）
MSG
```
