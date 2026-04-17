# Mail2IM 架构设计与实施计划

## 1. 系统架构概述

Mail2IM 将采用模块化架构，主要包含以下核心组件：

1.  **Email Engine (邮件引擎)**: 负责后台运行，管理多个 IMAP 连接，支持 OAuth 和普通密码。
2.  **Event Bus (事件总线)**: 系统的核心神经中枢，统一处理邮件到达、系统错误、认证过期等所有事件。
3.  **Notification Dispatcher (通知分发器)**: 订阅事件总线，根据优先级和策略分发到不同的 IM 渠道。
4.  **Web Server (API & Viewer)**: 提供管理界面 API 和单封邮件查看页面。
5.  **License Manager (授权管理器)**: 负责设备认证和权益管理。
6.  **Janitor (清理工)**: 负责定期清理过期数据。
7.  **Debug Monitor (调试监控)**: 暴露系统内部状态，提供开发者仪表盘。

## 2. 详细需求分解与设计

### 2.1 多账户邮件引擎 (Email Engine)
**需求**: 支持 100+ 账户，IDLE/轮询，精细化配置，**OAuth 支持**，**代理复用**。

**设计**:
*   **并发模型**: 每个账户启动一个独立的 Goroutine (Worker)。
*   **连接管理**:
    *   **Proxy Entity**: 独立的代理配置管理 (SOCKS5/HTTP)，账户仅引用 `ProxyID`。
    *   **Health Check**: 实时监控连接状态。
*   **认证模块 (Auth Provider)**:
    *   `Basic`: 账号/密码 (支持专用密码过期时间设置)。
    *   `OAuth2`: 支持 Gmail 等标准 OAuth 流程 (支持自定义 ClientID/Secret)。
    *   **过期提醒**: 对于设置了 `PasswordExpiresAt` 的账户，在过期前 7/3/1 天生成 `SystemEvent`。
*   **批量创建**: 提供 UI 支持 "模板化创建" (例如：输入 10 个账号密码，应用同一套服务器/代理配置)。

### 2.2 事件总线与分级 (Event Bus)
**需求**: 统一处理邮件和系统错误，区分优先级。

**设计**:
*   **Event Types**:
    *   `EmailReceived` (Priority: Normal)
    *   `AuthFailed` (Priority: **Critical**) -> 立即通知
    *   `ConnectionLost` (Priority: Low) -> 抖动去重 (Debounce)。
    *   `PasswordExpiring` (Priority: High) -> 每日检查触发。
*   **Dispatch Logic**: 通道配置可以设置 "Minimum Priority"。

### 2.3 通知分发系统 (Notification Dispatcher)
**需求**: 多渠道，策略配置，**附件处理**。

**设计**:
*   **Channel Interface**: `Send(event Event) error`。
*   **策略引擎**:
    *   **静默时间 (Quiet Hours)**: 仅拦截 Normal 级别的邮件通知，Critical 级别的系统报警透传。
    *   **过滤规则**: 黑白名单 (Regex)。
*   **附件策略**:
    *   小文件 (<10MB): 直接通过 IM API 发送。
    *   大文件: 存储在本地 `storage/attachments`，生成带 Token 的临时下载链接。

### 2.4 开发者调试面板 (Dev Dashboard)
**需求**: 全方位日志，Worker 状态，快速排查问题。

**设计**:
*   **Backend**: `DebugService` 维护一个内存中的状态快照 (State Snapshot)。
    *   `WorkerStatus`: Map[AccountID]Status (Connecting, IDLE, Polling, Error)。
    *   `LastLog`: 每个 Worker 保留最近 10 条日志在内存中。
*   **Frontend**: 独立的 `/dev` 路由。
    *   **Worker Grid**: 100 个小方块代表 100 个账户，颜色代表状态 (绿=正常, 红=错误, 黄=重连中)。
    *   **Log Stream**: 点击某个方块，查看该 Worker 的实时日志流 (WebSocket)。
    *   **System Stats**: Goroutine 数量，内存占用，事件总线积压数。

### 2.5 数据生命周期 (Data Retention)
**需求**: 避免数据膨胀，定期清理。

**设计**:
*   **Config**: 全局配置 `RetentionDays` (默认 30 天)。
*   **Janitor Job**: 每日凌晨运行，清理日志和附件。

## 3. 数据模型设计 (Updated)

