# FlyMail M6 增量同步 + 实时收信 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 后台自动检测并增量拉取新邮件(IDLE 优先 + 轮询兜底,覆盖所有订阅文件夹),通过 SSE 实时推送到前端自动刷新。

**Architecture:** 后端新增 `sync.Manager`:为每个启用账户起一个 goroutine worker,单 IMAP 连接串行驱动——轮询周期遍历所有订阅文件夹做增量同步 → 选 INBOX 进入 IDLE → 被 IDLE 事件 / 轮询定时器 / 29min IDLE 刷新 / ctx 取消唤醒 → 停 IDLE 回到轮询。账户增删用 reconcile 循环(每 30s 比对启用账户与运行 worker)解耦 account 模块。`message.IncrementalSync` 只拉新邮件(UIDNEXT 锚点;163 无 UIDNEXT 时用消息总数增量按序号补)。新邮件经 `sse.Hub` 发布,前端 `EventSource` 订阅 `GET /api/v1/events` 后失效相关查询自动刷新。

**Tech Stack:** Go(go-imap/v2 IDLE、context、ticker)、gin SSE(text/event-stream)、React EventSource、TanStack Query 失效。

**只拉新邮件**(用户决策):本里程碑不回拉服务器端 flag 变化(已读/删除被其他客户端改),状态双向同步推后到后续里程碑。

---

## 文件结构

**后端(flymail/backend):**
- 修改 `modules/email/message/service.go` — 新增 `IncrementalSync`(只拉新邮件)
- 新建 `modules/email/message/incremental_test.go` — IncrementalSync 单测
- 修改 `modules/email/account/repository.go` + `service.go` — 新增 `ListEnabledIDs`
- 新建 `internal/sse/hub.go` — SSE 事件 Hub(发布/订阅,无 DB 依赖)
- 新建 `internal/sse/hub_test.go` — Hub 单测
- 新建 `internal/sse/handler.go` — `GET /api/v1/events` SSE 端点(query-param access_token 鉴权)
- 修改 `modules/email/sync/service.go` — 扩展 `Session` 接口加 IDLE 能力;新增 `Event`/`Publisher` 类型
- 新建 `modules/email/sync/manager.go` — `Manager`(worker 生命周期 + IDLE 循环 + 轮询 + reconcile)
- 新建 `modules/email/sync/manager_test.go` — Manager 单测(reconcile + pollAll,fake CanIDLE=false)
- 修改 `modules/system/setting/service.go` — 新增 `KeySyncPollInterval` 常量
- 修改 `internal/server/router.go` — `Deps` 加 `Events http.HandlerFunc` + `Auth`,挂 `/events` 路由
- 修改 `internal/app/app.go` — 装配 Hub + Manager;`Start` 调 `manager.Start(ctx)`;`Shutdown` 调 `manager.Stop()`
- 修改 `core/imap/client.go` — `Session` 加 `CanIDLE()` 方法

**前端(flymail/frontend/src):**
- 新建 `lib/sse.ts` — EventSource 客户端封装(连接/重连/token 过期处理)
- 新建 `hooks/useRealtimeSync.ts` — 订阅 SSE,收到事件失效 folders/messages/sync-status 查询
- 修改 `pages/Shell.tsx` — 挂载 `useRealtimeSync()`
- 修改 `lib/types.ts` — 新增 `RealtimeEvent` 类型;`AppSettings` 加 `sync_poll_interval`
- 修改 `lib/queries.ts` — `useSettings` 读 `sync_poll_interval`
- 修改 `components/settings/MailSection.tsx` — 加"轮询间隔"输入
- 修改 `locales/zh.json` + `locales/en.json` — 轮询间隔文案

**文档/记忆:**
- 修改 `docs/superpowers/known-issues.md` — KI-2:SSE access_token 走 URL query
- 修改 memory `project_flymail_milestone1.md` — M6 完成条目

---

## Phase A:后端增量同步(message 模块)

### Task A1: message.IncrementalSync

**Files:**
- Modify: `flymail/backend/modules/email/message/service.go`
- Test: `flymail/backend/modules/email/message/incremental_test.go`

设计:`IncrementalSync` 复用现有 `fetchRangeBatched`/`fetchSeqRangeBatched`,只抓本地之后新增的邮件。`newCount` 用同步前后本地行数差(增量路径只增不删,准确且不受 prev 漂移影响)。UIDVALIDITY 变化时删除本地并退化为 `SyncFolderMessages` 完整重建。

- [ ] **Step 1: 写失败测试** `incremental_test.go`

复用 `service_test.go` 里已有的 fake fetcher 模式(读取该文件以对齐 fake 结构与 newTestService 辅助)。三个场景:

