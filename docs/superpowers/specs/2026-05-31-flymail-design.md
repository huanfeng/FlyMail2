# FlyMail 设计方案（V1 = MVP，架构为完整版预留）

- 日期：2026-05-31
- 状态：已通过 brainstorming 评审，待写实现计划
- 范围：FlyMail —— 自托管、单管理员、多邮箱账户的完整 Web 邮箱客户端，可售卖共享软件

---

## 1. 定位与边界（已锁定决策）

| 维度 | 决策 |
|---|---|
| 产品定位 | 自托管、单管理员、多邮箱账户的完整邮箱客户端；定位为可售卖共享软件 |
| 代码起点 | **保留模块划分与三栏 UI 思路，后端逻辑 + 前端组件大幅重写**，全面对齐 `core` |
| 前端 | **React + Vite + TypeScript + shadcn/ui**（统一到 mail2im `frontend-react` 技术栈） |
| 发行形态 | **一份代码、两种构建**：① Web/Docker（纯 Go，无 CGO，静态镜像）② 桌面 Wails（Windows/macOS/Linux，CGO + 原生 WebView）。DB 用 glebarez（纯 Go），server 目标 `CGO_ENABLED=0`，桌面因 WebView 需 CGO（见 §3） |
| 前端复用 | 桌面端前端走 HTTP 访问同一 gin 后端，**浏览器与 WebView 中字节级一致，零分叉** |
| 原生能力 | 托盘 / 系统通知 / 开机自启，**全部在 Go 侧实现**，由后端事件驱动 |
| 存储策略 | **本地全量缓存到 SQLite**：元数据 + 正文落库，**附件按需**拉取缓存 |
| V1 闭环 | 管理员登录 → 加邮箱账户 → 收取/阅读 → 回复/写信发送 → 基础文件夹 |
| 会话聚合 | MVP **平铺列表**，schema 预留 `message_id/in_reply_to/references/thread_id` |
| OAuth | MVP 用**密码 / 应用专用密码**；OAuth 后续复用 mail2im 中央代理 |
| License | MVP **不集成，留接口位**（stub 恒返回有效） |

### 与 mail2im / core 的关系
- `core/`（module: `flymail-core`）已完成协议层：IMAP v2 会话、UTF-7、中文编码、XOAUTH2、SOCKS5 代理、SMTP、parser、types。FlyMail 建在 core 之上。
- mail2im 是"邮件转 IM 转发"工具；FlyMail 是"完整邮箱客户端"。两者共享 core，互不依赖。
- FlyMail 的真正难点不在"怎么连 IMAP"（core 已解决），而在"**怎么把邮件高效、增量、可离线地组织进本地 SQLite**"。

---

## 2. 后端架构（module: `flymail`，建在 `core` 之上）

```
flymail/
├── backend/                 # module: flymail（单模块，两个入口）
│   ├── cmd/flymail/         # ① server 目标 → CGO_ENABLED=0 → Docker/Web
│   ├── cmd/desktop/         # ② Wails 目标 → CGO_ENABLED=1 → 桌面
│   │                        #    main: wails.Run → 内嵌 internal/app(127.0.0.1)
│   │                        #    + 托盘 / 系统通知 / 开机自启
│   ├── internal/
│   │   ├── app/             # ★服务生命周期，两个入口共用；装配 gin 并返回 http.Handler
│   │   ├── config/          # viper 多层级（默认值/文件/env/flag）+ 数据目录按形态解析
│   │   └── server/          # 路由装配 + 中间件（core/httputil）
│   ├── modules/
│   │   ├── auth/            # 管理员登录 JWT + 刷新（core/auth）
│   │   ├── email/
│   │   │   ├── account/     # 账户 CRUD + 连接测试 + 凭证 AES 加密（core/crypto）
│   │   │   ├── folder/      # 文件夹同步与类型映射（core/imap + types.ClassifyFolder）
│   │   │   ├── message/     # 邮件元数据/正文持久化、读取、标记、移动、删除
│   │   │   ├── sync/        # ★同步引擎：首同步 + 增量 + flag 回写 + 断点续传
│   │   │   ├── send/        # SMTP 发送/回复/转发/草稿（core/smtp）
│   │   │   └── protocol/    # core/imap、core/smtp 的薄适配
│   │   ├── attachment/      # 附件按需下载 + 本地缓存
│   │   ├── realtime/        # SSE 推送新邮件 / 同步状态
│   │   ├── system/
│   │   │   ├── setting/     # 系统设置
│   │   │   └── task/        # 定时任务（定时收信 / 自动备份）
│   │   └── license/         # 接口位 stub（恒返回有效）
│   └── pkg/                 # flymail 专属薄封装；能下沉的能力进 core
└── frontend/                # React + shadcn/ui，构建产物 go embed 进 backend，两形态共用
```