```go
type Proxy struct {
    ID       uint
    Name     string // "US Node 1"
    Type     string // "socks5", "http"
    Host     string
    Port     int
    Username string
    Password string
}

type Account struct {
    ID                uint
    Email             string
    AuthType          string // "password", "oauth2"
    Password          string // Encrypted
    OAuthToken        string // Encrypted JSON
    PasswordExpiresAt *time.Time

    ProxyID           *uint  // Foreign Key

    Provider          string
    IMAPServer        string
    // ... other config

    Status            string // Active, AuthFailed, NetworkError
    LastSyncAt        time.Time
}
```

### Update implementation plan for Email List & Detail View

### Backend
#### [MODIFY] [models.go](file:///home/dufeng/develop/workspace/go_dev/mail2im/backend/internal/models/models.go)
- Add `Email` struct to store email content (From, To, Subject, TextBody, HTMLBody).

#### [MODIFY] [worker.go](file:///home/dufeng/develop/workspace/go_dev/mail2im/backend/internal/core/worker.go)
- Integrate `go-message` to parse email content.
- Save fetched emails to the database.

#### [NEW] [email.go](file:///home/dufeng/develop/workspace/go_dev/mail2im/backend/internal/api/email.go)
- Implement `GetEmails` (list with pagination).
- Implement `GetEmail` (detail).
- Implement `GetEmailHTML` (raw HTML content).

#### [MODIFY] [main.go](file:///home/dufeng/develop/workspace/go_dev/mail2im/backend/cmd/server/main.go)
- Register email routes.

### Frontend
#### [NEW] [Emails.vue](file:///home/dufeng/develop/workspace/go_dev/mail2im/frontend/src/views/Emails.vue)
- Display list of emails.
- Support pagination and refresh.

#### [NEW] [EmailDetail.vue](file:///home/dufeng/develop/workspace/go_dev/mail2im/frontend/src/views/EmailDetail.vue)
- Display email details.
- Use iframe to render HTML content safely.

#### [MODIFY] [router/index.ts](file:///home/dufeng/develop/workspace/go_dev/mail2im/frontend/src/router/index.ts)
- Add routes for `/emails` and `/emails/:id`.

#### [MODIFY] [i18n.ts](file:///home/dufeng/develop/workspace/go_dev/mail2im/frontend/src/i18n.ts)
- Add translations for email view.

### Channel Management Refactor
#### [MODIFY] [models.go](file:///home/dufeng/develop/workspace/go_dev/mail2im/backend/internal/models/models.go)
- Add `Channel` struct:
    - `ID`, `Name`, `Type` (telegram, discord, etc.)
    - `Config` (JSON string for token, chat_id, etc.)
    - `Status` (Enabled/Disabled)

#### [NEW] [channel.go](file:///home/dufeng/develop/workspace/go_dev/mail2im/backend/internal/api/channel.go)
- Implement CRUD endpoints for Channels.
- Implement `TestChannel` endpoint.

#### [MODIFY] [dispatcher.go](file:///home/dufeng/develop/workspace/go_dev/mail2im/backend/internal/dispatcher/dispatcher.go)
- Remove hardcoded channels.
- Load channels from DB on startup.
- Provide method to reload channels.

#### [NEW] [Channels.vue](file:///home/dufeng/develop/workspace/go_dev/mail2im/frontend/src/views/Channels.vue)
- List existing channels.
- Wizard dialog to add new channel:
    - Step 1: Select Type (Telegram, etc.)
    - Step 2: Configure (Token, ChatID)
    - Step 3: Test & Save

## 4. 实施阶段规划

### Phase 1: 核心引擎与调试基座 (Core & Debug)
1.  实现 `Proxy` 和 `Account` 模型。
2.  实现 `EventBus`。
3.  **实现 `DebugService` 和基础的 `/dev` 面板** (为了方便后续开发调试，优先实现)。
4.  重构 Watcher，接入 EventBus 和 DebugService。
5.  实现批量创建 API。

### Phase 2: 通知与策略 (Dispatcher)
1.  实现 `NotificationDispatcher` 订阅 EventBus。
2.  实现基于优先级的通知分发。
3.  实现附件下载服务和 Janitor。

### Phase 3: Web 与高级功能
1.  Web 查看器。
2.  OAuth2 授权流程。

### Phase 4: 商业化与完善
1.  License 检查。
2.  密码过期检测。

## 5. 用户审查重点
- [x] **代理支持**: 已改为独立实体 `Proxy`，支持复用。
- [x] **批量导入**: 已改为 "模板化批量创建" 功能。
- [x] **调试面板**: 已加入 Phase 1，提供 Worker 状态网格和实时日志。
