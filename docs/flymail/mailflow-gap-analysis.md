# FlyMail vs MailFlow 功能对比分析

> 分析日期：2026-08-11
> 对比基线：`ref/mailflow`（README 记录至 v2.7.0，数据库迁移至 `0040_smtp_separate_auth.sql`）
> 我方基线：`flymail/` main 分支（同步队列 M-sync 已合并，含运行时诊断面板）

本文是一次**现状快照**，写完后原则上不再滚动更新；持续演进的任务清单见 [`roadmap.md`](roadmap.md)。

---

## 1. 对比基准的前提：两者定位不同

| | FlyMail | MailFlow |
|---|---|---|
| 形态 | 单人自部署 + 可桌面化客户端 | 多用户服务端 webmail |
| 技术栈 | Go + React + Wails | Node.js 22 + React |
| 存储 | SQLite 单文件 | PostgreSQL 16 + Redis 7 |
| 部署 | 单进程二进制 / 桌面应用 | Docker Compose（nginx + backend + pg + redis） |
| 阶段 | 核心功能构建期 | 生态整合期（CardDAV / GTD / AI / Todoist） |

这个差异决定了对比结论必须分成两类：

- **必备能力缺口** —— 任何邮件客户端都要有的（会话线程、全文搜索、规则），是真实欠账；
- **定位差异项** —— 多用户 / 邀请 / SSO / Redis 会话，对 FlyMail 未必是缺陷。

下文所有结论都按这个界线归类。

---

## 2. 总体评估

**FlyMail 的同步引擎质量高于 MailFlow**，但**邮件应用的"使用面"约完成 55%**：收发读管的骨架齐备，组织与检索能力接近空白。

| 能力域 | FlyMail | MailFlow | 判定 |
|---|---|---|---|
| IMAP 同步引擎 | ★★★★★ | ★★★☆☆ | **我方领先** |
| 收（多账户 / 文件夹 / 聚合） | ★★★★☆ | ★★★★★ | 接近 |
| 读（正文 / 附件 / HTML 渲染） | ★★★☆☆ | ★★★★★ | 缺会话线程、追踪像素防护 |
| 写（撰写 / 回复 / 草稿） | ★★★☆☆ | ★★★★★ | 编辑器简陋、无别名 |
| 管（组织 / 自动化） | ★☆☆☆☆ | ★★★★★ | **最大缺口** |
| 找（搜索） | ★★☆☆☆ | ★★★★★ | 无索引、无语法、无服务端兜底 |
| 安全与账户体系 | ★★☆☆☆ | ★★★★★ | 无 OAuth2 接入 / 2FA / 限流 / 净化 |
| 通知与集成 | ★★★★☆ | ★★★☆☆ | **我方领先**（IM 多渠道），缺 Web Push |
| 可观测性 | ★★★★★ | ★★☆☆☆ | **我方明显领先** |
| 分发部署 | ★★☆☆☆ | ★★★★★ | 无面向他人的分发路径 |

---

## 3. 我方领先的部分

### 3.1 IMAP 同步引擎

FlyMail 采用单账户 Actor 模型（`flymail/backend/modules/email/sync/runner.go`），每账户独占一条 IMAP 连接并串行执行任务，具备：

- **前台/后台双队列 + 严格前台优先**：用户操作（打开邮件、标记）不被后台全量同步阻塞，全量同步在文件夹边界主动让位（`drainForeground`）
- **IDLE ↔ 轮询自动降级**：`IDLEAllowed` 控制常驻 IDLE 名额，超额账户降为轮询
- **29 分钟 IDLE 刷新**：规避 RFC 建议的连接超时
- **熔断 + 指数退避**：建连失败 1s→60s 封顶（`onDialFailure`），熔断打开时拒绝后台任务但放行前台
- **轮询错峰**：`pollPhase` 用 `hash(accountID) % pollInterval` 避免重启后账户同时开跑
- **懒建连 + 空闲关闭**：60s 无任务自动断开
- **运行时诊断**：模式机 + 事件环形缓冲 + 诊断接口（`sync/diag.go`），前端有诊断面板

MailFlow 对应的 `backend/src/services/imapManager.js` 没有这套调度模型。

