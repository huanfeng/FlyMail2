# FlyMail 后端日志改造 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 flymail 后端散落的纯文本 `log.Printf` 升级为基于 zap 的结构化/分级/带上下文字段（JSON）日志，复用并向后兼容改造 core/logger。

**Architecture:** 改造 `flymail-core/logger`（纯增量加 3 个可选字段，默认行为零变化）→ flymail `internal/logging` 重写为接入该 zap 全局单例 → 新增 gin 的 request_id / 结构化访问日志 / recovery 中间件 → 迁移 20 处 `log.Printf` 为结构化日志并补关键点。

**Tech Stack:** Go、go.uber.org/zap、lumberjack、gin、标准库 testing。

**对应 spec:** `docs/superpowers/specs/2026-06-03-flymail-backend-logging-design.md`

**分支:** `feat/flymail-backend-logging`（已创建，spec 已提交）。

---

## 文件结构

| 文件 | 职责 | 动作 |
|------|------|------|
| `core/logger/logger.go` | zap 全局 logger；新增 3 个可选 Config 字段 | 修改 |
| `core/logger/logger_test.go` | 验证默认行为不变 + 新字段生效 | 新建 |
| `flymail/backend/internal/config/config.go` | 新增 `log.level` / `log.format` | 修改 |
| `flymail/backend/internal/logging/logging.go` | 重写为构造 logger.Config 并 Init | 重写 |
| `flymail/backend/internal/logging/middleware.go` | request_id / gin 访问日志 / recovery / FromGin helper | 新建 |
| `flymail/backend/internal/logging/logging_test.go` | 接入冒烟：文件生成 + JSON 可解析 | 新建 |
| `flymail/backend/internal/logging/middleware_test.go` | request_id 头与透传 | 新建 |
| `flymail/backend/internal/app/app.go` | 改 Setup 调用、Shutdown 调 Sync | 修改 |
| `flymail/backend/internal/server/router.go` | 替换 gin 中间件、挂 request_id | 修改 |
| `flymail/backend/modules/system/notify/service.go` | 迁移 4 处日志 | 修改 |
| `flymail/backend/modules/email/sync/writeback.go` | 迁移 5 处日志 | 修改 |
| `flymail/backend/modules/email/send/service.go` | 迁移 3 处日志 | 修改 |
| `flymail/backend/modules/email/sync/manager.go` | 迁移 8 处日志 + account_id 字段 + 同步耗时 | 修改 |

> 说明：`core/` 模块名为 `flymail-core`，flymail 通过 `replace flymail-core => ../../core` 引用，import 路径为 `flymail-core/logger`。

---

## Task 1: core/logger 向后兼容改造

**Files:**
- Modify: `core/logger/logger.go`
- Test: `core/logger/logger_test.go`

- [ ] **Step 1: 写失败测试**

创建 `core/logger/logger_test.go`：

```go
package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 新增字段：WarnErrorPath 指定分离文件路径
func TestInit_WarnErrorPathHonored(t *testing.T) {
	dir := t.TempDir()
	warnPath := filepath.Join(dir, "we.log")
	mainPath := filepath.Join(dir, "main.log")
	cfg := &Config{
		Level:         "info",
		Development:   false,
		OutputPaths:   []string{mainPath},
		EncoderFormat: "json",
		WarnErrorPath: warnPath,
		Rotation:      &RotationConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 1},
	}
	if err := Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Error("boom")
	_ = Sync()
	if _, err := os.Stat(warnPath); err != nil {
		t.Fatalf("warn_error 文件未按 WarnErrorPath 生成: %v", err)
	}
}

// 新增字段：DisableSeparateWarnError 关闭分离文件
func TestInit_DisableSeparateWarnError(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Level:                    "info",
		Development:              false,
		OutputPaths:              []string{filepath.Join(dir, "main.log")},
		EncoderFormat:            "json",
		DisableSeparateWarnError: true,
		WarnErrorPath:            filepath.Join(dir, "we.log"),
		Rotation:                 &RotationConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 1},
	}
	if err := Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Error("boom")
	_ = Sync()
	if _, err := os.Stat(filepath.Join(dir, "we.log")); !os.IsNotExist(err) {
		t.Fatalf("DisableSeparateWarnError=true 时不应生成分离文件, err=%v", err)
	}
}

// 新增字段：EncoderFormat=json 即使非 dev 也输出可解析 JSON
func TestInit_EncoderFormatJSON(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.log")
	cfg := &Config{
		Level:                    "info",
		Development:              false,
		OutputPaths:              []string{mainPath},
		EncoderFormat:            "json",
		DisableSeparateWarnError: true,
		Rotation:                 &RotationConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 1},
	}
	if err := Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Info("hello", String("k", "v"))
	_ = Sync()
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read main.log: %v", err)
	}
	line := strings.TrimSpace(string(data))
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("日志行不是合法 JSON: %q err=%v", line, err)
	}
	if m["msg"] != "hello" || m["k"] != "v" {
		t.Fatalf("JSON 字段不符: %v", m)
	}
}

// 兼容性：WarnErrorPath 为空且非 dev 时，沿用旧默认 ./logs/warn_error.log
func TestInit_DefaultWarnErrorPathUnchanged(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir) // Go 1.24+：隔离工作目录，避免污染仓库
	cfg := &Config{
		Level:         "info",
		Development:   false,
		OutputPaths:   []string{filepath.Join(dir, "main.log")},
		EncoderFormat: "json",
		Rotation:      &RotationConfig{MaxSize: 1, MaxBackups: 1, MaxAge: 1},
	}
	if err := Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Error("boom")
	_ = Sync()
	if _, err := os.Stat(filepath.Join(dir, "logs", "warn_error.log")); err != nil {
		t.Fatalf("默认 ./logs/warn_error.log 行为被破坏: %v", err)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd core && go test ./logger/ -run TestInit -v`