```go
package message

// 场景 1:已知 UIDNEXT,anchor=prevUIDNext,抓 [prevUIDNext, uidNext-1]。
//   预置文件夹本地无邮件;fake SELECT 返回 UIDNext=11、NumMessages=10;
//   prevUIDNext=6 → 应抓 UID[6,10],FetchByUIDRange 收到 from=6 to=10;
//   返回 state.UIDNext=11、newCount=5(本地新增 5 行)。
//
// 场景 2:无 UIDNEXT(163),用总数增量按序号补。
//   fake SELECT 返回 UIDNext=0、NumMessages=12;FolderStatus(UIDNext) 也返回 nil;
//   prevTotal=10 → delta=2 → FetchBySeqRange 收到 from=11 to=12;newCount=2;
//   state.UIDNext = 本地 maxUID+1。
//
// 场景 3:无新邮件。
//   已知 UIDNEXT;prevUIDNext == uidNext → 不调用任何 Fetch;newCount=0。
```

每个场景断言:Fetch 调用入参区间正确、`newCount` 正确、`state` 字段正确、无新邮件时不触发 Fetch。

- [ ] **Step 2: 跑测试确认失败** `go test ./modules/email/message/ -run TestIncrementalSync -v`,预期 FAIL(IncrementalSync 未定义)。

- [ ] **Step 3: 实现 IncrementalSync**

```go
// IncrementalSync 增量同步单文件夹：只抓取本地之后新增的邮件。
// prev* 为本地已存的该文件夹状态（来自 folders 表）。
// 返回：同步后状态、本次新增邮件数、错误。
// UIDVALIDITY 变化时删除本地缓存并退化为完整重建。
func (s *Service) IncrementalSync(accountID, folderID uint, folderPath string, prevUIDValidity, prevUIDNext uint32, prevTotal int, c IMAPFetcher) (*FolderState, int, error) {
	sel, err := c.SelectFolder(folderPath)
	if err != nil {
		return nil, 0, err
	}

	uidValidity := sel.UIDValidity
	if uidValidity == 0 {
		if st, serr := c.FolderStatus(folderPath, coreimap.StatusUIDValidity); serr == nil && st != nil && st.UIDValidity != nil {
			uidValidity = *st.UIDValidity
		}
	}

	// UIDVALIDITY 变化：本地缓存失效，删除后完整重建。
	if prevUIDValidity != 0 && uidValidity != 0 && uidValidity != prevUIDValidity {
		if err := s.repo.DeleteByFolder(folderID); err != nil {
			return nil, 0, err
		}
		state, _, err := s.SyncFolderMessages(accountID, folderID, folderPath, 0, c)
		if err != nil {
			return nil, 0, err
		}
		return state, state.Total, nil
	}

	beforeCount, _ := s.repo.CountByFolder(folderID)

	uidNext := sel.UIDNext
	if uidNext == 0 {
		if st, serr := c.FolderStatus(folderPath, coreimap.StatusUIDNext); serr == nil && st != nil && st.UIDNext != nil {
			uidNext = *st.UIDNext
		}
	}

	if uidNext > 0 {
		// 已知 UIDNEXT：抓 [anchor, uidNext-1]，anchor=prevUIDNext（无则本地 maxUID+1）。
		anchor := prevUIDNext
		if anchor == 0 {
			if maxUID, _ := s.repo.MaxUID(folderID); maxUID > 0 {
				anchor = maxUID + 1
			} else {
				anchor = 1
			}
		}
		if uidNext > anchor {
			if err := s.fetchRangeBatched(accountID, folderID, imapv2.UID(anchor), imapv2.UID(uidNext-1), c); err != nil {
				return nil, 0, err
			}
		}
	} else {
		// 无 UIDNEXT（163）：用消息总数增量，按序号补抓尾部 delta 封。
		currentTotal := int(sel.NumMessages)
		delta := currentTotal - prevTotal
		if delta > 0 {
			from := uint32(currentTotal - delta + 1)
			if from < 1 {
				from = 1
			}
			if err := s.fetchSeqRangeBatched(accountID, folderID, from, uint32(currentTotal), c); err != nil {
				return nil, 0, err
			}
		}
	}

	total, _ := s.repo.CountByFolder(folderID)
	unread, _ := s.repo.UnreadCountByFolder(folderID)
	newCount := int(total) - int(beforeCount)
	if newCount < 0 {
		newCount = 0
	}
	if uidNext == 0 {
		if maxUID, _ := s.repo.MaxUID(folderID); maxUID > 0 {
			uidNext = maxUID + 1
		}
	}
	return &FolderState{
		UIDValidity: uidValidity,
		UIDNext:     uidNext,
		Total:       int(total),
		Unread:      int(unread),
	}, newCount, nil
}
```

- [ ] **Step 4: 跑测试确认通过** `go test ./modules/email/message/ -run TestIncrementalSync -v`,预期 PASS。

- [ ] **Step 5: gofmt + 提交** `gofmt -w` 改动文件;`git add` 两文件;commit `feat(flymail): message 增量同步 IncrementalSync（只拉新邮件）`。

---

## Phase B:SSE Hub + 端点

### Task B1: sse.Hub

**Files:**
- Create: `flymail/backend/internal/sse/hub.go`
- Test: `flymail/backend/internal/sse/hub_test.go`

