# FlyMail 后端 E2E 测试基建 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development 或 superpowers:executing-plans 逐任务实现。步骤用 `- [ ]` 复选框跟踪。

**Goal:** 建立基于 GreenMail（Docker）的真协议 E2E 测试，经 HTTP API 全栈驱动，覆盖收信/回写/发送·删除·移动/IDLE·SSE 四条链路。

**Architecture:** docker-compose 起 GreenMail → `internal/e2e` 内进程内起真实 app（临时 SQLite + `httptest.Server(app.Handler())` + `StartBackground()`）→ 账户经 env 可配指向 GreenMail → HTTP client 调真实 API，并用 `core/imap` 直连 GreenMail 做服务器端断言。

**Tech Stack:** Go 标准库 testing、net/smtp、net/http/httptest、core/imap、GreenMail、Docker。

**对应 spec:** `docs/superpowers/specs/2026-06-04-flymail-e2e-testing-design.md`
**分支:** `feat/flymail-e2e-testing`（基于 main，spec 已提交；不含日志分支改动，互不依赖）。

---

## ⚠️ 关键执行约束（务必先读）

1. **本地（Windows）执行方式 —— GreenMail standalone JAR（2026-07-15 更新，经用户确认）**：开发机无 Docker，但有 Java 21。GreenMail 官方 Docker 镜像本质就是 `greenmail-standalone` JAR，因此本地直接用 `java -jar` 起 GreenMail（`e2e.ps1` 编排：下载缓存 JAR → 起服务 → 健康检查 → `go test` → 停服务），**E2E 可在 Windows 本地全量运行验证**。原「仅 Linux 可跑」约束作废；`docker-compose.e2e.yml` + `e2e.sh` 保留，供未来 Linux/CI 使用。JAR 缓存于 `.dev/greenmail/`（gitignore 内）。
2. **不臆测、先核对**：所有请求/响应 DTO 字段名、默认管理员凭证、SSE 事件格式，**必须读对应 handler/service 源码确认后再写**，不得照搬本计划的字段名假设（本计划标注了「核对来源」，但以源码为准）。这是 Mailpit 教训的延续。
3. **不并行**：E2E 测试顺序运行（GreenMail 有状态 + zap 全局单例），测试内不加 `t.Parallel()`；`e2e.sh` 用 `go test -p 1`。

---

## 已确认的端点清单（全部 `/api/v1` 前缀，protected 组需 `Authorization: Bearer <token>`）

| 用途 | 方法 | 路径 | 鉴权 |
|------|------|------|------|
| 登录 | POST | `/api/v1/auth/login` | 否 |
| 刷新 | POST | `/api/v1/auth/refresh` | 否 |
| 当前用户 | GET | `/api/v1/auth/me` | 是 |
| 建账户 | POST | `/api/v1/accounts` | 是 |
| 列账户 | GET | `/api/v1/accounts` | 是 |
| 连接测试 | POST | `/api/v1/accounts/test` | 是 |
| 启停账户 | POST | `/api/v1/accounts/:id/enabled` | 是 |
| 列文件夹 | GET | `/api/v1/accounts/:id/folders` | 是 |
| 列文件夹邮件 | GET | `/api/v1/folders/:fid/messages` | 是 |
| 邮件详情 | GET | `/api/v1/messages/:id` | 是 |
| 标记已读 | POST | `/api/v1/messages/:id/read` | 是 |
| 标记星标 | POST | `/api/v1/messages/:id/flag` | 是 |
| 删除 | POST | `/api/v1/messages/:id/delete` | 是 |
| 移动 | POST | `/api/v1/messages/:id/move` | 是 |
| 触发同步 | POST | `/api/v1/accounts/:id/sync` | 是 |
| 同步状态 | GET | `/api/v1/accounts/:id/sync/status` | 是 |
| 发送 | POST | `/api/v1/send` | 是 |
| SSE 事件流 | GET | `/api/v1/events?access_token=<token>` | query token |
| 健康 | GET | `/api/v1/healthz` | 否 |

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `docker-compose.e2e.yml`（仓库根）| GreenMail 服务定义 |
| `e2e.sh`（仓库根）| bash 编排：up → 健康检查 → `go test -p 1` → down |
| `flymail/backend/internal/e2e/env.go` | GreenMail 连接信息（env 可配，默认 localhost）+ 纯函数 |
| `flymail/backend/internal/e2e/env_test.go` | env 解析纯函数单测（可在 Windows 跑） |
| `flymail/backend/internal/e2e/harness.go` | 进程内起 app + httptest + cleanup |
| `flymail/backend/internal/e2e/greenmail.go` | SMTP 种子 / IMAP 断言客户端 / 唯一邮箱 / REST 健康 |
| `flymail/backend/internal/e2e/client.go` | HTTP 登录/建账户/触发同步等待 + JSON helper |
| `flymail/backend/internal/e2e/probe_test.go` | 连通 + 探测 GreenMail 默认文件夹/能力 |
| `flymail/backend/internal/e2e/sync_test.go` | 收信链路 |
| `flymail/backend/internal/e2e/writeback_test.go` | 回写链路 |
| `flymail/backend/internal/e2e/verbs_test.go` | 发送/删除/移动 |
| `flymail/backend/internal/e2e/realtime_test.go` | IDLE/SSE |
| `flymail/backend/internal/e2e/README.md` | 运行说明（模式 A 主、模式 B 附） |

