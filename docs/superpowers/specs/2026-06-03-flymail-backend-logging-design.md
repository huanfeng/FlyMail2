# FlyMail 后端日志改造 设计文档

- 日期：2026-06-03
- 范围：仅「日志」子系统（三项后端加强中的第一项）。E2E 测试基建、同步队列/IDLE 改造为独立后续 spec，本文不涉及。
- 状态：已与用户确认方向，待 spec 评审。

## 1. 背景与现状

当前 flymail 后端日志：`internal/logging/logging.go` 用 `lumberjack` 做纯文本按大小轮转，把标准库 `log` 与 gin 输出重定向到同一组 writer。

现状痛点：

- **无分级**：全部经 `log.Printf` 输出，无 debug/info/warn/error 区分。
- **无结构化**：手写格式化字符串，无法按字段（账户、阶段）检索。
- **无上下文**：每处日志独立拼字符串，易漏关键字段。
- 全后端约 20 处 `log.Printf` / `log.Println` 散落：`sync/manager.go`(8)、`sync/writeback.go`(5)、`send/service.go`(3)、`notify/service.go`(4)。

`core/logger` 已有成熟的 zap 封装（结构化 JSON / 分级 / lumberjack 轮转 / `WithTag`），但 flymail 未接入。

## 2. 目标

- 引入 **zap 结构化、分级、带上下文字段** 的日志，输出 **JSON**（配合专用日志管理工具查看，不要求纯文本可读）。
- **复用并改造 core/logger**，全项目统一到同一日志体系。
- 为下一阶段「超多邮箱同步」排查打基础：账户级上下文字段（`account_id` 等）必须贯穿同步日志。

## 3. 硬约束

`core/logger` 被多方依赖，改造**必须严格向后兼容、默认行为零变化**。已知调用方：

- `core/database/sqlite.go`
- `mail2im/backend`（worker / app / dispatcher / core / config 等多处）
- `FlyMailLicenseServer`（service / server / middleware / database / migrations 等多处）

判定标准：改造后 `go build ./...` 与 `go test ./...` 在 core、flymail、mail2im、FlyMailLicenseServer 四方全部通过。

## 4. 设计

### 4.1 core/logger 向后兼容改造（纯增量）

问题：`Init()` 硬编码 `./logs/warn_error.log` 并 `os.MkdirAll("./logs")`，仅在 `!Development` 时生成；无视调用方数据目录。

在 `logger.Config` 新增三个可选字段（零值时沿用旧行为）：

| 字段 | 类型 | 零值行为（兼容） | 新行为 |
|------|------|------------------|--------|
| `WarnErrorPath` | `string` | 空 → 沿用 `./logs/warn_error.log` | 指定路径作为 warn/error 分离文件 |
| `DisableSeparateWarnError` | `bool` | false → 沿用旧逻辑（非 dev 才生成） | true → 不生成分离文件 |
| `EncoderFormat` | `string` | 空 → 沿用旧逻辑（`Development?console:json`） | `json`/`console` 显式指定 |

实现要点：

- `Init()` 内把硬编码 `./logs/warn_error.log` 替换为 `WarnErrorPath`（空则回退旧默认值，并保留 `os.MkdirAll(dir)`）。
- 编码器选择由 `EncoderFormat` 优先，未指定再看 `Development`。
- 全局单例 `Logger`/`Sugar` 与全部包级函数（`Info/Warn/Error/With/WithTag/...`）签名与行为**不变**。
- 不新增独立构造函数（用户选择「复用全局单例统一」路线）。flymail 与 core/database 共享同一 `Logger` 实例，输出归一。

### 4.2 flymail 接入（重写 internal/logging）

`internal/logging.Setup` 改为构造 `logger.Config` 并调用 `logger.Init()`：

- 主输出：`<LogDir>/flymail.log`，lumberjack 参数复用 `cfg.Log`（max_size_mb / max_backups / max_age_days / compress）。
- 控制台：`cfg.Log.Console` 为真时追加 stdout（`OutputPaths` 含 `"stdout"`）。
- 分离文件：`WarnErrorPath = <LogDir>/warn_error.log`。
- 格式：默认 `json`（`EncoderFormat="json"`）。
- 级别：新增 `cfg.Log.Level`（默认 `"info"`）。
- 返回的 close 函数内调用 `logger.Sync()` 后再关闭句柄，接到 `app.Shutdown`。

`config.LogConfig` 新增字段：

| 字段 | mapstructure | 默认 | 说明 |
|------|--------------|------|------|
| `Level` | `log.level` | `info` | debug/info/warn/error |
| `Format` | `log.format` | `json` | json/console（dev 本地可设 console） |

`config.Load` 同步 `SetDefault("log.level","info")`、`SetDefault("log.format","json")`（遵守 viper「已注册 key 才被 env 读取」的既有教训）。

### 4.3 标准库 log 与 gin 桥接