设计:进程内发布/订阅。`Subscribe` 返回带缓冲的只读 channel + 取消函数;`Publish` 向所有订阅者非阻塞投递(满则丢弃,SSE 是尽力推送)。线程安全。

- [ ] **Step 1: 写失败测试** `hub_test.go`

```go
package sse

import (
	"testing"
	"time"
)

func TestHubPublishToSubscriber(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe()
	defer cancel()

	h.Publish([]byte(`{"type":"new_mail"}`))

	select {
	case msg := <-ch:
		if string(msg) != `{"type":"new_mail"}` {
			t.Fatalf("unexpected payload: %s", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive event")
	}
}

func TestHubUnsubscribeStopsDelivery(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe()
	cancel()
	h.Publish([]byte("x"))
	// 取消后 channel 已关闭，读取应立即返回（不阻塞、不收新值）。
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected no live value after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("read after cancel should not block")
	}
}

func TestHubPublishNonBlockingWhenBufferFull(t *testing.T) {
	h := NewHub()
	_, cancel := h.Subscribe() // 不消费
	defer cancel()
	// 远超缓冲容量的发布不得阻塞。
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			h.Publish([]byte("x"))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on full subscriber buffer")
	}
}
```

- [ ] **Step 2: 跑测试确认失败** `go test ./internal/sse/ -v`,预期 FAIL(包不存在)。

- [ ] **Step 3: 实现 hub.go**

```go
// Package sse 提供进程内的 Server-Sent Events 发布/订阅 Hub。
package sse

import "sync"

const subscriberBuffer = 16

// Hub 向所有订阅者广播字节负载（已序列化的事件）。
type Hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: map[chan []byte]struct{}{}}
}

// Subscribe 注册一个订阅者，返回只读 channel 与取消函数。
// 取消函数幂等：移除订阅并关闭 channel。
func (h *Hub) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, subscriberBuffer)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, ch)
			h.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

// Publish 向所有订阅者非阻塞投递；订阅者缓冲已满则丢弃该消息（尽力推送）。
func (h *Hub) Publish(payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- payload:
		default:
		}
	}
}
```

- [ ] **Step 4: 跑测试确认通过** `go test ./internal/sse/ -v`,预期 PASS。

- [ ] **Step 5: 提交** commit `feat(flymail): SSE Hub（进程内发布订阅）`。

### Task B2: SSE 端点 handler

**Files:**
- Create: `flymail/backend/internal/sse/handler.go`

设计:`NewHandler(hub *Hub, verify func(token string) error) http.HandlerFunc`。鉴权:从 `?access_token=` 取 token 交 `verify` 校验(EventSource 不能设 Authorization 头,故走 query)。设置 SSE 头,订阅 Hub,循环把负载写成 `data: <json>\n\n`,每 25s 发心跳注释 `: ping\n\n`,客户端断开(`r.Context().Done()`)退出。

- [ ] **Step 1: 实现 handler.go**

```go
package sse

import (
	"fmt"
	"net/http"
	"time"
)

// NewHandler 返回 SSE 端点处理器。verify 校验 access_token（失败返回非 nil → 401）。
func NewHandler(hub *Hub, verify func(token string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("access_token")
		if err := verify(token); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ch, cancel := hub.Subscribe()
		defer cancel()

		heartbeat := time.NewTicker(25 * time.Second)
		defer heartbeat.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeat.C:
				fmt.Fprint(w, ": ping\n\n")
				flusher.Flush()
			case msg, open := <-ch:
				if !open {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", msg)
				flusher.Flush()
			}
		}
	}
}
```

- [ ] **Step 2: 验证编译** `go build ./internal/sse/`,预期成功(此 handler 无独立单测,集成路径在 D1 装配后由真机验证)。

- [ ] **Step 3: 提交** commit `feat(flymail): SSE 端点 handler（query-param 鉴权 + 心跳）`。

---

## Phase C:同步管理器(sync.Manager)

### Task C1: account.ListEnabledIDs

**Files:**
- Modify: `flymail/backend/modules/email/account/repository.go`
- Modify: `flymail/backend/modules/email/account/service.go`
- Test: `flymail/backend/modules/email/account/repository_test.go`(追加用例)

- [ ] **Step 1: 写失败测试**(追加到 repository_test.go)

```go
func TestListEnabledIDs(t *testing.T) {
	repo := newTestRepo(t) // 复用文件内已有 helper
	// 建两个启用、一个停用账户（复用已有建账户 helper / 直接 Create）。
	// ... 创建 a1(enabled), a2(disabled), a3(enabled)
	ids, err := repo.ListEnabledIDs()
	if err != nil {
		t.Fatal(err)
	}
	// 仅含 a1、a3。
	if len(ids) != 2 {
		t.Fatalf("want 2 enabled ids, got %d (%v)", len(ids), ids)
	}
}
```

(读取 repository_test.go 对齐已有 helper 命名与建账户方式。)