---

## Task 1: GreenMail 容器编排 + 脚本

**Files:** Create `docker-compose.e2e.yml`、`e2e.sh`（仓库根）

- [ ] **Step 1: docker-compose.e2e.yml**
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

- [ ] **Step 2: e2e.sh**
```bash
#!/usr/bin/env bash
# 在 Linux 上运行 FlyMail 后端 E2E（需 Docker）。
set -euo pipefail
cd "$(dirname "$0")"

KEEP_UP="${KEEP_UP:-0}"
COMPOSE="docker compose -f docker-compose.e2e.yml"

echo "[e2e] starting GreenMail..."
$COMPOSE up -d

echo "[e2e] waiting for GreenMail REST API (:8080)..."
for i in $(seq 1 30); do
  if curl -fsS "http://localhost:8080/api/service/readiness" >/dev/null 2>&1; then
    echo "[e2e] GreenMail ready"; break
  fi
  sleep 1
  if [ "$i" -eq 30 ]; then echo "[e2e] GreenMail not ready, abort"; $COMPOSE logs; exit 1; fi
done

echo "[e2e] running tests..."
set +e
( cd flymail/backend && E2E_GREENMAIL=1 go test ./internal/e2e/... -p 1 -count=1 -timeout 180s -v )
RC=$?
set -e

if [ "$KEEP_UP" = "1" ]; then
  echo "[e2e] KEEP_UP=1, leaving GreenMail running"
else
  $COMPOSE down
fi
exit $RC
```
> 注意：`/api/service/readiness` 端点名在 Task 5 探针里再核实；若不对，e2e.sh 健康检查改用 `/api/messages` 或 TCP 探测 3143。

- [ ] **Step 3: 标记可执行（Linux 上）**：`chmod +x e2e.sh`（Windows 上 git 提交后 Linux 端 `chmod`；或在 README 注明）。

- [ ] **Step 4: Commit**（由 controller 提交，见执行约定）
```
git add docker-compose.e2e.yml e2e.sh
git commit -m "test(e2e): GreenMail 容器编排与运行脚本"
```

---

## Task 2: env 配置（可配 + 纯函数单测）

**Files:** Create `flymail/backend/internal/e2e/env.go`、`env_test.go`

- [ ] **Step 1: 写纯函数单测 env_test.go**（可在 Windows 跑，不需 GreenMail）
```go
package e2e

import (
	"os"
	"testing"
)

func TestGreenmailAddrs_Defaults(t *testing.T) {
	os.Unsetenv("GREENMAIL_HOST")
	os.Unsetenv("GREENMAIL_IMAP_PORT")
	os.Unsetenv("GREENMAIL_SMTP_PORT")
	os.Unsetenv("GREENMAIL_API_PORT")
	if got := greenmailIMAPAddr(); got != "localhost:3143" {
		t.Fatalf("IMAP 默认地址错: %q", got)
	}
	if got := greenmailSMTPAddr(); got != "localhost:3025" {
		t.Fatalf("SMTP 默认地址错: %q", got)
	}
	if got := greenmailAPIBase(); got != "http://localhost:8080" {
		t.Fatalf("API 默认地址错: %q", got)
	}
}

func TestGreenmailAddrs_EnvOverride(t *testing.T) {
	t.Setenv("GREENMAIL_HOST", "10.0.0.5")
	t.Setenv("GREENMAIL_IMAP_PORT", "13143")
	if got := greenmailIMAPAddr(); got != "10.0.0.5:13143" {
		t.Fatalf("env 覆盖 IMAP 地址错: %q", got)
	}
}
```

