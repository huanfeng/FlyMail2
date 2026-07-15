# FlyMail 同步队列改造 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development 或 superpowers:executing-plans 逐任务实现。步骤用 `- [x]` 复选框跟踪。

**Goal:** 把 sync 的 5 条散落建连路径统一为「每账户单连接持有者（AccountRunner）」，加全局并发闸门与熔断，回写队列持久化。规模目标 50–300 账户。

**对应 spec:** `docs/superpowers/specs/2026-07-15-flymail-sync-queue-design.md`（**实现前必读**，本计划只列任务骨架）
**分支:** `feat/flymail-sync-queue`（基于 main）

## ⚠️ 执行约束

1. **E2E 是回归安全网**：`./e2e.ps1` 本机可全量跑（GreenMail JAR + Java，见 `flymail/backend/internal/e2e/README.md`）。Task 5/8/10 结束时必须全绿；中间任务至少编译 + 单测绿。
2. **不臆测**：改动前先读 manager.go / service.go / writeback.go 现状；HTTP API 语义（202/409/phase 值/SSE 格式）不得改变。
3. **每任务一提交**（中文 message）；`gofmt` 后再提交。
4. 单测用 fake Session（现有 `SetDial` 注入模式延续），不依赖真实 IMAP。

## Task 1: writeback_ops 持久化层

**Files:** `modules/email/sync/wbstore.go`（表模型+repo）、`wbstore_test.go`；`internal/database/database.go`（AutoMigrate 追加）

- [x] 表结构见 spec §5；repo 方法：`Enqueue`、`Delete`、`DuePending(accountID, now)`（next_attempt_at ≤ now）、`Fail(id, err, backoff)`、`PendingByAccount(accountID)`（启动恢复）、attempts≥8 放弃路径。
- [x] TDD：临时 SQLite 单测覆盖入队/到期捞取/退避/放弃。
- [x] Commit `feat(sync): 回写操作持久化存储(writeback_ops 表+repo)`

## Task 2: 熔断器

**Files:** `modules/email/sync/breaker.go`、`breaker_test.go`

- [x] 纯逻辑：连续失败 ≥5 → Open(10min)；前台任务可穿透尝试；成功即 Reset。暴露 `State()` 供监控。
- [x] Commit `feat(sync): 账户熔断器(连续失败降频+前台穿透)`

## Task 3: AccountRunner 骨架

**Files:** `modules/email/sync/runner.go`、`runner_test.go`

- [x] 任务模型 `task{kind, run, reply}`；前台/后台两级 chan；select 优先前台。
- [x] 懒建连（dial 注入）、空闲 60s 关闭（非 IDLE 档）、连接错误重建退避（1s→60s）。
- [x] fake Session 单测：任务串行、优先级、懒建连、空闲关闭、失败退避。
- [x] Commit `feat(sync): AccountRunner 骨架(两级队列+懒建连+退避)`

## Task 4: Runner 集成 IDLE↔轮询

**Files:** `runner.go`（迁移 manager.go 的 runSession 逻辑）

- [x] IDLE 等待期的唤醒源扩展为「idleCh + 前台队列 + 后台队列 + 轮询 tick + 29min 刷新」；进任务前 `handle.Stop()`，Stop 失败重建连接（现状语义保留）。
- [x] 全文件夹同步在文件夹边界让位前台任务（spec §3.1）。
- [x] 轮询错峰相位 `hash(accountID) % pollInterval`。
- [x] fake 单测：IDLE 被任务打断、让位时序。
- [x] Commit `feat(sync): Runner 接管 IDLE↔轮询循环(队列唤醒+协作让位)`

## Task 5: Manager 改造为 Runner 编排

**Files:** `manager.go`

- [x] reconcile 管 runner 生死；全局同步信号量（默认 8）；IDLE 名额（默认 100，按 id 排序）；`WorkerAccountIDs/CurrentPollSeconds` 等监控方法适配。
- [x] 单测迁移 + `./e2e.ps1` 全绿（此时详情/回写仍走旧路径，行为应不变）。
- [x] Commit `refactor(sync): Manager 改造为 Runner 生命周期编排(信号量+IDLE 名额+错峰)`

## Task 6: Trigger/Status 合流

**Files:** `service.go`

- [x] Trigger 投递前台全量同步任务；同账户单飞 → ErrSyncRunning（409 语义不变）；新增 phase `queued`（排队等信号量时）。
- [x] Status 唯一写入方为 runner；后台定时同步同样上报进度。
- [x] Commit `refactor(sync): 手动触发与后台同步合流到 Runner(status 统一上报)`