### 关键架构原则
- **单一真相源 = gin router**：无论 Web 还是桌面，都是同一个 gin 实例提供 SPA 静态资源 + `/api/v1` + `/events`(SSE)。Server 目标把它绑到 TCP 监听；桌面目标绑到 `127.0.0.1` 随机端口，Wails WebView 导航过去。
- **`cmd/desktop` 与 `cmd/flymail` 同属一个 Go module**（因要 import `internal/app`）。Wails 依赖虽进 `go.mod`，但 `CGO_ENABLED=0 go build ./cmd/flymail` 只编译 server 入口，不触碰 webview 代码，Docker 镜像保持干净。
- **原生能力全部在 Go 侧实现**，由后端事件驱动，不需要前端配合，从而前端零分叉。唯一可接受的前端"感知"是只读标志 `window.__FLYMAIL_DESKTOP__`，仅用于极少数 UX 微调（如"最小化到托盘"提示），不影响业务逻辑。

---

## 3. 数据模型（GORM + SQLite 经 `core.OpenSQLite`，glebarez 纯 Go 无 CGO）

> **DB 驱动决策（2026-05-31）**：调查发现 core 原用的 mattn 驱动是建 core 时的未审视默认值，违背 flymail 原始 DESIGN.md「用 glebarez 避免 CGO」的要求。**决定迁移 core/database 到 `glebarez/sqlite`**（纯 Go，底层 modernc）。收益：两产品 DB 层统一且无 CGO、server/Docker 可 `CGO_ENABLED=0` 出 scratch/alpine 静态镜像、FTS5 开箱即用（modernc 内置，无需构建标签）。桌面 Wails 仍需 CGO（因 WebView，与 DB 无关）。迁移含 mail2im 回归，详见实现计划 Task 0。

- **AdminUser**：单管理员，bcrypt 密码、刷新 token。
- **Account**：`name / email / imap+smtp 配置（凭证 AES 加密） / proxy / auth_type / status / last_sync_at`。
- **Folder**：`account_id / path / display_name / type / uidvalidity / uidnext / last_seen_uid / unread_count / total_count`。
- **Message**：`account_id / folder_id / uid / message_id / in_reply_to / references / thread_id(预留) / subject / from / to / cc / date / flags(seen/flagged/answered/deleted) / snippet / has_attachment / size / body_synced(bool)`。
- **MessageBody**：`message_id / text_body / html_body`（**单独分表**，避免列表查询把大正文一起读出）。
- **Attachment**：`message_id / filename / mime / size / content_id / cached_path(nullable) / downloaded(bool)`。
- **Setting / Task**。
- **全文搜索**：SQLite **FTS5**（glebarez 底层 modernc 内置 FTS5，无需构建标签；若个别场景不可用则降级 LIKE）。

---

## 4. 同步引擎（核心）

### 方案 A（采用）—— UID 增量 + IDLE/轮询触发
- 每文件夹持久化 `UIDVALIDITY / UIDNEXT / last_seen_uid`。
- **首同步**：抓元数据（ENVELOPE + FLAGS），不抓正文。
- **增量**：
  - 若 `UIDVALIDITY` 变化 → 该文件夹重建。
  - 否则 `FetchByUIDRange(last_seen_uid+1, *)` 拉新邮件；对已知 UID 区间 fetch FLAGS，同步标记/删除变化。
- **正文**：用户打开邮件时按需 fetch 并落库（置 `body_synced=true`）。
- **触发**：IMAP IDLE 在线触发；轮询兜底（定时任务）。
- 优点：兼容所有 IMAP 服务器、可断点续传、流量小。代价：标记同步需做区间比对。