- [ ] **Step 2: 跑测试看失败**：`cd flymail/backend && go test ./internal/e2e/ -run TestGreenmailAddrs -v` → 编译失败（函数未定义）。

- [ ] **Step 3: 实现 env.go**
```go
// Package e2e 提供基于 GreenMail 的端到端测试（需 E2E_GREENMAIL=1 + Docker）。
package e2e

import (
	"os"
	"testing"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func greenmailHost() string    { return envOr("GREENMAIL_HOST", "localhost") }
func greenmailIMAPAddr() string { return greenmailHost() + ":" + envOr("GREENMAIL_IMAP_PORT", "3143") }
func greenmailSMTPAddr() string { return greenmailHost() + ":" + envOr("GREENMAIL_SMTP_PORT", "3025") }
func greenmailAPIBase() string  { return "http://" + greenmailHost() + ":" + envOr("GREENMAIL_API_PORT", "8080") }

// requireE2E 在未启用 E2E（无 GreenMail）时跳过测试。
func requireE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("E2E_GREENMAIL") == "" {
		t.Skip("set E2E_GREENMAIL=1 to run (requires GreenMail + Docker)")
	}
}
```

- [ ] **Step 4: 跑测试看通过**：`go test ./internal/e2e/ -run TestGreenmailAddrs -v` → PASS（2 例）。

- [ ] **Step 5: Commit**：`test(e2e): GreenMail 连接信息 env 可配 + 纯函数单测`

---

## Task 3: app harness

**Files:** Create `flymail/backend/internal/e2e/harness.go`

**核对来源（实现前必读）：**
- `internal/config/config.go` 的 `Config` 结构（DataDir/Server/Auth/Crypto/Log 字段）。
- `internal/app/app.go` 的 `New/Handler/StartBackground/Shutdown` 签名。
- 默认管理员如何产生：读 `modules/auth` 的 service/migrate 与 `internal/database`，确认首次启动是否 seed 默认管理员（用户名/密码），还是需要显式创建。**这是登录的前提，必须查清并在 harness 暴露 `adminUser/adminPass`。**

- [ ] **Step 1: 实现 harness.go**（骨架，字段按核对结果微调）
```go
package e2e

import (
	"net/http/httptest"
	"testing"

	"flymail/internal/app"
	"flymail/internal/config"
)

type testApp struct {
	app     *app.App
	server  *httptest.Server
	baseURL string
}

// newTestApp 起进程内真实 app：临时 SQLite + httptest server + 后台同步。
func newTestApp(t *testing.T) *testApp {
	t.Helper()
	cfg := &config.Config{
		DataDir: t.TempDir(),
		Server:  config.ServerConfig{Host: "127.0.0.1", Port: 0},
		Auth:    config.AuthConfig{JWTSecret: "e2e-test-secret", AccessTokenTTL: 15, RefreshTokenTTL: 168},
		Crypto:  config.CryptoConfig{EncryptionKey: "e2e-test-key-32-bytes-xxxxxxxxxxx"}, // 长度按 crypto.New 要求核对
		Log:     config.LogConfig{Dir: t.TempDir(), Console: false},
	}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	srv := httptest.NewServer(a.Handler())
	a.StartBackground()
	ta := &testApp{app: a, server: srv, baseURL: srv.URL}
	t.Cleanup(func() {
		srv.Close()
		_ = a.Shutdown()
	})
	return ta
}
```
> **必须核对**：① `crypto.New` 对 EncryptionKey 的长度要求（AES-128/256 → 16/32 字节），否则 `app.New` 报错；② 默认管理员：若 migrate 不 seed，harness 需在此调用 auth service 或 DB 直接建一个已知管理员，并暴露 `adminUser()/adminPass()`。把结果做成 harness 的辅助方法。

