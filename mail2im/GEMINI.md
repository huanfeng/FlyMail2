# Mail2IM 开发助手指南 (GEMINI.md)

本文档旨在为 AI 助手提供 `Mail2IM` 项目的上下文、架构规范及开发准则。

**⚠️ 核心指令**: 请始终使用**简体中文**与用户进行交互。

## 1. 项目概述
`Mail2IM` 是一个将邮件转换为即时通讯（IM）通知的系统。它通过后台 Worker 监听 IMAP 邮箱，将邮件事件通过事件总线（Event Bus）分发到不同的 IM 渠道（如 Telegram, Discord, 企业微信等）。

### 技术栈
*   **Backend**: Go 1.25.4
    *   **Web Framework**: Gin (`github.com/gin-gonic/gin`)
    *   **Database**: SQLite (`gorm.io/driver/sqlite`, `gorm.io/gorm`)
    *   **Mail Engine**: `github.com/emersion/go-imap/v2` (IMAP), `github.com/emersion/go-message` (Parsing)
    *   **Auth/Crypto**: `golang.org/x/crypto`, `github.com/golang-jwt/jwt/v5`
*   **Frontend**: Vue 3.5 + Vite 7.2
    *   **Language**: TypeScript 5.9
    *   **UI Library**: PrimeVue 4.4 + TailwindCSS 4.1 + PrimeIcons
    *   **State Management**: Pinia
    *   **Routing**: Vue Router
    *   **HTTP**: Axios
    *   **Utils**: date-fns, vue-i18n

## 2. 项目结构与模块职责

### 目录结构
*   `backend/`: Go 后端代码
    *   `cmd/server/`: 程序入口 (`main.go`)
    *   `config/`: 默认配置文件
    *   `internal/api/`: HTTP 接口 Handlers (Gin)
    *   `internal/core/`: 核心业务逻辑 (DB, Auth, EventBus, ImapWorker, Janitor)
    *   `internal/dispatcher/`: 消息分发与渠道适配器 (Telegram, Discord 等)
    *   `internal/models/`: GORM 数据模型
    *   `mail2im_data/`: 运行时数据目录 (SQLite DB, Configs)
    *   `storage/attachments/`: 邮件附件存储目录
*   `frontend/`: Vue 前端代码
    *   `src/views/`: 页面视图 (Dashboard, Emails, Accounts, Settings 等)
    *   `src/components/`: 通用组件
    *   `src/services/`: API 封装 (`api.ts`)
    *   `src/stores/`: Pinia 状态管理
*   `docs/`: 项目文档 (DESIGN.md, AGENTS.md, PROGRESS.md)

### 核心模块
1.  **Email Engine (`backend/internal/core/imap.go`, `worker.go`)**: 负责维护 IMAP 连接，支持多账户、代理（SOCKS5/HTTP）及 OAuth2 认证。
2.  **Event Bus (`backend/internal/core/eventbus.go`)**: 系统的神经中枢，处理 `EmailReceived`, `AuthFailed` 等事件。
3.  **Dispatcher (`backend/internal/dispatcher/`)**: 订阅事件总线，根据策略将通知推送到配置的 IM 渠道。
4.  **Web Server (`backend/internal/api/`)**: 提供 RESTful API 供前端管理配置及查看邮件。

## 3. 开发与运行指南

### 启动开发环境
在项目根目录运行：
```bash
./start_dev.sh
```
该脚本会同时启动后端（默认端口 8080）和前端（默认端口 8008）。
*   **Backend URL**: `http://localhost:8080`
*   **Frontend URL**: `http://localhost:8008`
*   **环境变量**: 可在运行脚本前设置 `PORT`, `FRONTEND_PORT` 等（参考脚本内容）。

### 后端开发 (`backend/`)
*   **运行**: `go run cmd/server/main.go`
*   **测试**: `go test ./...`
*   **规范**:
    *   使用 `gofmt` 格式化。
    *   错误处理需显式，避免直接 Panic。
    *   数据库变更需同步更新 `internal/models` 并确保 `AutoMigrate` 正常工作。

### 前端开发 (`frontend/`)
*   **安装依赖**: `pnpm install`
*   **运行**: `pnpm dev`
*   **构建**: `pnpm build` (包含 `vue-tsc` 类型检查)
*   **规范**:
    *   组件使用 `<script setup lang="ts">`。
    *   样式优先使用 TailwindCSS，必要时使用 CSS Modules 或 `style.css`。
    *   API 调用统一封装在 `src/services/api.ts`。
    *   LocalStorage Key 统一使用 `mail2im_` 前缀。

## 4. 当前进度与任务 (截至 2025-12-01)

### 已完成 (Phase 1 & 2 & 部分 3)
*   ✅ **核心架构**: 数据库模型，EventBus，DebugService，多账户 Worker 引擎。
*   ✅ **基础功能**: 代理管理，批量账户创建，附件下载，Janitor 清理。
*   ✅ **通知渠道**: 支持 Email, Telegram, Discord, 企业微信, 飞书, 钉钉。
*   ✅ **前端界面**:
    *   Dashboard (仪表盘)
    *   Accounts (账户管理 - 列表/添加/批量)
    *   Proxies (代理管理)
    *   Settings (全局设置)
    *   Debug (调试面板 - 实时日志/状态)
    *   Emails (邮件列表 - 列表/详情/HTML预览)
    *   Channels (渠道管理 - 列表/向导/测试)

### 待办事项 (TODO)
*   🔲 **高级 Web 功能**:
    *   AI 翻译 API 集成
    *   OAuth2 授权流程完善
*   🔲 **商业化/安全**:
    *   设备指纹与 License 检查
    *   密码过期检测
*   🔲 **运维**: API 文档完善，系统监控。

## 5. 交互准则
1.  **语言**: 必须使用**简体中文**。
2.  **代码修改**:
    *   修改代码前，先阅读相关文件理解上下文。
    *   遵循现有的代码风格（Go 命名规范，Vue 组件结构）。
    *   涉及到新功能时，优先考虑复用现有模块（如 `api.ts`, `models`）。
3.  **提交**:
    *   使用清晰的 Git Commit Message (e.g., `feat: add email detail view`, `fix: resolve imap timeout`).
    *   不要主动 revert 除非用户要求或发生错误。
4.  **安全**: 避免在代码中硬编码密钥；敏感配置应通过环境变量或 `providers.json` 加载。