### 3.2 可观测性

`/monitoring/overview`、`/monitoring/accounts`、`/monitoring/accounts/:id/diagnostics` 三个端点 + 前端 `MonitoringSection.tsx`，暴露待回写量、熔断状态、队列深度、IDLE 三态、事件时间线。MailFlow 无对等能力。

### 3.3 IM 通知派发

`modules/system/notify/` 提供多渠道推送（Telegram / 企业微信 / 飞书 / 钉钉，自 mail2im 迁移），含渠道管理、连通性测试、派发日志。MailFlow 只有 WebSocket 提示 + Web Push。

---

## 4. 缺口详解

### P0 — 核心体验硬伤

#### 4.1 没有会话线程视图

数据层已就位但完全未被使用：

- `modules/email/message/model.go` 已有 `MessageID`（索引）、`InReplyTo`、`References`、`ThreadID`（索引）
- 后端**无 thread 聚合查询**，`/folders/:fid/messages` 与 `/aggregate/messages` 均按单封平铺返回
- 前端 `Reader.tsx` 仅有 `thread-msg` / `thread-head` / `thread-body` 的 CSS 类名，渲染的仍是单封邮件；`MailList.tsx` 无折叠逻辑

结果：一次十几封往返的讨论在列表里占十几行，读一封回复看不到上下文。这是与现代邮件客户端最直观的体验落差。

MailFlow 侧：迁移 `0002_subject_threading`、`0006_threaded_query_indexes`、`0007_thread_paging_index`、`0009_thread_key_column` 四个版本专门处理线程，前端消息组内联展示已发送回复。

#### 4.2 搜索是全表 LIKE 扫描

`modules/email/message/repository.go:42` `SearchMessages` 的实现：

```
LEFT JOIN message_bodies ON message_bodies.message_id = messages.id
WHERE subject LIKE '%q%' OR from_name LIKE '%q%' OR from_addr LIKE '%q%'
   OR snippet LIKE '%q%' OR message_bodies.text_body LIKE '%q%'
```

三层问题：

1. **前缀通配符使所有索引失效** —— 每次搜索都是 messages 全表扫 + message_bodies 全正文 JOIN 扫。邮件量上万后延迟不可接受
2. **无搜索语法** —— 不支持 `from:`、`subject:`、`has:attachment`、`is:unread`、`before:` 等限定符
3. **只搜本地已同步内容** —— 正文未同步（`body_synced = false`）的历史邮件搜不到，且无 IMAP `SEARCH` 兜底

SQLite 自带 FTS5，改造成本远低于收益。

MailFlow 侧：迁移 `0008_search_indexes`、`0018_contacts_search_indexes`，独立的 `routes/search.js` 与三个测试文件。

#### 4.3 撰写器能力不足

前端使用 `react-simple-wysiwyg@3.4.1` —— 一个仅数百行的极简组件。当前 `ComposeDialog.tsx` 648 行。

缺失项：签名、引用折叠、内联图片、粘贴保留格式、表格、字号/颜色。MailFlow 提供完整 WYSIWYG（字体族、字号、颜色、高亮、表格、emoji、链接、附件、图片缩放手柄、Excel 表格粘贴）。

对"正式回复"场景（自部署邮件共享的主要用途），至少需要签名与引用折叠。

---

### P1 — 组织与自动化（完全空白）

FlyMail 目前**没有任何自动化规则能力**。MailFlow 在此有完整一层：

| 能力 | MailFlow 实现 | FlyMail |
|---|---|---|
| 收件箱规则 | `routes/rules.js` + `services/inboxRules.js`，条件涵盖发件人/主题/收件人/头/正文/附件，动作涵盖移动/归档/删除/标记已读/加星/转发（`0010_inbox_rules`、`0039_inbox_rule_forwards`） | 无 |
| 黑名单 | `routes/blockList.js`，命中直接进垃圾箱，**在规则之前**执行（`0013_block_list`） | 无 |
| 邮件分类 | `services/categorizer.js`，基于头部检测 + 发件人启发式分为 Primary/Newsletters/Social/Notifications/Other（`0023_message_categories`、`0024_category_improvements`），非 AI | 无 |
| 一键退订 | 解析 `List-Unsubscribe`，消息面板按钮（`0025_unsubscribe_state`） | 无 |
| Snooze | 稍后提醒，到点回到收件箱顶部 | 无 |
| 垃圾邮件标记 | 右键/工具栏/批量标记（`0021_spam_training`） | 无 |

