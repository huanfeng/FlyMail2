# M17 账户健康可观测性（故障推送 + 熔断展示）

状态：设计已批准（2026-09-28），待实现。

## 背景与问题

1. **Google OAuth 账号陆续失效**。根因：Google Cloud OAuth 应用处于 Testing
   发布状态时，refresh_token 每 7 天强制过期（每个用户从授予同意时起算），
   刷新返回 `invalid_grant`。`docs/flymail/m14-oauth.md` 已有记载。治本在
   Google Cloud Console → Google Auth Platform → 受众群体 → 发布为正式版；
   发布后需对每个 Google 账号重新授权一次（过期的 refresh_token 不复活）。
   本里程碑不解决 Google 侧的过期，解决的是失效后的可观测性。

2. **账号故障后毫无提示**：后台轮询/IDLE 失败只记诊断日志和熔断计数，
   从不发通知；`sync_failed` 仅手动同步失败路径发出。用户几天未收邮件
   也无人告知。

3. **熔断/故障在主界面无区分**：侧栏只认 enabled；账户数据已带 status
   字段但不渲染；needs_reauth/熔断徽标只在设置→监控页可见。

## 目标

1. 故障账户在几天不开界面的场景下也能收到外发推送（飞书/webhook）。
2. 故障持续期间每 24h 重复提醒；恢复时收到恢复通知。
3. 侧栏、全局横幅、账户管理页三处对故障账户有明确视觉区分。
4. needs_reauth 停止无效重试（30 分钟节流），减少无意义拨号与诊断噪音。

## 非目标

- 不自动禁用故障账户（用户决策权保留）。
- 不解决 Google Testing 7 天过期本身。
- 不改 sync_failed 手动路径现有行为。
- 不做故障通知的多级正文（短文案，不涉及 content_level 裁剪）。

## 方案（已批准：后端统一「账户健康出口」）

**health watcher**（新增 `flymail/backend/modules/email/sync/health.go`）：
挂在 sync.Manager 上，tick 60s 差分故障态集合：

- 故障态判据：runner 熔断打开（连续 5 次失败，天然防抖）或账户
  status=needs_reauth（OAuth 失效，终态）。
- 翻转沿：健康→故障 emit `sync_failed`（红卡片：账户名+原因+计数）；
  故障→健康 emit `account_status`（橙卡片：「同步已恢复」）。延迟 ≤1 分钟。
- 每 24h 重复：持续故障每 24h 重复推送一次，文案带「已持续 N 天」。
- 重启补推：watcher 内存故障集合初始为空，第一个 tick 的差分天然发现
  存量 needs_reauth 账户并补推，无需特判。
- 事件类型：故障=sync_failed（红），恢复=account_status（橙），全部复用
  现有类型与飞书配色，零新事件类型。
- 边界：故障期间账户被禁用→移出故障集合且不发恢复通知（SetEnabled
  已有自己的通知）；手动路径与 watcher 撞车概率低（单次失败不进熔断），
  双重推送可接受（信息不矛盾）。

**站内落库口径**（notify.Emit 加 WithInApp(false) 选项）：

- 熔断故障沿：落库 + 外发
- needs_reauth 故障沿：不另发通知（markNeedsReauth 已即时推送并落库；watcher
  只负责该账户的 24h 重复与恢复通知）
- 每日 24h 重复：只外发，不落库（防通知中心刷屏）
- 恢复通知：落库 + 外发
- 重启补推：第一个 tick 把存量熔断故障按新翻转沿补推（落库+外发）；needs_reauth
  存量不重复推送（通知中心已有 markNeedsReauth 的落库记录）

**needs_reauth 停止无效重试**：

- runner 后台路径收到 ErrNeedsReauth 后 30 分钟内不再拨号（节流），记一条
  diag 事件 needs_reauth（信息级）；前台路径（手动同步/发信）行为不变。
- 重新授权成功（状态翻回 ok）后自动恢复拨号。

**AccountResponse.health 注入**：