### 已否决/延后
- 方案 B（纯定时全量 diff）：实现简单但慢、重、费流量。❌
- 方案 C（CONDSTORE/QRESYNC）：增量最优，但 core 未实现且服务器支持不一。**列为后续演进**。

### 标记回写
- 本地操作（已读/星标/删除/移动）先写本地、即时反映 UI，再异步通过 `core/imap` 的 `MarkRead/MarkStarred/Delete/...` 回写服务器；失败进入重试队列。

---

## 5. API（OpenAPI `api/v1`）

- `auth`：login / refresh / logout
- `accounts`：CRUD + test-connection
- `folders`：list（每账户 + 虚拟文件夹聚合）
- `messages`：list（分页/过滤/搜索）/ detail / mark / move / delete
- `send`：send / reply / forward / draft
- `attachments`：download
- `sync`：trigger / status
- `events`：SSE 新邮件 / 同步状态推送
- `settings` / `tasks`

---

## 6. 前端（React + shadcn/ui）

- **三栏布局**：`AccountSidebar`（用户中心 + 账户树 + 虚拟文件夹）/ `MailList`（虚拟滚动）/ `MailDetail`。设计参考 Notion Mail。
- **数据层**：TanStack Query（服务端状态 + 缓存）+ 轻量 client state；URL 驱动当前账户/文件夹/邮件选择。
- `ComposeDialog`：富文本（**tiptap**）。
- `SettingsDialog`：左右分栏（账户管理 / 通用 / 邮件 / 安全 / 通知）。
- **i18n**：zh / en。
- **实时**：SSE 订阅 → 失效化对应 Query 缓存自动刷新。
- **跨形态一致**：使用相对 API base URL，浏览器与 WebView 中行为一致。

---

## 7. 部署 / 发行矩阵

| 形态 | 构建命令 | 产物 | 数据目录 |
|---|---|---|---|
| **Docker/Web** | `CGO_ENABLED=0 go build ./cmd/flymail` | 静态二进制 → scratch/alpine 镜像 | `./data`（挂载卷） |
| **桌面** | `wails build`（按 OS） | Windows `.exe`(WebView2) / macOS `.app`(WKWebView) / Linux(WebKitGTK) | OS 用户目录（`%APPDATA%/FlyMail`、`~/Library/Application Support/FlyMail`） |

- **Wails 版本**：用 **Wails v2**（稳定版）；v3 仍 alpha，不进 V1。
- **CI**：Docker 构建可交叉编译；桌面因 webview 需原生工具链，需 **GitHub Actions 三平台 matrix**（windows/macos/linux runner）各自 `wails build`，**不能交叉编译**。Windows 需目标机有 WebView2 Runtime（Win11 自带）。
- **SSE 一致性**：桌面端用真实 localhost 监听（而非 Wails assetServer），从根上规避 SSE over assetServer 的流式兼容问题。
- **CLI**：`flymail server` / `flymail db init` / `flymail db reset-admin-password`；所有配置在 `data/` 下，可经命令行参数覆盖。

---

## 8. MVP 构建顺序（里程碑）

1. **骨架**：CLI（server / db init / reset-admin）+ viper 配置 + GORM/DB + 管理员登录(JWT) + 前端登录页 + go embed。
2. **账户管理**：CRUD + 连接测试 + 凭证加密。
3. **文件夹 + 首同步（元数据）** → 邮件列表展示。
4. **正文按需 + 阅读 + 已读/标记回写**。
5. **写信 / 回复 / 转发（SMTP）+ 草稿**。
6. **增量同步 + IDLE/SSE 实时新邮件**。
7. **附件按需下载 + 定时任务（收信 / 备份）**。
8. **设置中心 + i18n + Wails 桌面壳打包**。

---

## 9. 待验证项（实现阶段需先确认）

- SQLite FTS5 在 glebarez/modernc 下的实际表现（预期内置可用）；不可用时的搜索降级策略。
- Wails v2 在三平台的构建工具链与 CI matrix 细节。
- IMAP IDLE 在常见服务商（Gmail/Outlook/163 等）的稳定性与轮询兜底参数。
- 桌面端数据目录迁移/首次运行初始化流程。