Expected: 编译失败（`Config` 无 `WarnErrorPath`/`DisableSeparateWarnError`/`EncoderFormat` 字段）。

- [ ] **Step 3: 给 Config 增字段**

`core/logger/logger.go` 的 `Config` 结构体（当前第 19-24 行）改为：

```go
// Config represents logger configuration
type Config struct {
	Level       string          `mapstructure:"level"`        // debug, info, warn, error
	Development bool            `mapstructure:"development"`  // development mode
	OutputPaths []string        `mapstructure:"output_paths"` // output destinations
	Rotation    *RotationConfig `mapstructure:"rotation"`     // log rotation config

	// 以下为可选增量字段；零值时沿用历史行为，保证既有调用方不受影响。
	EncoderFormat            string `mapstructure:"encoder_format"`              // "json"/"console"；空→按 Development 推断
	WarnErrorPath            string `mapstructure:"warn_error_path"`             // 分离 warn/error 文件路径；空→沿用 ./logs/warn_error.log
	DisableSeparateWarnError bool   `mapstructure:"disable_separate_warn_error"` // true→不生成分离文件
}
```

- [ ] **Step 4: 改 Init 使用新字段**

`core/logger/logger.go` 中，把编码器选择段（当前第 130-135 行）替换为：

```go
	var encoder zapcore.Encoder
	useConsole := cfg.EncoderFormat == "console" || (cfg.EncoderFormat == "" && cfg.Development)
	if useConsole {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}
```

把分离 warn/error 文件段（当前第 106-128 行）替换为：

```go
	// 分离的 warn/error 文件输出。
	// 兼容：WarnErrorPath 为空且非 development 时，沿用历史默认 ./logs/warn_error.log。
	if !cfg.DisableSeparateWarnError {
		warnErrorPath := cfg.WarnErrorPath
		if warnErrorPath == "" && !cfg.Development {
			warnErrorPath = "./logs/warn_error.log"
		}
		if warnErrorPath != "" {
			if dir := filepath.Dir(warnErrorPath); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return err
				}
			}
			if cfg.Rotation != nil {
				lj := &lumberjack.Logger{
					Filename:   warnErrorPath,
					MaxSize:    cfg.Rotation.MaxSize,
					MaxBackups: cfg.Rotation.MaxBackups,
					MaxAge:     cfg.Rotation.MaxAge,
					Compress:   cfg.Rotation.Compress,
				}
				warnErrorWriters = append(warnErrorWriters, zapcore.AddSync(lj))
			} else {
				warnErrorFile, err := os.OpenFile(warnErrorPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
				if err != nil {
					return err
				}
				warnErrorWriters = append(warnErrorWriters, zapcore.AddSync(warnErrorFile))
			}
		}
	}
```

在文件顶部 import 块加入 `"path/filepath"`（当前 import 只有 `os`、zap、zapcore、lumberjack）：

```go
import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)
```