- [ ] **Step 2: 编译验证**：`go build ./internal/e2e/`（无 GreenMail 也能编译）。`go vet ./internal/e2e/`。

- [ ] **Step 3: Commit**：`test(e2e): 进程内 app harness（临时库+httptest+后台同步）`

---

## Task 4: GreenMail 辅助 + HTTP client

**Files:** Create `flymail/backend/internal/e2e/greenmail.go`、`client.go`

**核对来源：** `core/imap` 的 `Dial(types.IMAPConfig)` 与 Session 方法（SelectFolder/FetchByUIDRange/ListFolders）；`core/types/connection.go` 的 IMAPConfig 字段；auth login 的请求/响应 DTO（`modules/auth/handler.go` 的 login struct + token 响应字段名）；account create 的请求 DTO（`modules/email/account/handler.go` + dto）。

- [ ] **Step 1: greenmail.go**
```go
package e2e

import (
	"fmt"
	"net/smtp"
	"sync/atomic"
	"testing"

	coreimap "flymail-core/imap"
	"flymail-core/types"
)

var mailboxSeq int64

// uniqueMailbox 生成隔离用唯一收件地址。
func uniqueMailbox(t *testing.T) string {
	t.Helper()
	n := atomic.AddInt64(&mailboxSeq, 1)
	return fmt.Sprintf("e2e-%d@localhost", n)
}

// sendSeed 经 GreenMail SMTP 投递一封邮件到 to 的 INBOX。
func sendSeed(t *testing.T, from, to, subject, body string) {
	t.Helper()
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n",
		from, to, subject, body)
	if err := smtp.SendMail(greenmailSMTPAddr(), nil, from, []string{to}, []byte(msg)); err != nil {
		t.Fatalf("sendSeed: %v", err)
	}
}

// imapConnect 直连 GreenMail 做服务器端断言（GreenMail auth.disabled，密码随意）。
func imapConnect(t *testing.T, mailbox string) *coreimap.Session {
	t.Helper()
	host := greenmailHost()
	sess, err := coreimap.Dial(types.IMAPConfig{
		Host: host, Port: 3143, Username: mailbox, Password: "x", Security: types.SecurityNone, // 字段名/常量核对 types
	})
	if err != nil {
		t.Fatalf("imapConnect: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess
}
```
> 核对：IMAPConfig 字段（Host/Port/Username/Password/Security）与 `SecurityNone` 常量名；GREENMAIL_IMAP_PORT 若非 3143 需用 env（这里硬编码 3143，建议改读 env 解析出的 port）。

- [ ] **Step 2: client.go**（HTTP 辅助；DTO 字段务必核对 handler）
```go
package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

type apiClient struct {
	t       *testing.T
	baseURL string
	token   string
}

func (c *apiClient) do(method, path string, body any) (*http.Response, []byte) {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.baseURL+path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, data
}

// login 调 /auth/login 并保存 token。DTO 字段名核对 modules/auth/handler.go。
func (c *apiClient) login(user, pass string) {
	c.t.Helper()
	resp, data := c.do(http.MethodPost, "/api/v1/auth/login",
		map[string]string{"username": user, "password": pass}) // 字段名核对
	if resp.StatusCode != http.StatusOK {
		c.t.Fatalf("login 失败 %d: %s", resp.StatusCode, data)
	}
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	// token 字段名核对（access_token? token?）
	if tok, ok := out["access_token"].(string); ok {
		c.token = tok
	} else {
		c.t.Fatalf("login 响应无 token: %s", data)
	}
}

// triggerSyncAndWait 触发同步并轮询状态至 done/error。
func (c *apiClient) triggerSyncAndWait(accountID int, timeout time.Duration) {
	c.t.Helper()
	c.do(http.MethodPost, "/api/v1/accounts/"+itoa(accountID)+"/sync", nil)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, data := c.do(http.MethodGet, "/api/v1/accounts/"+itoa(accountID)+"/sync/status", nil)
		_ = resp
		var st map[string]any
		_ = json.Unmarshal(data, &st)
		if ph, _ := st["phase"].(string); ph == "done" || ph == "error" { // phase 值核对 sync Status
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	c.t.Fatalf("同步超时（account %d）", accountID)
}
```
> 还需：`itoa`(strconv.Itoa 包装)、`eventually(t, timeout, interval, cond func() bool)` helper、`createAccount(...)` 调 `POST /api/v1/accounts`（请求字段名核对 account create DTO：name/email/imap_host/imap_port/imap_security/smtp_host/smtp_port/password/username）并返回账户 id。把这些补全。