- `account/dto.go` 加 `Health *AccountHealthInfo`：
  `{ needs_reauth, breaker_open, failures, last_error }`。
- 来源：账户 Status（DB）+ Manager.RunnerStats()（内存熔断/计数/last_error）。
- 装配：app.go 组装 accounts handler 时注入 HealthFor(accountID) 函数
  （sync.Manager 实现）。空值/零值表示健康。

## 前端设计

### 侧栏账户行（AccountSidebar.tsx AccountBlock）

状态优先级：故障标记优先于置灰，可叠加（needs_reauth + disabled 时既显示
故障标记又置灰——故障信息不因停用而隐藏）。

- needs_reauth：橙色图标（lucide KeyRound）+「需重新授权」小字，点击打开
  账户编辑对话框（重新授权入口已有）
- breaker_open：红色图标（lucide AlertTriangle）+「同步异常」小字，
  title 悬停显示最近错误与失败次数
- disabled：整行置灰（opacity）
- 正常：现状不变

### 全局横幅（Shell.tsx 品牌栏下方）

- 数据源：['accounts'] 的 health 派生，无新轮询。
- 显示条件：存在 needs_reauth || breaker_open 账户时显示「N 个账户无法同步」，
  最多点名 2 个账户名，其余收进「等 N 个」。
- 点击 → 打开设置→监控页（settings-nav registry 的 monitoring 页面 ID）。
- 关闭按钮 + 记忆：存「已关闭的故障账户 id 集合」，同批故障不重现，
  新故障账户出现则重现；sessionStorage（同会话跨刷新，浏览器重启后重现）。

### 账户管理页（AccountsSection.tsx）

- 摘要行加「上次同步」相对时间 + 状态徽标（复用监控页 chip 样式）：
  needs_reauth 橙「需重新授权」/ breaker_open 红「同步异常」/ disabled 灰「已停用」。
- 启停徽标已有，并列显示。

### 桌面通知扩展（useRealtimeSync notify 分支）

- account_status/sync_failed 也弹桌面通知（无提示音，与 mail_new 区分），
  按账户+事件类型去重（防 watcher/手动路径撞车重复弹窗）；恢复通知也弹。
- 现有闸门不动。

### i18n

- 后端通知文案硬编码中文（与现状一致）：故障沿「同步异常：{账户名} 连续
  N 次失败——{错误}」；每日重复「同步异常持续：{账户名} 已持续 N 天无法
  同步——{原因}」；恢复「同步已恢复：{账户名}」。
- 前端 zh/en 各约 10 条，复用 settings.monitoring.breakerOpen/needsReauth 键。

## 测试计划

后端：
- watcher 单测：差分翻转沿、24h 重复（外发 only）、第一个 tick 存量补推、
  禁用边界、事件类型+落库口径。
- runner 测试：ErrNeedsReauth 30 分钟节流 + diag 事件。
- dto/装配测试：health 注入 + last_error 文案。
- notify WithInApp(false)：落库跳过、外发照常。

前端（vitest）：
- 侧栏三状态渲染（故障优先于置灰、可叠加）。
- HealthBanner：出现/点名/关闭记忆/新故障重现。
- 账户页徽标三态。
- useRealtimeSync notify 分支：故障/恢复弹桌面通知 + 失效 accounts。

端到端（GreenMail）：密码错误→5 次失败熔断→飞书红卡片+横幅+侧栏红警告+
账户页徽标；改回正确密码→恢复通知+UI 自动刷新。OAuth 端到端待真机凭据
（M14 以来的已知约束），needs_reauth UI 可用测试钩子/手推 DB 验证。

## 风险与边界

- watcher 与手动路径撞车重复推送可接受（信息不矛盾）。
- 30 分钟节流是常量调参，先不做 UI。
- needs_reauth + disabled 叠加显示的优先级已在设计锁定。

---
*设计对话确认记录（2026-09-28）：通知策略=状态变化+每 24h 重复+恢复通知；
UI=侧栏标记+全局横幅+账户页徽标；方案 A（后端统一健康出口）批准。*
