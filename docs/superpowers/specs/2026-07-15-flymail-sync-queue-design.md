# FlyMail 同步队列改造 设计文档

- 日期：2026-07-15
- 范围：后端加强三部曲第三项（日志✅、E2E✅ 均已合并 main）。改造 sync 的连接管理与任务调度。
- 状态：方向已与用户确认（规模 50–300 账户 / 每账户单连接持有者 / 回写队列入库）。
- 回归保障：`./e2e.ps1` 四链路 E2E（收信/回写/发送·删除·移动/IDLE·SSE）必须全程全绿。

## 1. 现状与问题

当前同一账户的 IMAP 操作散落在 5 条互不知晓的路径，各自建连：

| 路径 | 连接行为 | 问题 |
|------|----------|------|
| Manager 后台 worker（manager.go） | 每 enabled 账户 1 goroutine + 1 常驻连接，IDLE↔轮询 | 连接数随账户数线性增长 |
| 手动触发（service.go Trigger/run） | 每次新开连接，只同步文件夹+INBOX | 与 Manager 可能同账户并发同步 |
| 详情按需抓正文（MessageDetail） | 每次点击新开连接 | 交互延迟 = TLS 握手 + 登录 |
| 附件下载（AttachmentContent） | 每次新开连接 | 同上 |
| 回写 worker（writeback.go） | 全局单 worker，每 op 新开连接 | 队列内存 256 满则静默丢弃；重试 3 次放弃 → 本地/服务器永久漂移 |

其余问题：
- 同步 Status 为内存 map，重启丢失（可接受，见 §6）。
- 脱机账户重连 backoff 封顶 60s，永久空转，无熔断。
- 无全局并发上限：重启后 N 个账户同时全量同步 → IMAP/SQLite/带宽雪崩。

## 2. 目标与非目标

**目标**
1. 支撑 50–300 个账户：连接数与并发受控、可配置。
2. 每账户所有 IMAP 操作（同步/触发/详情/附件/回写）统一走**一个连接持有者**串行执行，消灭临时建连与同账户并发冲突。
3. 回写操作持久化（SQLite），重启不丢、失败可退避重试、可观察。
4. 脱机/持续失败账户熔断降频，不再空转。
5. IDLE 实时体验保留，并可分级（常驻连接数上限可配）。

**非目标（YAGNI）**
- 不做多进程/分布式调度；单进程内解决。
- 不改 HTTP API 语义（trigger 202/409、status phase 值、SSE 事件格式全部不变）。
- 不做同步 Status 持久化（重启后下次同步自然重建，用户已确认可接受）。
- 不动 core/imap 协议层。

## 3. 核心设计：账户 Runner（Actor 模型）

### 3.1 AccountRunner

每个 enabled 账户一个 runner goroutine，**独占**该账户的 IMAP 连接（懒建连）：

```
                 ┌────────────── AccountRunner (goroutine) ──────────────┐
 前台任务队列 ──▶ │ select { 前台任务 > 后台任务 > IDLE 事件 > 轮询 tick } │──▶ 唯一 IMAP 连接
 后台任务队列 ──▶ │  任务自带 SelectFolder，串行执行互不干扰                │
                 └───────────────────────────────────────────────────────┘
```

- **两级任务队列**：
  - 前台（interactive）：详情抓正文、附件下载、手动触发同步。用户在 HTTP 请求里等，优先执行。
  - 后台（background）：定时全文件夹同步、IDLE 唤醒的 INBOX 同步、回写 op。
- **任务模型**：`task{kind, run func(Session) error, reply chan error}`。前台任务带 reply channel，HTTP handler 投递后带超时等待（默认 20s，超时返回 502/504，任务仍会执行完）。
- **协作式让位**：全文件夹同步在**每个文件夹之间**检查前台队列，有则先执行前台任务再继续（同一连接，SELECT 由各任务自理）。
- **连接生命周期**：
  - 懒建连：队列有任务且无连接时建连。
  - IDLE 档账户：空闲时保持连接并进入 IDLE（现有 29min 刷新逻辑保留）。
  - 非 IDLE 档 / 服务器不支持 IDLE：一轮任务处理完后空闲 60s 关闭连接，下轮任务/轮询 tick 再重建（延续现有 163 经验：不持有空闲连接）。
  - 连接错误：关闭连接、当前任务失败（回写任务转入退避重试，见 §5），指数退避重连（1s→60s 封顶，保留现状）。
- **熔断**：连续失败 ≥5 次 → 熔断 10 分钟：后台轮询暂停、不再自动重连；**前台任务仍可尝试**（用户主动操作给一次立即恢复的机会，成功则清零熔断）。熔断状态暴露给监控 API。

### 3.2 全局调度（Manager 改造）

Manager 职责收缩为 runner 生命周期管理 + 全局资源闸门：