## Task 7: 详情/附件走前台任务

**Files:** `service.go`（MessageDetail / AttachmentContent）

- [x] 封装为前台任务投递，20s 超时；错误映射维持现状（502/404）。
- [x] `./e2e.ps1 -Run TestSync` 验证详情链路。
- [x] Commit `refactor(sync): 详情/附件按需抓取复用 Runner 连接`

## Task 8: 回写切换持久队列

**Files:** `writeback.go`（重写）、`service.go`（SetRead/SetFlagged 入库+投递）

- [x] 删除 `wbCh`/全局 worker；启动恢复 `PendingByAccount`；退避重试由轮询 tick 捎带；放弃时 notify。
- [x] `./e2e.ps1 -Run TestWriteback` 全绿。
- [x] Commit `feat(sync): 回写走持久队列+Runner 连接(重启恢复/退避/放弃通知)`

## Task 9: 设置项与监控

**Files:** `modules/system/setting`（常量+默认值）、`modules/system/monitoring`

- [x] `sync_max_concurrent`(8)、`sync_max_idle_conns`(100) 接入 settings API；监控暴露熔断状态、每账户队列深度、待回写数。
- [x] Commit `feat(sync): 并发/IDLE 上限可配 + 熔断与队列监控`

## Task 10: E2E 回归 + 新增重启恢复用例

**Files:** `internal/e2e/`（新增 `restart_test.go`）

- [x] 新 E2E：标记已读后立即 Shutdown app → 同 DataDir 起新 app → eventually 断言 IMAP `\Seen` 生效（验证持久队列恢复）。
- [x] `./e2e.ps1` 全套全绿 + `go test ./...` 全绿 + `go vet`。
- [x] 更新本计划执行结果记录；合并 main。

## 完成定义（DoD）

- 全部任务提交完成；`./e2e.ps1` ≥10 用例全绿（含新增重启恢复）；后端全量单测通过。
- 同一账户任意时刻 ≤1 条 IMAP 连接（详情/附件/回写不再新建连接）。
- 熔断/并发上限/IDLE 名额可配可观测。

## 执行结果（2026-07-15 完成）

分支 `feat/flymail-sync-queue`，10 个任务每任务一提交（gofmt + 编译/单测绿后提交）。

| Task | 提交 | 关键实现 |
|------|------|----------|
| 1 | writeback_ops 表+repo | `wbstore.go`：入队/到期捞取/退避(2^n×30s 封顶 30min)/放弃(≥8)/启动恢复；迁移经 `MigrateWriteback`（不入 `database.Migrate` 以避测试期 import cycle） |
| 2 | 账户熔断器 | `breaker.go`：连续失败 ≥5 开 10min，前台穿透，成功即复位，可注入时钟 |
| 3 | AccountRunner 骨架 | `runner.go`：两级队列(前台优先)、懒建连、空闲 60s 关连接、连接退避 1s→60s |
| 4 | Runner 接管 IDLE↔轮询 | 统一 select 折入 IDLE/轮询/刷新/队列唤醒；文件夹边界 `yield` 让位前台；错峰相位 |
| 5 | Manager→Runner 编排 | reconcile 管生死、全局同步信号量(非阻塞 try+5s 重试)、IDLE 名额(按 id 排序)、Manager 实现 `runnerHost` |
| 6 | 触发/Status 合流 | Service 经 `orchestrator` 投递；`statusStore` 由 Service/Manager 共享；新增 `queued` 相位 |
| 7 | 详情/附件走前台任务 | `MessageDetail`/`AttachmentContent` 封装为前台任务(20s 超时)复用 runner 连接 |
| 8 | 回写走持久队列 | 删 `wbCh`/全局 worker；本地乐观写→入库→runner 执行(删行/退避/放弃通知)；轮询 tick 捎带重试；启动恢复 |
| 9 | 设置项与监控 | `sync_max_concurrent`(8)/`sync_max_idle_conns`(100) 接入 settings；监控暴露熔断/队列深度/待回写数 |
| 10 | E2E 重启恢复 | `restart_test.go`：注入待回写 op→同 DataDir 重启→断言 IMAP `\Seen` 恢复生效 |

验证：`./e2e.ps1` 10/10 全绿（含 `TestRestart_WritebackRecovery`、`TestRealtime_IDLESSE` 真实 IDLE）；`go test ./...`（含 `-race`）全绿；`go vet` 干净。

> 说明：delete/move 按 spec §6 保持即时同步（不入持久队列），仍为独立短连接，符合 DoD 括注范围。