> 注意：旧逻辑用 `!cfg.Development` 门控且固定 `os.MkdirAll("./logs")`。新逻辑保留「非 dev 默认仍写 ./logs/warn_error.log」，因此原有调用方（mail2im / LicenseServer，均 `Development=false` 且不设新字段）行为不变。

- [ ] **Step 5: 运行测试确认通过**

Run: `cd core && go test ./logger/ -run TestInit -v`
Expected: PASS（4 个用例全过）。

- [ ] **Step 6: 回归编译 core 全模块**

Run: `cd core && go build ./...`
Expected: 无错误。

- [ ] **Step 7: 提交**

```bash
git add core/logger/logger.go core/logger/logger_test.go
git commit -m "feat(core/logger): 增加 WarnErrorPath/EncoderFormat/DisableSeparateWarnError 可选配置(向后兼容)"
```

---

## Task 2: flymail config 增 log.level / log.format

**Files:**
- Modify: `flymail/backend/internal/config/config.go`

- [ ] **Step 1: 给 LogConfig 增字段**

`config.go` 的 `LogConfig`（当前第 27-34 行）改为：

```go
// LogConfig 日志输出与轮转策略。Dir 为空时默认 <dataDir>/logs。
type LogConfig struct {
	Dir        string `mapstructure:"dir"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`  // 单文件最大 MB
	MaxBackups int    `mapstructure:"max_backups"`  // 保留备份数
	MaxAgeDays int    `mapstructure:"max_age_days"` // 备份保留天数
	Compress   bool   `mapstructure:"compress"`     // 是否压缩旧备份
	Console    bool   `mapstructure:"console"`      // 是否同时输出到控制台
	Level      string `mapstructure:"level"`        // debug/info/warn/error，默认 info
	Format     string `mapstructure:"format"`       // json/console，默认 json
}
```

- [ ] **Step 2: 注册默认值**

`config.go` 的 `Load` 中，在 `v.SetDefault("log.console", true)`（当前第 81 行）后追加：

```go
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
```

- [ ] **Step 3: 编译验证**

Run: `cd flymail/backend && go build ./internal/config/`
Expected: 无错误。

- [ ] **Step 4: 提交**

```bash
git add flymail/backend/internal/config/config.go
git commit -m "feat(flymail/config): 新增 log.level 与 log.format 配置项"
```

---

## Task 3: 重写 flymail internal/logging 接入 zap

**Files:**
- Rewrite: `flymail/backend/internal/logging/logging.go`
- Test: `flymail/backend/internal/logging/logging_test.go`

- [ ] **Step 1: 写失败测试**

创建 `flymail/backend/internal/logging/logging_test.go`：

```go
package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flymail-core/logger"

	"go.uber.org/zap"
)

func TestSetup_WritesParsableJSON(t *testing.T) {
	dir := t.TempDir()
	closeFn, err := Setup(Options{
		Dir:     dir,
		Console: false,
		Level:   "info",
		Format:  "json",
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	logger.Info("接入测试", zap.Uint("account_id", 7))
	_ = closeFn()

	data, err := os.ReadFile(filepath.Join(dir, "flymail.log"))
	if err != nil {
		t.Fatalf("read flymail.log: %v", err)
	}
	last := ""
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(l) != "" {
			last = l
		}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(last), &m); err != nil {
		t.Fatalf("日志行非合法 JSON: %q err=%v", last, err)
	}
	if m["msg"] != "接入测试" {
		t.Fatalf("msg 字段不符: %v", m)
	}
	if _, ok := m["account_id"]; !ok {
		t.Fatalf("缺 account_id 字段: %v", m)
	}
}

func TestSetup_LevelFiltersDebug(t *testing.T) {
	dir := t.TempDir()
	closeFn, err := Setup(Options{Dir: dir, Console: false, Level: "info", Format: "json"})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	logger.Debug("不应出现")
	logger.Info("应出现")
	_ = closeFn()

	data, _ := os.ReadFile(filepath.Join(dir, "flymail.log"))
	if strings.Contains(string(data), "不应出现") {
		t.Fatalf("info 级别下 debug 日志不应写入")
	}
	if !strings.Contains(string(data), "应出现") {
		t.Fatalf("info 日志缺失")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd flymail/backend && go test ./internal/logging/ -run TestSetup -v`
Expected: 编译失败（旧 `Setup` 签名为 `(io.Writer, func() error, error)`，且 `Options` 无 `Level`/`Format`）。