- **reconcile**（保留 30s 间隔 + Start 立即一次）：enabled 账户集合 ↔ runner 集合对齐。
- **全局同步信号量**：同时执行「全文件夹同步」的 runner 数上限，默认 8（setting 可配 `sync_max_concurrent`）。runner 执行全量同步前获取、完成后释放；前台任务与回写**不受限**（轻量、用户在等）。
- **IDLE 分级**：常驻 IDLE 连接数上限，默认 100（setting 可配 `sync_max_idle_conns`）。超额账户自动降为「轮询+每轮重连」模式。名额分配：按账户 id 稳定排序（简单、可预测；后续需要再演进为按活跃度）。
- **轮询错峰**：各 runner 首轮同步加 `hash(accountID) % pollInterval` 的随机相位，避免重启后 300 账户同时开跑（信号量之外的平滑手段）。

### 3.3 手动触发与 Status 合流

- `Trigger(accountID)` 改为向 runner 前台队列投递「全量同步任务」；runner 内部保证同账户单飞（正在同步则返回 ErrSyncRunning → HTTP 409，语义不变）。
- Status 仍为内存 map，但**唯一写入方是 runner**（消除 Trigger 路径与 Manager 路径各写各的问题）；后台定时同步同样更新 Status（前端监控页从此能看到后台同步进度，现状只显示手动触发的）。
- `sync.Service` 保留对外门面（Trigger/StatusOf/MessageDetail/AttachmentContent/SetRead...），内部改为投递任务，公开 API 不变。

## 4. 详情/附件按需抓取

`MessageDetail` / `AttachmentContent` 不再自行 Dial：封装为前台任务投递到对应账户 runner，复用其连接。收益：点开详情从「TLS+登录+FETCH」变为「FETCH」；断网/熔断时快速失败且错误统一。

## 5. 回写队列持久化

**表 `writeback_ops`**（gorm AutoMigrate 追加）：

| 列 | 说明 |
|----|------|
| id | 主键 |
| account_id / folder_path / uid | 定位邮件 |
| op | read / unread / star / unstar（delete/move 已即时执行，不入此队列，维持现状） |
| attempts | 已尝试次数 |
| next_attempt_at | 退避后的下次尝试时间 |
| last_error | 最近失败原因（可观察性） |
| created_at | |

**流程**：
1. API 标已读/星标：本地写库（乐观，现状不变）→ **插入 writeback_ops** → 投递后台任务到 runner。
2. runner 执行成功 → 删除行。
3. 失败 → attempts++、next_attempt_at = now + 2^attempts × 30s（封顶 30min）、记 last_error；由 runner 的轮询 tick 顺带捞取到期 op 重试。
4. attempts ≥ 8 → 标记放弃（删除行 + Warn 日志 + notify 站内通知「回写失败」）；下次全量同步会以服务器状态覆盖本地（现状兜底逻辑保留）。
5. **启动恢复**：runner 启动时加载该账户未完成 op 入队。

替换现有 `wbCh chan wbOp`（256 缓冲、满则丢）与全局单 worker。

## 6. 明确不做（已确认）

- Status 持久化入库：重启丢失可接受。
- delete/move 进持久队列：这两个动词当前为同步执行、失败直接报给用户（HTTP 502），交互语义更好，不改。

## 7. 兼容与风险

- **接口兼容**：`Session` 接口、HTTP API、SSE 事件格式、E2E 测试全部不变；改造完成的定义是 `./e2e.ps1` 9/9 全绿 + 全量单测通过。
- **风险 1 — IDLE 打断**：runner 在 IDLE 等待中收到队列任务需先 `handle.Stop()` 再执行（现有 idleCh 模式扩展为「任务队列也是唤醒源」）。Stop 失败按现状视为脏连接重建。
- **风险 2 — 前台任务超时后的孤儿执行**：HTTP 已超时返回但任务仍在 runner 执行，结果丢弃。可接受（幂等操作）。
- **风险 3 — SQLite 写放大**：writeback_ops 每 op 一插一删。规模 300 账户下回写频率低（人工操作），忽略。
- **风险 4 — 信号量饿死**：长同步占满 8 个名额时新账户首同步排队。前台触发的全量同步同样受限但用户可从 status 看到 pending 阶段（新增 phase `queued`，向后兼容——前端未识别的 phase 按进行中展示）。

## 8. 交付物

1. `modules/email/sync/runner.go` — AccountRunner（两级队列/懒建连/IDLE/熔断/协作让位）。
2. `modules/email/sync/manager.go` — 改造为 runner 生命周期 + 信号量 + IDLE 名额 + 错峰。
3. `modules/email/sync/writeback.go` — 持久化队列（表+repo+退避+启动恢复），删除 chan 方案。
4. `modules/email/sync/service.go` — 门面改投递；MessageDetail/AttachmentContent 走前台任务。
5. setting 新增 `sync_max_concurrent`（默认 8）、`sync_max_idle_conns`（默认 100）。
6. 单测（fake Session 驱动 runner 状态机）+ E2E 回归全绿；新增 E2E：重启恢复回写（进程内关 app→同库起新 app→断言回写最终生效）。
