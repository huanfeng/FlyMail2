# FlyMail 后端 E2E 测试基建 设计文档

- 日期：2026-06-04
- 范围：E2E 测试基建（三项后端加强中的第二项）。基于 GreenMail（Docker）真协议、HTTP API 全栈驱动。日志改造（已完成）与同步队列改造（第三项）为独立 spec，本文不涉及。
- 状态：已与用户逐项确认方向，待 spec 评审。

## 1. 背景与目标

flymail 后端当前有 40+ 单元测试（标准库 testing），但**没有端到端测试**：无法验证「HTTP API → service → sync manager → IMAP 服务器 → DB → HTTP 查询」整条链路在真实 IMAP/SMTP 协议下的正确性。同步、回写、发送、删除/移动、IDLE/SSE 这些核心功能改动频繁，缺少自动化回归保护。

目标：建立基于 **GreenMail** 的真协议 E2E 测试，经 **HTTP API 全栈**驱动，覆盖四条核心链路（收信 / 回写 / 发送·删除·移动 / IDLE·SSE），可在 Linux + Docker 环境自动化运行，并为将来接 CI 打基础。

## 2. 方案选型

### 2.1 为什么不是 Mailpit（重要纠正）

最初设想复用 mail2im 的 Mailpit 范式，但经权威核实：**Mailpit 从未实现 IMAP server**（仅 SMTP + Web UI + 可选 POP3，社区 issue #249/#72 长期未落地）。mail2im 的 `docker-compose.test.yml` 虽映射了 `1143:1143`，但 Mailpit 不监听该端口；其 `idle_test.go` 实际只用 SMTP + HTTP API 验证，从不连 IMAP——因为 mail2im 是「收信→分发」系统，不依赖 IMAP 写。flymail 是完整邮件客户端，核心是 IMAP 收信/回写，Mailpit 不可用。

### 2.2 为什么 GreenMail

GreenMail（`greenmail/standalone` Docker 镜像）是成熟的测试用邮件服务器，提供**完整 IMAP 读写**（FETCH/STORE/APPEND/COPY/EXPUNGE/IDLE/SEARCH）+ SMTP + REST API。默认端口：SMTP 3025 / IMAP 3143 / IMAPS 3993 / REST API 8080。默认 `-Dgreenmail.setup.test.all -Dgreenmail.auth.disabled`，任意用户名/密码登录即自动建邮箱，账户配置零摩擦。

### 2.3 已否决的替代

- **进程内内存假 IMAP（Session 层）**：无 Docker、CI 友好，但不过 `core/imap` 真客户端协议。用户选择真协议，故否决。
- **自写最小 TCP IMAP server**：实现复杂、易踩协议 bug，否决。

## 3. 部署与运行模式

**模式 A：代码与 GreenMail 同在 Linux 服务器**（用户选定）。
开发在 Windows（仓库工作目录），但 Docker 部署在远程 Linux。E2E 在 Linux 上运行：代码同步到服务器（git push/pull 或 rsync）→ `docker compose -f docker-compose.e2e.yml up -d` → `E2E_GREENMAIL=1 go test ./internal/e2e/...`。GreenMail 与测试同机 localhost，无网络/防火墙问题，IDLE/SSE 时序最稳，且天然等同将来的 CI（Linux + Docker）环境。

**env 解耦（基础设计，与模式无关）**：GreenMail 连接信息全部环境变量可配，默认 localhost，使测试代码与部署位置无关：
- `GREENMAIL_HOST`（默认 `localhost`）
- `GREENMAIL_IMAP_PORT`（默认 `3143`）
- `GREENMAIL_SMTP_PORT`（默认 `3025`）
- `GREENMAIL_API_PORT`（默认 `8080`）
- `E2E_GREENMAIL`（门控；未设则全部 E2E 测试 `t.Skip`）

模式 B（Windows 跑测试 + 远程 GreenMail）经 env 解耦后自动可用，仅需设 `GREENMAIL_HOST=<服务器IP>`，本 spec 不为其单独写脚本（文档附带说明即可）。

## 4. 基础设施

### 4.1 docker-compose.e2e.yml（仓库根或 flymail/）
```yaml
services:
  greenmail:
    image: greenmail/standalone:2.1.8
    ports:
      - "3025:3025"   # SMTP
      - "3143:3143"   # IMAP
      - "8080:8080"   # REST API
    environment:
      GREENMAIL_OPTS: "-Dgreenmail.setup.test.all -Dgreenmail.hostname=0.0.0.0 -Dgreenmail.auth.disabled -Dgreenmail.verbose"
```