- [ ] **Step 3: 重写 logging.go**

把 `flymail/backend/internal/logging/logging.go` 整体替换为：

```go
// Package logging 基于 flymail-core/logger(zap) 提供结构化 JSON 日志。
// 主输出 <Dir>/flymail.log（lumberjack 轮转）+ 可选 stdout；
// warn/error 另写 <Dir>/warn_error.log；并把标准库 log 收编进 zap。
package logging

import (
	"path/filepath"

	"flymail-core/logger"

	"go.uber.org/zap"
)

// Options 控制日志输出与轮转策略。
type Options struct {
	Dir        string // 日志目录（必填）
	Filename   string // 文件名，默认 flymail.log
	MaxSizeMB  int    // 单文件最大 MB，默认 10
	MaxBackups int    // 保留备份数，默认 5
	MaxAgeDays int    // 备份保留天数，默认 30
	Compress   bool   // 是否 gzip 压缩旧备份，默认 false
	Console    bool   // 是否同时输出到 stdout
	Level      string // debug/info/warn/error，默认 info
	Format     string // json/console，默认 json
}

// Setup 初始化全局 zap logger（flymail-core/logger 单例），返回 close 函数（内部 Sync）。
func Setup(opts Options) (func() error, error) {
	if opts.Filename == "" {
		opts.Filename = "flymail.log"
	}
	if opts.MaxSizeMB <= 0 {
		opts.MaxSizeMB = 10
	}
	if opts.MaxBackups <= 0 {
		opts.MaxBackups = 5
	}
	if opts.MaxAgeDays <= 0 {
		opts.MaxAgeDays = 30
	}
	if opts.Level == "" {
		opts.Level = "info"
	}
	if opts.Format == "" {
		opts.Format = "json"
	}

	outputs := []string{filepath.Join(opts.Dir, opts.Filename)}
	if opts.Console {
		outputs = append(outputs, "stdout")
	}

	cfg := &logger.Config{
		Level:         opts.Level,
		Development:   false,
		OutputPaths:   outputs,
		EncoderFormat: opts.Format,
		WarnErrorPath: filepath.Join(opts.Dir, "warn_error.log"),
		Rotation: &logger.RotationConfig{
			MaxSize:    opts.MaxSizeMB,
			MaxBackups: opts.MaxBackups,
			MaxAge:     opts.MaxAgeDays,
			Compress:   opts.Compress,
		},
	}
	if err := logger.Init(cfg); err != nil {
		return nil, err
	}

	// 把标准库 log（残留 log.Printf 与第三方）收编进 zap（迁移期安全网）。
	_ = zap.RedirectStdLog(logger.Logger)

	return func() error { return logger.Sync() }, nil
}
```

> 说明：core/logger.Init 对 `OutputPaths` 中非 `stdout`/`stderr` 的路径会用 lumberjack（带 Rotation）写文件，因此 flymail.log 自动获得轮转。

- [ ] **Step 4: 运行测试确认通过**

Run: `cd flymail/backend && go test ./internal/logging/ -run TestSetup -v`
Expected: PASS（2 个用例）。

> 若报缺少 zap 依赖，先执行 Step 5 的 `go mod tidy` 再跑。

- [ ] **Step 5: 同步 go.mod 依赖**

Run: `cd flymail/backend && go mod tidy`
Expected: `go.uber.org/zap` 被加入 require。

- [ ] **Step 6: 提交**

```bash
git add flymail/backend/internal/logging/logging.go flymail/backend/internal/logging/logging_test.go flymail/backend/go.mod flymail/backend/go.sum
git commit -m "feat(flymail/logging): 重写为接入 core zap(JSON+轮转+收编标准库 log)"
```

---

## Task 4: gin 中间件（request_id / 访问日志 / recovery / helper）

**Files:**
- Create: `flymail/backend/internal/logging/middleware.go`
- Test: `flymail/backend/internal/logging/middleware_test.go`

- [ ] **Step 1: 写失败测试**

创建 `flymail/backend/internal/logging/middleware_test.go`：