- [ ] **Step 2: 跑测试确认失败** `go test ./modules/email/account/ -run TestListEnabledIDs -v`,预期 FAIL。

- [ ] **Step 3: 实现** repository.go:

```go
// ListEnabledIDs 返回所有 enabled=true 账户的 ID。
func (r *Repository) ListEnabledIDs() ([]uint, error) {
	var ids []uint
	err := r.db.Model(&Account{}).Where("enabled = ?", true).Pluck("id", &ids).Error
	return ids, err
}
```

service.go:

```go
// ListEnabledIDs 透传启用账户 ID 列表（供同步管理器调度）。
func (s *Service) ListEnabledIDs() ([]uint, error) { return s.repo.ListEnabledIDs() }
```

- [ ] **Step 4: 跑测试确认通过** `go test ./modules/email/account/ -run TestListEnabledIDs -v`,预期 PASS。

- [ ] **Step 5: 提交** commit `feat(flymail): account.ListEnabledIDs（启用账户调度用）`。

### Task C2: 扩展 sync.Session 接口 + core CanIDLE

**Files:**
- Modify: `core/imap/client.go`
- Modify: `flymail/backend/modules/email/sync/service.go`

- [ ] **Step 1: core 加 CanIDLE 方法** `core/imap/client.go`:

```go
// CanIDLE 报告服务器是否支持 IDLE（基于已读取的 capabilities）。
func (s *Session) CanIDLE() bool { return s.SupportsIDLE }
```

- [ ] **Step 2: 扩展 sync.Session 接口** `service.go`,在现有 `Session` 接口内追加 IDLE 能力:

```go
type Session interface {
	folder.IMAPLister
	message.IMAPFetcher
	FetchByUIDs(uids []imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error)
	MarkRead(uids ...imapv2.UID) error
	MarkUnread(uids ...imapv2.UID) error
	MarkStarred(uids ...imapv2.UID) error
	MarkUnstarred(uids ...imapv2.UID) error
	// IDLE 能力（M6）：
	CanIDLE() bool
	StartIDLE() (*coreimap.IdleHandle, error)
	SetIDLEHandler(func(coreimap.IDLEEvent))
	Close() error
}
```

- [ ] **Step 3: 验证编译** `go build ./...`(在 backend 与 core 两处),`*coreimap.Session` 应满足扩展接口(已有 StartIDLE/SetIDLEHandler,新增 CanIDLE)。预期成功。

- [ ] **Step 4: 提交** commit `feat(flymail): Session 接口加 IDLE 能力 + core CanIDLE`。

### Task C3: setting KeySyncPollInterval

**Files:**
- Modify: `flymail/backend/modules/system/setting/service.go`(或常量所在文件)

- [ ] **Step 1: 加常量**(与 `KeySyncDepth` 同处):

```go
// KeySyncPollInterval 后台轮询间隔（秒）。
const KeySyncPollInterval = "sync_poll_interval"
```

- [ ] **Step 2: 验证编译** `go build ./modules/system/setting/`。

- [ ] **Step 3: 提交** commit `feat(flymail): setting 增加 sync_poll_interval 常量`。

### Task C4: sync.Manager

**Files:**
- Modify: `flymail/backend/modules/email/sync/service.go`(加 `Event`/`Publisher` 类型 + `AccountLister` 接口扩展 `ListEnabledIDs`)
- Create: `flymail/backend/modules/email/sync/manager.go`
- Test: `flymail/backend/modules/email/sync/manager_test.go`

设计要点(见本计划顶部 Architecture):单连接单 goroutine worker;reconcile 循环按启用账户增删 worker;`pollAll` 先 `SyncFolders`(发现新文件夹)再遍历 DB 文件夹做 `IncrementalSync`,`newCount>0` 时 `Publish`;IDLE 仅 INBOX,事件/29min/轮询定时器/ctx 唤醒。单测用 `CanIDLE()=false` 的 fake session 绕开 IDLE,覆盖 reconcile 与 pollAll;IDLE 路径靠真机验证。

- [ ] **Step 1: service.go 追加类型**

```go
// Event 是推送给前端的同步事件。
type Event struct {
	Type      string `json:"type"`       // "new_mail"
	AccountID uint   `json:"account_id"`
	FolderID  uint   `json:"folder_id"`
	NewCount  int    `json:"new_count"`
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
```

- [ ] **Step 2: 写失败测试** `manager_test.go`

复用 `service_test.go` 的 fake session/DB 搭建(读取该文件对齐)。fake session 实现扩展后的 Session 接口,`CanIDLE()` 返回 false,`StartIDLE` 返回 `(nil,nil)`,`SetIDLEHandler` no-op。