### 4.2 e2e.sh（bash，Linux 上运行）
职责：`docker compose up -d greenmail` → 轮询 GreenMail REST API（`http://localhost:8080/api/service/readiness` 或等价健康端点）直到就绪 → `E2E_GREENMAIL=1 go test ./internal/e2e/... -count=1 -timeout 120s -v` → 结束后 `docker compose down`（可加 `--keep-up` 选项保留容器便于反复跑）。

### 4.3 env 门控
所有 E2E 测试首行：`if os.Getenv("E2E_GREENMAIL") == "" { t.Skip("set E2E_GREENMAIL=1 to run (requires GreenMail)") }`。保证 Windows 本地 `go test ./...` 与无 Docker 的 CI 不受影响（仅编译 + skip）。

## 5. 测试脚手架（internal/e2e）

### 5.1 app harness（harness.go）
- 构造测试 `config.Config`：临时 `DataDir`（`t.TempDir()`）、固定 `Auth.JWTSecret` 与 `Crypto.EncryptionKey`、`Log` 指向临时目录、`Server` 不监听（用 httptest）。
- `app.New(cfg)` → `httptest.NewServer(app.Handler())` 暴露真实路由；`app.StartBackground()` 启动 sync manager（IDLE/SSE 链路需要）。
- 返回 `{ baseURL, httpClient, db, app, cleanup }`；`cleanup` 调 `app.Shutdown()` + 关 httptest server。
- **注意**：日志为全局 zap 单例，多个 harness 顺序运行；E2E 测试不并行（`-p 1` 或不加 `t.Parallel`），避免单例与 GreenMail 状态串扰。

### 5.2 GreenMail 辅助（greenmail.go）
- `greenmailAddr()`：从 env 读 host/port，返回 IMAP/SMTP/API 地址。
- `waitReady(timeout)`：轮询 REST API 就绪。
- `sendSeed(from, to, subject, body)`：`net/smtp` 发到 GreenMail SMTP（投递到 `to` 的 INBOX）。
- `uniqueMailbox(t)`：基于 `t.Name()` + 递增计数生成唯一收件地址（如 `e2e-<n>@localhost`），隔离测试。
- 可选 `purge()`：经 REST API 清空（隔离备用手段）。
- `imapClient(mailbox)`：直接用 `core/imap.Dial` 连 GreenMail 做断言（验证 `\Seen`/`\Flagged`/文件夹内容）。

### 5.3 HTTP 辅助（client.go）
- `login(adminUser, adminPass)`：调 `/api/v1/auth/login` 拿 JWT；返回带 Bearer 的 client。
- `createAccount(...)`：调账户创建 API，IMAP/SMTP 指向 GreenMail，security=none，email=唯一邮箱。
- `triggerSyncAndWait(accountID, timeout)`：触发同步并轮询 `/accounts/:id/sync/status` 直到 `done`/`error`。
- JSON 解码 helper。

### 5.4 等待原语
同步/IDLE 是异步——统一用「轮询 + timeout」：`eventually(t, timeout, interval, cond)`。SSE 测试用真实 SSE 客户端读事件流，带超时。

## 6. 四条链路测试设计

### 6.1 sync_test.go — 收信链路
建账户（指向 GreenMail）→ 连接测试 API 返回成功 → `sendSeed` 投 3 封到该邮箱 → 触发同步并等待 `done` → `GET /folders` 含 INBOX → `GET /folders/:id/messages` 返回 3 封、主题/发件人正确 → 未读数=3 → `GET /messages/:id` 详情正文正确。

### 6.2 writeback_test.go — 回写链路
在收信基础上：`POST /messages/:id/read`（已读）→ 等回写 → 用 `imapClient` 直连 GreenMail 验证该 UID 带 `\Seen`。同理 `POST /messages/:id/flag`（星标）→ 验证 `\Flagged`。

> 约束：回写是异步队列（writeback），断言前需 `eventually` 等待 IMAP 端生效。

### 6.3 verbs_test.go — 发送·删除·移动
- **发送**：`POST /send`（to 指向另一 GreenMail 邮箱）→ `imapClient` 登录收件邮箱验证收到；若账户有 Sent 文件夹，验证 APPEND 副本。
- **删除**：对一封邮件 `POST /messages/:id/delete` → 验证移到 Trash 或（无 Trash 时）EXPUNGE。
- **移动**：`POST /messages/:id/move` 到目标文件夹 → IMAP 端验证。