**对"多账户混流"场景，规则引擎 + 黑名单价值最高** —— 它是把多个账户合并后的收件箱变成可用视图的前提。目前 FlyMail 已有聚合收件箱（`/aggregate/messages`），但没有任何过滤手段，账户越多噪音越大。

---

### P2 — 安全与账户体系

| 项 | FlyMail 现状 | 风险 | 分类 |
|---|---|---|---|
| **HTML 邮件净化** | **仅 iframe sandbox，无服务端 sanitize** | 脚本被 sandbox 挡住，但**远程图片（追踪像素）与 CSS `url()` 外链未剥离**，每次打开邮件都在向发件人回报"已读 + IP + 时间" | 必备缺口 |
| OAuth2（Gmail / M365） | `core/imap/xoauth2.go` 有底层能力，**backend 无授权流程与 token 刷新** | 微软已停用基本认证，Outlook.com / M365 账户接不进来 | 必备缺口 |
| 登录限流 | 无（`modules/auth/` 无限流代码） | 暴露公网时可被暴力破解 | 必备缺口 |
| iframe sandbox 组合 | `Reader.tsx:548` 使用 `sandbox="allow-same-origin allow-popups"` + `srcDoc` | 同源 srcDoc 下 `allow-same-origin` 弱化了沙箱隔离。当前**未开 `allow-scripts` 所以仍安全**，但这是易碎组合：将来若为交互加上 scripts 会立即变成逃逸漏洞 | 需加固 |
| 2FA / SSO / OIDC | 无 | 单人自用可接受 | 定位差异 |
| 多用户 / 邀请 / 权限 | 单管理员（`auth/model.go` 的 `AdminUser`） | 与桌面化定位一致 | 定位差异 |

**追踪像素是最该优先处理的一项** —— 它不是理论风险，而是每天都在实际发生的隐私泄露。

---

### P3 — 分发部署

MailFlow 提供：GHCR 预构建镜像、三种安装方式（预构建 / 源码构建 / 原生无 Docker）、Caddy 自动 HTTPS 覆盖层、nginx 与 systemd 配置模板、备份恢复文档、版本固定与升级说明。

FlyMail 目前只有 `flymail/dev.ps1`（开发用途，默认仅监听 127.0.0.1），**没有任何面向他人的分发路径**。这直接决定了它当前只能作者自用。

---

## 5. 明确不跟进的项

| 项 | 理由 |
|---|---|
| 多用户 + 邀请 + 用户管理 | 与"单人自部署 + 桌面端"定位冲突，会把权限模型、数据隔离、会话管理的复杂度拉高一个量级 |
| SSO / OIDC | 同上，依附于多用户体系 |
| AI 助手（摘要 / 起草 / 问答） | MailFlow 的差异化尝试，投入大、收益取决于个人习惯，且引入外部 API 依赖 |
| Todoist 集成 / GTD 工作流 | 长尾功能，绑定特定工作方法论 |
| CardDAV 服务端 | 把联系人簿暴露为 CardDAV 是独立产品方向，非邮件客户端核心 |
| PostgreSQL + Redis | **SQLite 单文件是 FlyMail 的优势**：零外部依赖、备份即拷贝、可直接打包进 Wails 桌面端。不应跟进 |

---

## 6. 结论

按"可感知收益 ÷ 实现成本"排序，最该先做的三件事：

1. **检索重构**（FTS5 + 搜索语法）—— 性能与体验双重瓶颈，改动集中在 message 模块内部，不触碰已稳定的同步引擎
2. **会话线程**（数据层已就位，只差聚合查询与前端折叠）—— 投入产出比最高的体验提升
3. **规则引擎 + 黑名单**（含追踪像素阻断）—— 让聚合收件箱真正可用，同时补上最实际的隐私缺口

完整分阶段计划见 [`roadmap.md`](roadmap.md)。