```go
package sync

// 测试 1:reconcile 启动/停止 worker。
//   fakeAccounts.ListEnabledIDs 先返回 {1,2} → Start 后 m.workerCount()==2；
//   改为返回 {1} → 手动调 m.reconcile() → workerCount()==1；
//   返回 {} → reconcile → workerCount()==0。
//   （worker 用 CanIDLE=false 的 fake，poll 间隔设大，worker 进入阻塞 select。）
//   用一个导出测试辅助 m.workerCount() 或在测试内通过 mu 读 len(workers)（同包可访问）。

// 测试 2:pollAll 增量并发布事件。
//   in-memory sqlite 预置 account=1 + INBOX 文件夹(TotalCount=10,UIDNext=11)。
//   fake session：ListFolders 返回该 INBOX；SELECT 返回 UIDNext=16、NumMessages=15；
//   FetchByUIDRange(11,15) 返回 5 封新邮件。
//   调 m.pollAll(1, sess) → DB 新增 5 行；folder 状态更新为 UIDNext=16/Total=15；
//   pub 收到一条 Event{Type:"new_mail",FolderID:<inbox>,NewCount:5}。
//   fakePublisher 记录 Publish 的 payload，反序列化断言。

// 测试 3:pollAll 无新邮件不发布。
//   SELECT 返回 UIDNext == 文件夹已存 UIDNext → newCount=0 → pub 无调用。
```

- [ ] **Step 3: 跑测试确认失败** `go test ./modules/email/sync/ -run TestManager -v`,预期 FAIL。

- [ ] **Step 4: 实现 manager.go**

```go
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	gosync "sync"
	"time"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	"flymail/modules/email/folder"
	"flymail/modules/email/message"
)

const (
	defaultPollInterval = 180 * time.Second
	minPollInterval     = 30 * time.Second
	idleRefreshInterval = 29 * time.Minute
	reconcileInterval   = 30 * time.Second
	maxReconnectBackoff = 60 * time.Second
)

// Manager 后台调度：每启用账户一个 worker（单连接串行驱动），IDLE 优先 + 轮询兜底。
type Manager struct {
	accounts AccountLister
	folders  *folder.Service
	messages *message.Service
	pub      Publisher

	dial         func(types.IMAPConfig) (Session, error)
	pollInterval func() time.Duration

	mu      gosync.Mutex
	workers map[uint]context.CancelFunc
	wg      gosync.WaitGroup
	rootCtx context.Context
}

func NewManager(accounts AccountLister, folders *folder.Service, messages *message.Service, pub Publisher) *Manager {
	return &Manager{
		accounts:     accounts,
		folders:      folders,
		messages:     messages,
		pub:          pub,
		dial:         defaultDial,
		pollInterval: func() time.Duration { return defaultPollInterval },
		workers:      map[uint]context.CancelFunc{},
	}
}

// SetDial 测试注入。
func (m *Manager) SetDial(d func(types.IMAPConfig) (Session, error)) { m.dial = d }

// SetPollIntervalProvider 注入轮询间隔（秒，<minPollInterval 取下限）。
func (m *Manager) SetPollIntervalProvider(fn func() int) {
	if fn == nil {
		return
	}
	m.pollInterval = func() time.Duration {
		d := time.Duration(fn()) * time.Second
		if d < minPollInterval {
			return minPollInterval
		}
		return d
	}
}

// Start 启动调度：立即调和一次，并起 reconcile 循环。
func (m *Manager) Start(ctx context.Context) {
	m.rootCtx = ctx
	m.reconcile()
	go m.reconcileLoop(ctx)
}

// Stop 取消所有 worker 并等待退出。
func (m *Manager) Stop() {
	m.mu.Lock()
	for id, cancel := range m.workers {
		cancel()
		delete(m.workers, id)
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *Manager) reconcileLoop(ctx context.Context) {
	t := time.NewTicker(reconcileInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.reconcile()
		}
	}
}

func (m *Manager) reconcile() {
	ids, err := m.accounts.ListEnabledIDs()
	if err != nil {
		return
	}
	want := make(map[uint]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range want {
		if _, ok := m.workers[id]; !ok {
			wctx, cancel := context.WithCancel(m.rootCtx)
			m.workers[id] = cancel
			m.wg.Add(1)
			go m.worker(wctx, id)
		}
	}
	for id, cancel := range m.workers {
		if !want[id] {
			cancel()
			delete(m.workers, id)
		}
	}
}

func (m *Manager) worker(ctx context.Context, accountID uint) {
	defer m.wg.Done()
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := m.runSession(ctx, accountID)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxReconnectBackoff {
				backoff = maxReconnectBackoff
			}
			continue
		}
		backoff = time.Second
	}
}

// runSession 建立一条连接并驱动「轮询 ↔ IDLE」循环，直到出错或 ctx 取消。
func (m *Manager) runSession(ctx context.Context, accountID uint) error {
	cfg, err := m.accounts.IMAPConfig(accountID)
	if err != nil {
		return err
	}
	sess, err := m.dial(cfg)
	if err != nil {
		return err
	}
	defer sess.Close()

	// 初始全文件夹增量同步。
	m.pollAll(accountID, sess)

	idleCh := make(chan struct{}, 1)
	if sess.CanIDLE() {
		sess.SetIDLEHandler(func(coreimap.IDLEEvent) {
			select {
			case idleCh <- struct{}{}:
			default:
			}
		})
	}

	pollTicker := time.NewTicker(m.pollInterval())
	defer pollTicker.Stop()

	for {
		var handle *coreimap.IdleHandle
		var idleDone <-chan error
		var idleRefresh <-chan time.Time
		if sess.CanIDLE() {
			if inbox, _ := m.folders.FindInbox(accountID); inbox != nil {
				if _, err := sess.SelectFolder(inbox.Path); err == nil {
					if h, err := sess.StartIDLE(); err == nil {
						handle = h
						idleDone = h.Done()
						t := time.NewTimer(idleRefreshInterval)
						defer t.Stop()
						idleRefresh = t.C
					}
				}
			}
		}

		select {
		case <-ctx.Done():
			if handle != nil {
				_ = handle.Stop("shutdown")
			}
			return nil
		case <-idleCh:
			if handle != nil {
				_ = handle.Stop("new-mail")
			}
			m.pollInbox(accountID, sess)
		case <-idleDone:
			return fmt.Errorf("idle connection closed")
		case <-idleRefresh:
			if handle != nil {
				_ = handle.Stop("refresh")
			}
		case <-pollTicker.C:
			if handle != nil {
				_ = handle.Stop("poll")
			}
			m.pollAll(accountID, sess)
		}
	}
}

// pollAll 重列文件夹（发现新文件夹）后对所有可选文件夹做增量同步。
func (m *Manager) pollAll(accountID uint, sess Session) {
	_ = m.folders.SyncFolders(accountID, sess)
	fs, err := m.folders.List(accountID)
	if err != nil {
		return
	}
	for i := range fs {
		f := &fs[i]
		if !f.Selectable {
			continue
		}
		m.syncFolder(accountID, f, sess)
	}
	_ = m.accounts.TouchLastSync(accountID, time.Now())
}

// pollInbox 只增量同步收件箱（IDLE 唤醒用）。
func (m *Manager) pollInbox(accountID uint, sess Session) {
	inbox, err := m.folders.FindInbox(accountID)
	if err != nil || inbox == nil {
		return
	}
	m.syncFolder(accountID, inbox, sess)
}

func (m *Manager) syncFolder(accountID uint, f *folder.Folder, sess Session) {
	state, newCount, err := m.messages.IncrementalSync(
		accountID, f.ID, f.Path, f.UIDValidity, f.UIDNext, f.TotalCount, sess,
	)
	if err != nil {
		return
	}
	_ = m.folders.UpdateSyncState(f.ID, state.UIDValidity, state.UIDNext, state.Total, state.Unread, time.Now())
	if newCount > 0 && m.pub != nil {
		payload, _ := json.Marshal(Event{
			Type:      "new_mail",
			AccountID: accountID,
			FolderID:  f.ID,
			NewCount:  newCount,
		})
		m.pub.Publish(payload)
	}
}
```