> **关键约束（实现时验证）**：GreenMail 默认可能只有 INBOX。删除/移动到 Trash、发送 APPEND 到 Sent 需要这些文件夹存在。harness 应在建账户后经 IMAP `CREATE` 预建 `Sent`/`Trash`（GreenMail 支持 CREATE），否则 flymail 删除逻辑会走「无回收站→EXPUNGE 永久删除」分支、发送 APPEND 会失败（flymail 仅 warn）。实现时先用 `imapClient` 探明 GreenMail 默认文件夹集合，再决定预建哪些 + 断言哪个分支。

### 6.4 realtime_test.go — IDLE/SSE 链路
harness `StartBackground()` 后 manager 对账户进 IDLE → 用 SSE 客户端订阅 `/api/v1/events?access_token=` → `sendSeed` 投新邮件 → `eventually` 断言收到 `new_mail` 事件（合理 timeout，如 30s）。

> **风险（最高）**：依赖 GreenMail IDLE 在 SMTP 投递后主动推送 EXISTS 给 IDLE 中的客户端，以及 flymail manager 的 IDLE→pollInbox→SSE 时序。此链路最易 flaky，timeout 需宽松；若 GreenMail IDLE 推送不可靠，降级为「轮询间隔触发同步后 SSE」验证，并在 spec/plan 记录降级。

## 7. 关键约束与风险

- **Windows/Linux 工作流**：开发在 Windows，E2E 在 Linux 跑。implementer 在 Windows 上只能：写代码 + `go build ./internal/e2e/` + `go vet` + `go test`（无 GreenMail 时 skip，验证 skip 逻辑与编译）。**真正的端到端运行验证必须由用户在 Linux 上执行**（`docker compose up` + `E2E_GREENMAIL=1 go test`）。这是交付的固有边界。
- **测试隔离**：每测试唯一收件邮箱；不并行（顺序跑），避免 GreenMail 状态与 zap 单例串扰。
- **异步时序**：同步触发、回写队列、IDLE 推送均异步，一律 `eventually` + timeout，禁用固定 `sleep` 作为同步手段（可作小退避）。
- **配置注入**：account email = 唯一邮箱，IMAP/SMTP host/port 来自 env，security=none，密码随意（GreenMail auth.disabled）。
- **GreenMail 行为不确定项（实现时以真实探测为准，勿臆测）**：默认文件夹集合、IDLE 推送时机、MOVE 扩展是否支持（flymail 有 COPY+EXPUNGE 回退）。计划阶段第一个任务应是「连通性探针测试」先摸清这些。
- **CI**：本设计面向 Linux + Docker；env gate 使其在无 Docker 环境自动 skip。接 CI 留作后续。

## 8. 范围边界（YAGNI）

- 本轮**只做 E2E 基建 + 四条链路首批测试**。不改业务代码（除非 E2E 暴露真实 bug，另行处理）。
- 不接入 CI 流水线（仅保证 env gate 让 CI 可选跑）。
- 不做性能/压力 E2E、不做多账户并发 E2E（属第三项「同步队列」范畴）。
- 不为模式 B 写专用脚本（env 解耦后文档说明即可）。

## 9. 交付物清单

1. `docker-compose.e2e.yml`（GreenMail 服务定义）。
2. `e2e.sh`（bash 编排：up → 健康检查 → go test → down）。
3. `flymail/backend/internal/e2e/harness.go`（app harness + cleanup）。
4. `flymail/backend/internal/e2e/greenmail.go`（GreenMail SMTP/IMAP/API 辅助 + 唯一邮箱）。
5. `flymail/backend/internal/e2e/client.go`（HTTP 登录/建账户/触发同步等待）。
6. `flymail/backend/internal/e2e/probe_test.go`（连通性 + GreenMail 默认能力探针，第一个落地，摸清文件夹/IDLE/MOVE）。
7. `flymail/backend/internal/e2e/sync_test.go`（收信链路）。
8. `flymail/backend/internal/e2e/writeback_test.go`（回写链路）。
9. `flymail/backend/internal/e2e/verbs_test.go`（发送/删除/移动）。
10. `flymail/backend/internal/e2e/realtime_test.go`（IDLE/SSE）。
11. 文档：README 或 docs 段落说明模式 A 跑法 + 模式 B 注意事项。