```go
package logging

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestID_GeneratesAndSetsHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	var seen string
	r.GET("/x", func(c *gin.Context) {
		v, _ := c.Get(RequestIDKey)
		seen, _ = v.(string)
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(w, req)

	if seen == "" {
		t.Fatal("context 未注入 request_id")
	}
	if w.Header().Get(RequestIDHeader) != seen {
		t.Fatalf("响应头 X-Request-ID(%q) 与 context(%q) 不一致", w.Header().Get(RequestIDHeader), seen)
	}
}

func TestRequestID_PassthroughIncoming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(RequestIDHeader, "abc123")
	r.ServeHTTP(w, req)

	if got := w.Header().Get(RequestIDHeader); got != "abc123" {
		t.Fatalf("应透传传入的 X-Request-ID，got=%q", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `cd flymail/backend && go test ./internal/logging/ -run TestRequestID -v`
Expected: 编译失败（`RequestID`/`RequestIDKey`/`RequestIDHeader` 未定义）。

- [ ] **Step 3: 实现 middleware.go**

创建 `flymail/backend/internal/logging/middleware.go`：

```go
package logging

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"flymail-core/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	// RequestIDKey 是 request_id 在 gin.Context 中的键。
	RequestIDKey = "request_id"
	// RequestIDHeader 是 request_id 的 HTTP 头名。
	RequestIDHeader = "X-Request-ID"
)

// genRequestID 生成 16 位 hex 短 ID；失败时返回空串（不阻断请求）。
func genRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}

// RequestID 为每个请求生成（或透传传入的）request_id，写入 context 与响应头。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(RequestIDHeader)
		if rid == "" {
			rid = genRequestID()
		}
		c.Set(RequestIDKey, rid)
		c.Header(RequestIDHeader, rid)
		c.Next()
	}
}

// GinLogger 记录结构化访问日志；skipPaths 中的路径不记录（健康检查/SSE 长连接）。
func GinLogger(skipPaths ...string) gin.HandlerFunc {
	skip := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = struct{}{}
	}
	return func(c *gin.Context) {
		if _, ok := skip[c.Request.URL.Path]; ok {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", status),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
		}
		if rid, ok := c.Get(RequestIDKey); ok {
			fields = append(fields, zap.String("request_id", rid.(string)))
		}
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("error", c.Errors.String()))
		}

		switch {
		case status >= 500:
			logger.Error("http request", fields...)
		case status >= 400:
			logger.Warn("http request", fields...)
		default:
			logger.Info("http request", fields...)
		}
	}
}

// GinRecovery 捕获 panic 并记录带堆栈的 error 日志，返回 500。
func GinRecovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, err any) {
		fields := []zap.Field{
			zap.Any("panic", err),
			zap.String("path", c.Request.URL.Path),
			zap.Stack("stack"),
		}
		if rid, ok := c.Get(RequestIDKey); ok {
			fields = append(fields, zap.String("request_id", rid.(string)))
		}
		logger.Error("panic recovered", fields...)
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}