- [ ] **Step 3: 编译 + vet**：`go build ./internal/e2e/ && go vet ./internal/e2e/`。

- [ ] **Step 4: Commit**：`test(e2e): GreenMail SMTP/IMAP 辅助与 HTTP client`

---

## Task 5: 连通性 + 能力探针（probe_test.go）

**目的**：第一个真正连 GreenMail 的测试。摸清 GreenMail 默认文件夹集合、是否支持 MOVE、IDLE 行为，输出诊断日志，供后续测试自适应。

- [ ] **Step 1: 实现 probe_test.go**
```go
package e2e

import "testing"

func TestProbe_GreenMailCapabilities(t *testing.T) {
	requireE2E(t)
	mb := uniqueMailbox(t)
	sendSeed(t, "probe@localhost", mb, "probe", "hello")
	sess := imapConnect(t, mb)

	folders, err := sess.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	t.Logf("GreenMail 默认文件夹: %+v", folders) // 记录：是否只有 INBOX？有无 Sent/Trash

	// 选 INBOX 验证收到种子邮件
	sel, err := sess.SelectFolder("INBOX")
	if err != nil {
		t.Fatalf("SelectFolder INBOX: %v", err)
	}
	t.Logf("INBOX 状态: %+v", sel)
	// 记录 CAPABILITY/MOVE 支持（若 Session 暴露），否则在 verbs_test 用 COPY+EXPUNGE 回退路径断言。
}
```

- [ ] **Step 2: 编译 + Windows skip 验证**：`go test ./internal/e2e/ -run TestProbe -v` → 输出 `SKIP`（无 E2E_GREENMAIL）。

- [ ] **Step 3: Commit**：`test(e2e): GreenMail 连通性与能力探针`

> **用户 Linux 执行点**：此测试需在 Linux 跑出 `t.Logf` 结果（默认文件夹/MOVE/IDLE），据此微调 Task 8 的预建文件夹与断言分支。

---

## Task 6: 收信链路（sync_test.go）

- [ ] **Step 1: 实现**（结构如下，DTO 字段核对）
  1. `ta := newTestApp(t)`；`c := &apiClient{t, ta.baseURL, ""}`；`c.login(adminUser, adminPass)`。
  2. `mb := uniqueMailbox(t)`；`acctID := c.createAccount(指向 GreenMail, email=mb)`。
  3. `sendSeed` 投 3 封不同主题到 mb。
  4. `c.triggerSyncAndWait(acctID, 60s)`。
  5. `GET /api/v1/accounts/{acctID}/folders` → 断言含 INBOX，取 inbox folder id。
  6. `GET /api/v1/folders/{fid}/messages` → 断言 3 封，主题/发件人匹配（响应字段核对 message list DTO）。
  7. `GET /api/v1/messages/{id}` → 断言正文含种子 body。
  8. 断言 inbox `unread_count==3`。

- [ ] **Step 2: 编译 + skip 验证。Step 3: Commit** `test(e2e): 收信链路（SMTP 种子→同步→列表/详情/未读）`

---

## Task 7: 回写链路（writeback_test.go）

- [ ] **Step 1: 实现**
  1. 复用收信流程拿到一封邮件 id 与其 IMAP UID（UID 从 message list DTO 取，字段名核对）。
  2. `POST /api/v1/messages/{id}/read`（已读）。
  3. `eventually(t, 15s, 300ms, ...)`：`imapConnect(mb)` → SelectFolder INBOX → FETCH 该 UID 的 flags → 含 `\Seen`。
  4. 同理 `POST /messages/{id}/flag` → 断言 `\Flagged`。