注意:`folder.List` 返回 `[]Folder`(见 folder/service.go),`Selectable` 字段用于跳过 `\Noselect`。`idleRefresh`/`idleDone` 用局部 `var` 默认 nil channel(nil channel 在 select 中永久阻塞),`CanIDLE()=false` 时三者均 nil,select 只在 ctx/pollTicker 上唤醒。

- [ ] **Step 5: 跑测试确认通过** `go test ./modules/email/sync/ -run TestManager -v`,预期 PASS。

- [ ] **Step 6: gofmt + 提交** commit `feat(flymail): 同步管理器 Manager（IDLE+轮询+reconcile 调度）`。

---

## Phase D:装配 + 生命周期

### Task D1: app/router 装配 Hub + Manager + SSE 路由

**Files:**
- Modify: `flymail/backend/internal/server/router.go`
- Modify: `flymail/backend/internal/app/app.go`

- [ ] **Step 1: router.go — Deps 加字段 + 挂 /events**

`Deps` 追加:`Events http.HandlerFunc`(SSE 端点)。在受保护组之外、`/api/v1` 组之内挂载(SSE 自带 query 鉴权,不走 Bearer 中间件):

```go
// 在 api := r.Group("/api/v1") 之后、protected 之前：
if deps.Events != nil {
	api.GET("/events", gin.WrapF(deps.Events))
}
```

- [ ] **Step 2: app.go — 装配**

```go
// import 增加：
//   "flymail/internal/sse"
//   "net/http"（已存在）

// 在构造 syncSvc 之后：
hub := sse.NewHub()
manager := syncmod.NewManager(accountSvc, folderSvc, messageSvc, hub)
manager.SetPollIntervalProvider(func() int { return settingSvc.GetInt(setting.KeySyncPollInterval, 180) })

eventsHandler := sse.NewHandler(hub, func(token string) error {
	_, err := authSvc.VerifyAccessToken(token)
	return err
})

handler := server.New(server.Deps{
	Auth:    authSvc,
	Account: accountSvc,
	Folder:  folderSvc,
	Message: messageSvc,
	Sync:    syncSvc,
	Setting: settingSvc,
	Send:    sendSvc,
	Draft:   draftSvc,
	Events:  eventsHandler,
})

a := &App{cfg: cfg, srv: &http.Server{Handler: handler}, manager: manager}
return a, nil
```

