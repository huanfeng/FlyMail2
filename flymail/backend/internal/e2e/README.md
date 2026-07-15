# FlyMail 后端 E2E 测试

基于 GreenMail 真协议（IMAP/SMTP）的端到端测试：进程内起真实 app（临时 SQLite +
`httptest.Server`），经 HTTP API 全栈驱动，并用 `core/imap` 直连 GreenMail 做服务器端断言。
覆盖四条链路：收信（sync）/ 回写（writeback）/ 发送·删除·移动（verbs）/ IDLE·SSE（realtime）。

所有测试由 `E2E_GREENMAIL=1` 门控——未设置时全部 `t.Skip`，普通 `go test ./...` 不受影响。

## 运行方式

### 模式 A：Windows 本机（推荐，无需 Docker）

前置：Java 11+（GreenMail standalone JAR）、Go。仓库根执行：

```powershell
./e2e.ps1                 # 全量
./e2e.ps1 -Run TestProbe  # 只跑匹配项
./e2e.ps1 -KeepUp         # 跑完保留 GreenMail 进程，迭代时省启动时间
```

脚本自动：下载缓存 JAR 到 `.dev/greenmail/` → 起 GreenMail（SMTP :3025 / IMAP :3143 /
API :18080，8080 常被 IDE 占用故换端口）→ readiness 健康检查 → `go test -p 1` → 停服务。
已有就绪实例时直接复用。

### 模式 B：Linux / CI（Docker）

```bash
./e2e.sh              # docker compose 起 GreenMail(API :8080) → go test → down
KEEP_UP=1 ./e2e.sh    # 保留容器
```

### 模式 C：远程 GreenMail

GreenMail 常驻任意可达主机时，本机只需：

```powershell
$env:GREENMAIL_HOST='<ip>'; $env:E2E_GREENMAIL='1'
cd flymail/backend; go test ./internal/e2e/... -p 1 -count=1 -v
```

环境变量：`GREENMAIL_HOST`（默认 localhost）、`GREENMAIL_IMAP_PORT`（3143）、
`GREENMAIL_SMTP_PORT`（3025）、`GREENMAIL_API_PORT`（8080）。

## 约定与事实

- **顺序运行**：`-p 1`、不加 `t.Parallel()`（GreenMail 有状态 + zap 全局单例）。
- **管理员**：server 不自动建管理员，harness 在迁移后同库 seed `admin/admin`。
- **隔离**：每测试唯一收件邮箱（时间戳+序号），GreenMail `auth.disabled` 下任意登录自动建邮箱。
- **异步断言**：同步/回写/IDLE 均异步，一律 `eventually` 轮询，禁止裸 sleep 断言。
- **GreenMail 能力**（probe 实测）：默认仅 INBOX（无 Sent/Trash）、SupportsIDLE=true。
  因此删除断言走 EXPUNGE 分支；移动测试先经 `Session.Client`（go-imap/v2）CREATE 目标文件夹；
  发送不断言 Sent APPEND 副本（flymail 对此仅 warn）。