> 核对：Session 是否有读取单 UID flags 的方法；若无，用 FetchByUIDRange 取 flags 字段（ParsedEmail 的 flags）。

- [ ] **Step 2: 编译 + skip。Step 3: Commit** `test(e2e): 回写链路（已读/星标→IMAP \Seen/\Flagged）`

---

## Task 8: 发送/删除/移动（verbs_test.go）

> **自适应**：依据 Task 5 探针结果。若 GreenMail 默认无 Sent/Trash，则：发送 APPEND 测试改为「断言对方邮箱收到」即可（APPEND 到 Sent 是尽力而为，flymail 仅 warn）；删除断言走「无回收站→EXPUNGE 永久删除」分支（邮件从 INBOX 消失）。若 harness 经 IMAP `CREATE` 预建了 Sent/Trash，则断言移动到对应文件夹。实现时二选一并注释说明依据。

- [ ] **Step 1: 发送**：建两个邮箱账户或一个账户发往另一 GreenMail 地址 `to2`；`POST /api/v1/send`（字段核对 send DTO：account_id/to/subject/body_html 等）→ `imapConnect(to2)` 验证 INBOX 收到。
- [ ] **Step 2: 删除**：对一封邮件 `POST /messages/{id}/delete` → `eventually` 用 imapConnect 验证该 UID 从源文件夹消失（或移到 Trash，依探针）。
- [ ] **Step 3: 移动**（若有目标文件夹）：`POST /messages/{id}/move`（目标 folder id，请求字段核对）→ IMAP 验证。
- [ ] **Step 4: 编译 + skip。Step 5: Commit** `test(e2e): 发送/删除/移动链路`

---

## Task 9: IDLE/SSE 实时（realtime_test.go）

> **最易 flaky**。timeout 放宽（30s）。若 GreenMail IDLE 推送不可靠，降级：投递后等待轮询间隔触发同步，仍经 SSE 验证 new_mail；在测试注释记录降级。

- [ ] **Step 1: 实现**
  1. `newTestApp` + login + 建账户（enabled，使 manager 起 worker 进 IDLE）。
  2. 用 `http.Client` GET `/api/v1/events?access_token={token}`，流式读 `text/event-stream`（解析 `event:`/`data:` 行，事件名/数据结构核对 internal/sse 与前端 useRealtimeSync）。
  3. `sendSeed` 投新邮件。
  4. `eventually(30s)`：从 SSE 流读到 `new_mail`（或等价）事件。
- [ ] **Step 2: 编译 + skip。Step 3: Commit** `test(e2e): IDLE/SSE 新邮件实时推送`

---

## Task 10: 文档 + 用户 Linux 运行验证

**Files:** Create `flymail/backend/internal/e2e/README.md`

- [ ] **Step 1: README.md**：说明
  - 模式 A（推荐）：在 Linux 服务器 `./e2e.sh`（仓库根）。
  - 模式 B：GreenMail 远程常驻，Windows 设 `GREENMAIL_HOST=<ip>` 后 `cd flymail/backend && E2E_GREENMAIL=1 go test ./internal/e2e/... -p 1 -v`。
  - 默认管理员凭证 / 如何登录。
- [ ] **Step 2: Commit** `docs(e2e): 运行说明`
- [ ] **Step 3（用户在 Linux 执行）**：
  1. 同步代码到 Linux → `./e2e.sh`。
  2. 先看 `TestProbe` 输出（默认文件夹/能力），据此回报，必要时调整 Task 8。
  3. 跑全套，记录通过/失败；失败项反馈给开发迭代。

---

## 完成定义（DoD）

- 脚手架（env/harness/greenmail/client）+ 5 个测试文件齐全，Windows 上 `go build ./internal/e2e/` + `go vet` 通过、`go test`（未设 env）全部 SKIP、`env_test` 纯函数 PASS。
- `docker-compose.e2e.yml` + `e2e.sh` + README 齐全。
- **用户在 Linux 跑 `./e2e.sh`**：probe 输出 GreenMail 真实能力；四链路测试通过（IDLE/SSE 允许按降级方案）。这是真正的验收，发生在 Linux。