// FromGin 返回带 request_id 字段的子 logger，供 handler 内打日志关联请求。
func FromGin(c *gin.Context) *zap.Logger {
	if rid, ok := c.Get(RequestIDKey); ok {
		return logger.With(zap.String("request_id", rid.(string)))
	}
	return logger.With()
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `cd flymail/backend && go test ./internal/logging/ -v`
Expected: PASS（Setup 2 + RequestID 2 共 4 个用例）。

- [ ] **Step 5: 提交**

```bash
git add flymail/backend/internal/logging/middleware.go flymail/backend/internal/logging/middleware_test.go
git commit -m "feat(flymail/logging): 新增 request_id/访问日志/recovery gin 中间件与 FromGin helper"
```

---

## Task 5: app.go 与 router.go 接入新日志

**Files:**
- Modify: `flymail/backend/internal/app/app.go:44-57`、`:153-168`
- Modify: `flymail/backend/internal/server/router.go:44-51`

- [ ] **Step 1: 改 app.go 的 Setup 调用**

`app.go` 中把日志初始化段（当前第 44-57 行）替换为：

```go
	// 最先初始化统一日志：core zap 全局单例，标准库 log 经 RedirectStdLog 收编。
	logClose, err := logging.Setup(logging.Options{
		Dir:        cfg.LogDir(),
		MaxSizeMB:  cfg.Log.MaxSizeMB,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAgeDays: cfg.Log.MaxAgeDays,
		Compress:   cfg.Log.Compress,
		Console:    cfg.Log.Console,
		Level:      cfg.Log.Level,
		Format:     cfg.Log.Format,
	})
	if err != nil {
		return nil, err
	}
```

> 这删除了旧的 `logWriter` 返回值与 `gin.DefaultWriter = logWriter` / `gin.DefaultErrorWriter = logWriter` 两行（gin 日志已由中间件接管）。`logClose` 仍赋给 `App.logClose`（见 Step 2 的 return 已有 `logClose: logClose`，无需改）。

- [ ] **Step 2: 确认 import 不再需要 gin 仅为 DefaultWriter**

`app.go` 仍在别处用到 `gin`？当前仅 `gin.DefaultWriter` 两处。删除后若 `gin` 未再被引用，移除 import `"github.com/gin-gonic/gin"`。

Run: `cd flymail/backend && go build ./internal/app/`
Expected: 若报 `"github.com/gin-gonic/gin" imported and not used`，删除该 import 行后重新构建通过。

- [ ] **Step 3: 改 router.go 中间件**

`router.go` 中把中间件装配段（当前第 44-51 行）替换为：

```go
	r := gin.New()
	// request_id 必须最先注册，使后续访问日志能带上它。
	r.Use(logging.RequestID())
	// 结构化访问日志；跳过健康检查与长连接 SSE，避免噪音。
	r.Use(logging.GinLogger("/api/v1/healthz", "/api/v1/events"))
	r.Use(logging.GinRecovery())
	r.Use(cors.Default())
```

在 `router.go` 的 import 块加入 `"flymail/internal/logging"`（与其他 `flymail/...` import 并列）。

- [ ] **Step 4: 编译验证**

Run: `cd flymail/backend && go build ./...`
Expected: 无错误。

- [ ] **Step 5: 运行既有测试确认无回归**

Run: `cd flymail/backend && go test ./internal/... ./modules/...`
Expected: 全部 PASS（沿用旧有约 130 用例 + 本计划新增）。

- [ ] **Step 6: 提交**

```bash
git add flymail/backend/internal/app/app.go flymail/backend/internal/server/router.go
git commit -m "feat(flymail): app/router 接入 zap 日志与 request_id/recovery 中间件"
```

---

## Task 6: 迁移 notify/service.go（4 处）

**Files:**
- Modify: `flymail/backend/modules/system/notify/service.go:42,48,68,79`

- [ ] **Step 1: 替换 4 处日志**

逐处替换（保持周边代码不变）：

第 42 行：
```go
		logger.Error("notify: 写入站内通知失败", zap.Error(err))
```
第 48 行：
```go
		logger.Warn("notify: 分发队列已满，丢弃外发", zap.String("type", string(evt.Type)))
```
第 68 行：
```go
		logger.Error("notify: 查询启用渠道失败", zap.Error(err))
```
第 79 行：
```go
			logger.Error("notify: 写入投递日志失败", zap.Error(lerr))
```

- [ ] **Step 2: 调整 import**

`service.go` 顶部 import：删除 `"log"`，加入：
```go
	"flymail-core/logger"

	"go.uber.org/zap"
```

- [ ] **Step 3: 格式化 + 编译**

Run: `cd flymail/backend && gofmt -w modules/system/notify/service.go && go build ./modules/system/notify/`
Expected: 无错误。

- [ ] **Step 4: 提交**

```bash
git add flymail/backend/modules/system/notify/service.go
git commit -m "refactor(notify): 迁移日志到结构化 zap"
```

---

## Task 7: 迁移 sync/writeback.go（5 处）

**Files:**
- Modify: `flymail/backend/modules/email/sync/writeback.go:27,51,56,102,107`

- [ ] **Step 1: 替换 5 处日志**

第 27 行：
```go
		logger.Warn("sync/writeback: 队列已满，丢弃回写操作",
			zap.Uint("uid", uint(op.uid)), zap.Uint("account_id", op.accountID))
```
第 51 行：
```go
		logger.Error("sync/writeback: 取文件夹失败",
			zap.Uint("folder_id", op.folderID), zap.Error(err))
```
第 56 行：
```go
		logger.Error("sync/writeback: 取 IMAP 配置失败",
			zap.Uint("account_id", op.accountID), zap.Error(err))
```
第 102-103 行（整条）：
```go
		logger.Error("sync/writeback: 放弃回写",
			zap.Uint("uid", uint(op.uid)), zap.Uint("account_id", op.accountID),
			zap.Int("attempt", op.attempt), zap.Error(err))
```
第 107-108 行（整条）：
```go
	logger.Warn("sync/writeback: 重试回写",
		zap.Int("attempt", op.attempt), zap.Int("max_retry", maxRetry),
		zap.Uint("uid", uint(op.uid)), zap.Duration("backoff", backoff), zap.Error(err))
```

> 注：`op.uid` 是 go-imap 的 `imapv2.UID`（底层 uint32），用 `uint(op.uid)` 转换以匹配 `zap.Uint`。若编译报类型不符，改用 `zap.Uint32("uid", uint32(op.uid))`。

- [ ] **Step 2: 调整 import**

`writeback.go` 顶部 import：删除 `"log"`，加入 `"flymail-core/logger"` 与 `"go.uber.org/zap"`。

- [ ] **Step 3: 格式化 + 编译**

Run: `cd flymail/backend && gofmt -w modules/email/sync/writeback.go && go build ./modules/email/sync/`
Expected: 无错误（如类型报错按 Step 1 注释改 `zap.Uint32`）。

- [ ] **Step 4: 提交**

```bash
git add flymail/backend/modules/email/sync/writeback.go
git commit -m "refactor(sync/writeback): 迁移日志到结构化 zap"
```

---

## Task 8: 迁移 send/service.go（3 处）

**Files:**
- Modify: `flymail/backend/modules/email/send/service.go:119,124,127`

- [ ] **Step 1: 替换 3 处日志**

第 119 行：
```go
		logger.Warn("send: 查找已发送文件夹失败",
			zap.Uint("account_id", req.AccountID), zap.Error(err))
```
第 124 行：
```go
			logger.Warn("send: 取 IMAP 配置以 APPEND 失败",
				zap.Uint("account_id", req.AccountID), zap.Error(err))
```
第 127 行：
```go
				logger.Warn("send: APPEND 到已发送文件夹失败",
					zap.String("folder", sentFolder.Path), zap.Error(err))
```

> 这些均为「尽力而为」非致命路径，故用 `Warn`。

- [ ] **Step 2: 调整 import**

`send/service.go` 顶部 import：删除 `"log"`，加入 `"flymail-core/logger"` 与 `"go.uber.org/zap"`。

- [ ] **Step 3: 格式化 + 编译**

Run: `cd flymail/backend && gofmt -w modules/email/send/service.go && go build ./modules/email/send/`
Expected: 无错误。

- [ ] **Step 4: 提交**

```bash
git add flymail/backend/modules/email/send/service.go
git commit -m "refactor(send): 迁移日志到结构化 zap"
```

---

## Task 9: 迁移 sync/manager.go（8 处）+ account_id 字段 + 同步耗时

**Files:**
- Modify: `flymail/backend/modules/email/sync/manager.go:135,136,147,175,287,332,336,339`

- [ ] **Step 1: 替换 worker 生命周期日志（135-136 行）**

```go
	logger.Info("sync-manager: worker 启动", zap.Uint("account_id", accountID))
	defer logger.Info("sync-manager: worker 退出", zap.Uint("account_id", accountID))
```

- [ ] **Step 2: 替换重连日志（147 行）**

```go
			logger.Warn("sync-manager: 会话结束，准备重连",
				zap.Uint("account_id", accountID), zap.Duration("backoff", backoff), zap.Error(err))
```

- [ ] **Step 3: 替换连接成功日志（175 行）**

```go
	logger.Info("sync-manager: 已连接",
		zap.Uint("account_id", accountID), zap.Bool("idle", sess.CanIDLE()))
```

- [ ] **Step 4: 替换 pollAll 列文件夹失败（287 行）**

```go
		logger.Error("sync-manager: 列文件夹失败",
			zap.Uint("account_id", accountID), zap.Error(err))
```

- [ ] **Step 5: 替换 syncFolder 三处（332、336、339-340 行）**

第 332-333 行：
```go
		logger.Error("sync-manager: 增量同步失败",
			zap.Uint("account_id", accountID), zap.String("folder", f.Path), zap.Error(err))
```
第 336 行：
```go
		logger.Warn("sync-manager: 回写同步状态失败",
			zap.Uint("account_id", accountID), zap.String("folder", f.Path), zap.Error(err))
```
第 339-340 行（整条）：
```go
	logger.Info("sync-manager: 文件夹同步完成",
		zap.Uint("account_id", accountID), zap.String("folder", f.Path),
		zap.Int("local", state.Total), zap.Int("unread", state.Unread),
		zap.Uint("uid_next", uint(state.UIDNext)), zap.Int("new", newCount))
```

> 注：`state.UIDNext` 若为 `imapv2.UID`/uint32，用 `uint(...)` 转换；类型报错则改 `zap.Uint32`。

- [ ] **Step 6: pollAll 增加每轮耗时日志**

在 `pollAll` 方法（当前第 285 行起）开头记录起始时间，并在成功返回前打一条耗时日志。把方法体首尾改为：

```go
func (m *Manager) pollAll(accountID uint, sess Session) error {
	start := time.Now()
	if err := m.folders.SyncFolders(accountID, sess); err != nil {
		logger.Error("sync-manager: 列文件夹失败",
			zap.Uint("account_id", accountID), zap.Error(err))
		return err
	}
	// ...（中间逐文件夹同步逻辑保持不变）...
```

在 `pollAll` 的**成功返回点**（方法末尾 `return nil` 之前）加入：

```go
	logger.Info("sync-manager: 一轮同步完成",
		zap.Uint("account_id", accountID), zap.Duration("duration", time.Since(start)))
	return nil
```

> 实施者注意：阅读 `pollAll` 完整方法体，确认仅有一个成功 `return nil`；若有多个返回路径，仅在最终成功路径加耗时日志，错误路径已各自有日志。

- [ ] **Step 7: 调整 import**

`manager.go` 顶部 import：删除 `"log"`，加入 `"flymail-core/logger"` 与 `"go.uber.org/zap"`（`"time"` 已存在）。

- [ ] **Step 8: 格式化 + 编译 + 测试**

Run: `cd flymail/backend && gofmt -w modules/email/sync/manager.go && go build ./modules/email/sync/ && go test ./modules/email/sync/ -v`
Expected: 编译无错误；sync 包既有测试（manager_test/service_test/mailops_test）全 PASS。

- [ ] **Step 9: 提交**

```bash
git add flymail/backend/modules/email/sync/manager.go
git commit -m "refactor(sync/manager): 迁移日志到结构化 zap，补 account_id 与同步耗时"
```

---

## Task 10: 全量回归 + 真机验证

**Files:** 无（验证任务）

- [ ] **Step 1: 确认无残留 log.Printf**

Run: `cd flymail/backend && grep -rn "log.Print" modules/ internal/ || echo "CLEAN"`
Expected: 输出 `CLEAN`（或仅剩有意保留处——本计划应已清零）。

- [ ] **Step 2: 脱敏核查（敏感字段绝不进日志）**

Run: `cd flymail/backend && grep -rniE "zap\.(String|Any)\([^)]*(password|passwd|secret|token|jwt|credential)" modules/ internal/ || echo "NO-SENSITIVE-IN-LOGS"`
Expected: 输出 `NO-SENSITIVE-IN-LOGS`。若有命中，人工确认该字段非真实凭证（如仅为字段名常量），否则移除。

- [ ] **Step 3: 四方编译回归**

Run（逐个）：
```bash
cd core && go build ./... && go test ./logger/
cd flymail/backend && go build ./... && go test ./...
cd mail2im/backend && go build ./...
cd FlyMailLicenseServer && go build ./...
```
Expected: 全部无错误、测试 PASS（验证 core/logger 改造对其他依赖方零破坏）。

- [ ] **Step 4: 真机验证（手动）**

在 `flymail/` 目录用 dev.ps1 重启后端（用户操作：`! ./dev.ps1` 选重启后端，或菜单项 rebe）。然后检查 `flymail/logs/flymail.log`：
- [ ] 每行为合法 JSON（`level`/`msg`/`time` 字段齐全）
- [ ] 同步日志含 `account_id`
- [ ] 发起一次 HTTP 请求后有 `"msg":"http request"` 行且含 `request_id`、`status`、`latency`
- [ ] `flymail/logs/warn_error.log` 存在且仅含 warn/error 级别
- [ ] 触发一次手动同步，确认 `"sync-manager: 一轮同步完成"` 带 `duration`

- [ ] **Step 5: 更新记忆（可选）**

把「flymail 已接入 core zap 结构化日志、字段规范、request_id」记入项目记忆 `project_flymail_dev_infra.md`。

---

## 完成定义（DoD）

- core/logger 新增 3 字段且默认行为零变化（4 方编译/测试通过）。
- flymail 全后端日志为结构化 JSON，分级正确，同步日志带 `account_id`，HTTP 日志带 `request_id`。
- 20 处 `log.Printf` 全部迁移，无残留。
- `go test ./...`（core + flymail）全绿；真机日志验证清单全部勾选。