App 结构体加字段 `manager *syncmod.Manager` 与生命周期 context:

```go
type App struct {
	cfg     *config.Config
	srv     *http.Server
	addr    string
	manager *syncmod.Manager
	cancel  context.CancelFunc
}
```

`Start`:HTTP 监听成功后启动 manager:

```go
func (a *App) Start(addr string) (string, error) {
	// ...现有监听逻辑...
	a.addr = ln.Addr().String()
	go func() { _ = a.srv.Serve(ln) }()

	if a.manager != nil {
		ctx, cancel := context.WithCancel(context.Background())
		a.cancel = cancel
		a.manager.Start(ctx)
	}
	return a.addr, nil
}
```

`Shutdown`:先停 manager 再关 HTTP:

```go
func (a *App) Shutdown() error {
	if a.cancel != nil {
		a.cancel()
	}
	if a.manager != nil {
		a.manager.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.srv.Shutdown(ctx)
}
```

- [ ] **Step 3: 验证编译 + 全量后端测试** `go build ./...` 后 `go test ./...`(backend),预期编译通过、原有测试全绿。

- [ ] **Step 4: 提交** commit `feat(flymail): 装配 SSE Hub + 同步管理器（启动/优雅关闭）`。

---

## Phase E:前端实时

### Task E1: lib/sse.ts + useRealtimeSync

**Files:**
- Create: `flymail/frontend/src/lib/sse.ts`
- Create: `flymail/frontend/src/hooks/useRealtimeSync.ts`
- Modify: `flymail/frontend/src/lib/types.ts`(加 `RealtimeEvent`)

- [ ] **Step 1: types.ts 加类型**

```ts
export interface RealtimeEvent {
  type: 'new_mail'
  account_id: number
  folder_id: number
  new_count: number
}
```

- [ ] **Step 2: lib/sse.ts**

EventSource 封装:用当前 access token 建连 `/api/v1/events?access_token=...`;`onerror` 时关闭并尝试用 refresh 刷新 token 后重连(指数退避,上限 30s);返回 `close()`。token 取自 `@/lib/auth`(`auth.access`),刷新复用 `/auth/refresh`(裸 fetch,避免与 axios 拦截器耦合)。

```ts
import { auth } from '@/lib/auth'
import type { RealtimeEvent } from '@/lib/types'

// 连接 SSE 流；onEvent 收到解析后的事件。返回关闭函数。
export function connectRealtime(onEvent: (ev: RealtimeEvent) => void): () => void {
  let es: EventSource | null = null
  let closed = false
  let backoff = 1000

  async function refreshToken(): Promise<boolean> {
    const rt = auth.refresh
    if (!rt) return false
    try {
      const res = await fetch('/api/v1/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: rt }),
      })
      if (!res.ok) return false
      const data = (await res.json()) as { access_token: string; refresh_token: string }
      auth.set(data.access_token, data.refresh_token)
      return true
    } catch {
      return false
    }
  }

  function open() {
    if (closed) return
    const token = auth.access
    if (!token) return
    es = new EventSource(`/api/v1/events?access_token=${encodeURIComponent(token)}`)

    es.onmessage = (e) => {
      try {
        onEvent(JSON.parse(e.data) as RealtimeEvent)
      } catch {
        /* 忽略心跳/非 JSON */
      }
    }

    es.onerror = () => {
      es?.close()
      es = null
      if (closed) return
      // 可能是 token 过期：先尝试刷新，再退避重连。
      void refreshToken().finally(() => {
        if (closed) return
        setTimeout(open, backoff)
        backoff = Math.min(backoff * 2, 30000)
      })
    }

    es.onopen = () => {
      backoff = 1000
    }
  }

  open()
  return () => {
    closed = true
    es?.close()
    es = null
  }
}
```

- [ ] **Step 3: hooks/useRealtimeSync.ts**

```ts
import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { connectRealtime } from '@/lib/sse'

// 订阅 SSE：收到新邮件事件后失效 folders/messages/sync-status，触发自动刷新。
export function useRealtimeSync(): void {
  const qc = useQueryClient()
  useEffect(() => {
    const close = connectRealtime((ev) => {
      if (ev.type === 'new_mail') {
        void qc.invalidateQueries({ queryKey: ['folders'] })
        void qc.invalidateQueries({ queryKey: ['messages'] })
      }
    })
    return close
  }, [qc])
}
```

- [ ] **Step 4: 类型检查** `pnpm -C flymail/frontend exec tsc --noEmit`(或项目既有 lint/typecheck 命令),预期通过。

- [ ] **Step 5: 提交** commit `feat(flymail-web): SSE 客户端 + useRealtimeSync`。