- `zap.RedirectStdLog(logger.Logger)`：把残留 `log.Printf` 及 core 中别处的标准库 log 收编进 zap（迁移期安全网）。在 `Init` 成功后于 flymail 接入处调用。
- gin：自写约 30 行 zap 中间件替换 `gin.LoggerWithConfig`：
  - 记录 `method` / `path` / `status` / `latency`(ms) / `client_ip` / `request_id` / `error`（如有）。
  - 4xx 记 `Warn`，5xx 记 `Error`，其余 `Info`。
  - `gin.Recovery` 改为接 zap 的恢复中间件，panic 记 `Error` 带 stacktrace。
  - app.go 中 `gin.DefaultWriter/DefaultErrorWriter` 不再需要指向文件（保留为兜底也无妨）。
  - *已否决的替代*：引入 `gin-contrib/zap`——多一个外部依赖，不如自写可控。

### 4.4 request_id

- 新增中间件：每请求生成短随机 ID（无外部依赖，用 crypto/rand 取 8~12 字节 hex；置于 gin context 与响应头 `X-Request-ID`；若请求头已带 `X-Request-ID` 则透传）。
- 该中间件需在 zap 请求日志中间件之前注册，使请求日志能带上 `request_id`。
- handler 内如需打日志，从 gin context 取 `request_id` 加入字段（提供一个小 helper，如 `logging.FromGin(c) *zap.Logger`）。

### 4.5 上下文字段规范（snake_case）

| 字段 | 类型 | 含义 |
|------|------|------|
| `account_id` | uint | 账户 ID（同步日志必带） |
| `folder` | string | 文件夹路径或类型 |
| `phase` | string | folders / messages / idle / poll / reconnect |
| `uid_next` | uint | 文件夹 UIDNEXT |
| `new` / `unread` / `local` | int | 同步计数 |
| `attempt` / `backoff` | int / duration | 重连次数 / 退避时长 |
| `duration` | duration | 操作耗时 |
| `request_id` | string | HTTP 请求关联 ID |
| 错误 | — | 统一 `zap.Error(err)`，键为 `error` |

约定：worker 入口用 `logger.With(zap.Uint("account_id", id))` 建子 logger，该账户后续日志自动带 `account_id`。

### 4.6 迁移现有日志 + 补关键点 + 脱敏

- 逐一改写 20 处 `log.Printf`：`sync/manager.go`、`sync/writeback.go`、`send/service.go`、`notify/service.go`。
- 分级原则：正常流程 `Info`；可恢复异常（重连、单文件夹失败）`Warn`；导致功能失效 `Error`。
- 补充缺失日志点：worker 启动/停止、reconcile 增减账户、IDLE 进入/退出、同步耗时 `duration`。
- **脱敏铁律**：账户密码、JWT、token 绝不进日志；送审时逐条核查同步/发信/认证路径。

## 5. 测试与验证

- **core/logger 单测**（新增）：
  - 默认行为不变：`WarnErrorPath` 空 + `Development=false` → 仍写 `./logs/warn_error.log`。
  - 新字段生效：指定 `WarnErrorPath` → 写到指定路径；`DisableSeparateWarnError=true` → 不生成；`EncoderFormat="json"` 在 dev 下仍输出 JSON。
- **flymail logging 接入测试**：临时目录跑 `Init`，断言 `flymail.log` 生成、每行可被 `json.Unmarshal`、含预期字段、level 过滤生效（debug 在 info 级下不出现）。
- **回归**：`go build ./...` + `go test ./...` 覆盖 core / flymail / mail2im / FlyMailLicenseServer 四方。
- **真机**：dev.ps1 重启后端，确认 `flymail/logs/flymail.log` 为 JSON、同步日志含 `account_id`、`warn_error.log` 落在 `flymail/logs`、请求日志含 `request_id`。

## 6. 范围边界（YAGNI）

- 本轮**只做日志**。不引入 worker 池、不改同步调度、不改状态持久化（属第三项「同步队列」）。
- 不引入外部 gin 日志库、不引入 OpenTelemetry/分布式追踪（request_id 仅本地关联，够用即止）。
- 不改其他模块（mail2im / LicenseServer）的业务日志，仅保证它们继续编译通过、行为不变。

## 7. 交付物清单

1. `core/logger/logger.go` 增量改造 + `core/logger/logger_test.go` 单测。
2. `flymail/backend/internal/logging/logging.go` 重写为 zap 接入。
3. `flymail/backend/internal/config/config.go` 新增 `log.level` / `log.format`。
4. flymail 新增 request_id 中间件 + zap gin 中间件 + recovery 中间件。
5. `flymail/backend/internal/app/app.go` 接入（含 `RedirectStdLog`、Shutdown 调 `Sync`）。
6. `flymail/backend/internal/server/router.go` 替换 gin 日志/恢复中间件、挂 request_id。
7. 20 处 `log.Printf` 迁移为结构化日志 + 补关键点。
8. flymail logging 接入测试。