### Task E2: Shell 挂载 useRealtimeSync

**Files:**
- Modify: `flymail/frontend/src/pages/Shell.tsx`

- [ ] **Step 1: 在 ShellPage 顶部调用 hook**

```tsx
import { useRealtimeSync } from '@/hooks/useRealtimeSync'
// ...
export function ShellPage() {
  useRealtimeSync()
  // ...其余不变
```

- [ ] **Step 2: 类型检查** 同上,预期通过。

- [ ] **Step 3: 提交** commit `feat(flymail-web): Shell 挂载实时同步`。

### Task E3: 设置「轮询间隔」

**Files:**
- Modify: `flymail/frontend/src/lib/types.ts`(`AppSettings` 加 `sync_poll_interval`)
- Modify: `flymail/frontend/src/lib/queries.ts`(`useSettings` 读该值)
- Modify: `flymail/frontend/src/components/settings/MailSection.tsx`
- Modify: `flymail/frontend/src/locales/zh.json` + `en.json`

- [ ] **Step 1: types.ts** `AppSettings` 加 `sync_poll_interval: number`。

- [ ] **Step 2: queries.ts** `useSettings` 返回:

```ts
return {
  sync_depth: Number(data.settings?.sync_depth ?? 1000) || 1000,
  sync_poll_interval: Number(data.settings?.sync_poll_interval ?? 180) || 180,
}
```

- [ ] **Step 3: MailSection.tsx** 加一个轮询间隔数字输入(秒,min 30,max 3600),并入 `handleSave` 一起 `updateSettings.mutate({ sync_depth: ..., sync_poll_interval: ... })`。校验:30–3600,越界报 `settings.mail.invalidInterval`。state 初值与 `useEffect` 回填同 `sync_depth` 模式。

- [ ] **Step 4: i18n** zh.json `settings.mail` 加:`"syncInterval": "后台轮询间隔（秒）"`、`"syncIntervalHint": "范围 30–3600"`、`"invalidInterval": "轮询间隔需在 30–3600 之间"`;en.json 对应英文。**改完用 JSON 解析校验无重复 key、无引号内嵌。**

- [ ] **Step 5: 类型检查 + JSON 校验** `tsc --noEmit`;`node -e "require('./flymail/frontend/src/locales/zh.json');require('./flymail/frontend/src/locales/en.json')"` 或等价校验。

- [ ] **Step 6: 提交** commit `feat(flymail-web): 设置增加后台轮询间隔`。

---

## Phase F:文档 + 记忆

### Task F1: 已知问题 + 记忆

**Files:**
- Modify: `docs/superpowers/known-issues.md`
- Modify: memory `project_flymail_milestone1.md`

- [ ] **Step 1: known-issues.md 追加 KI-2**

> KI-2:SSE 端点 `GET /api/v1/events` 的 access_token 经 URL query 传递(EventSource 不能设自定义头)。自托管 localhost 下可接受;后续可改为一次性 stream ticket(短 TTL,POST 换取后 query 带 ticket)以避免 token 落 URL 日志。

- [ ] **Step 2: 记忆补 M6 条目**(项目记忆文件,简述:Manager 单连接 worker、IDLE+轮询+reconcile、IncrementalSync、SSE Hub/端点、前端 useRealtimeSync;只拉新邮件,状态回拉推后)。

- [ ] **Step 3: 提交** commit `docs(flymail): 登记 KI-2（SSE token 走 URL）+ M6 记忆`。

---

## 最终审查 + 收尾

- [ ] 全量后端测试 `go test ./...`(backend + core)全绿。
- [ ] 前端类型检查 + 构建 `pnpm build` 通过(确认 embed 资源可生成)。
- [ ] 派发最终 code-reviewer 子代理审查整支分支(重点:Manager goroutine 泄漏/优雅关闭、IDLE handle Stop 时序、SSE 断连清理、增量 newCount 准确性、token-in-URL)。
- [ ] 使用 superpowers:finishing-a-development-branch 收尾(合并回 main 并删分支)。
- [ ] 真机验证清单交用户:重启后端 → 不手动点同步,向自己发一封 → 数秒内(IDLE)或一个轮询周期内(163 兜底)收件箱自动出现新邮件且未读数自增 → 设置里调小轮询间隔验证生效。

## 已知限制 / 取舍

- **只拉新邮件**:服务器端被其他客户端标已读/删除不回拉(用户决策),下次全量同步才会以服务器为准。
- **每账户一连接**:IDLE 常驻 1 条连接;轮询其他文件夹复用同一连接(串行)。多账户 = 账户数条连接。
- **IDLE 单测不覆盖**:IDLE 路径靠真机 + 后续可加 env-gated 集成测试;单测用 `CanIDLE()=false` 覆盖轮询/调和。
- **SSE token 走 URL**:见 KI-2。
- **reconcile 延迟**:新增/启停账户最多 30s 后生效(reconcile 周期)。
