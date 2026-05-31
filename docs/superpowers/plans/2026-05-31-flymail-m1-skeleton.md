# FlyMail 里程碑 1：骨架（CLI + 配置 + DB + 管理员登录 + 前端登录页 + embed）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 搭起 FlyMail 后端骨架与前端登录闭环：能用 CLI 初始化数据库、设置管理员密码，`flymail server` 启动后浏览器访问前端、用管理员账号登录拿到 JWT，并看到一个受保护的空壳页面。

**Architecture:** 单 Go module `flymail`，建在 `core` 之上。`internal/app` 装配 gin 返回 `http.Handler`，被 `cmd/flymail`（server，CGO 关）共用；config 用 viper 多层级 + 数据目录按形态解析；DB 用 `core/database`(glebarez/sqlite，无 CGO) + GORM；登录用 `core/auth`（JWT + bcrypt）。前端 React + Vite + shadcn/ui，构建产物经 `go:embed` 内嵌后端，由同一 gin 实例提供 SPA + `/api/v1`。

**Tech Stack:** Go 1.26 / gin v1.12 / cobra v1.9 / viper v1.21 / gorm v1.31 / SQLite 经 `core.OpenSQLite`（迁移到 glebarez/sqlite，纯 Go 无 CGO）/ golang-jwt v5.3 / React 19 + Vite 6 + TypeScript + shadcn/ui + TanStack Query。

**核对到的 core 真实签名（module `flymail-core`，replace 到 `../../core`）：**
- `flymail-core/config`：`LoadConfig(opts LoadOptions, target any) error`（opts 含 `EnvPrefix/ConfigPath/Defaults`）、`GenerateRandomSecret() string`
- `flymail-core/database`：`OpenSQLite(opts Options) (*gorm.DB, error)`（`Options{Path, LogMode}`）、`Close(db)` — **注意：core 当前用 mattn CGO 驱动 `gorm.io/driver/sqlite`，见下方 DB 驱动决策**
- `flymail-core/auth`：`HashPassword(password) (string, error)`、`VerifyPassword(hash, password) bool`、`NewJWTManager(secret, expireSeconds) *JWTManager`、`GenerateToken(userID, username)`、`ValidateToken(tokenString) (*Claims, error)`（无 refresh 类型，故本计划自实现双 token）
- `flymail-core/crypto`：`NewAESCrypto(key []byte) (*AESCrypto, error)`、`Encrypt/Decrypt`（里程碑 2 账户凭证用）

**DB 驱动决策（已定，2026-05-31）：** 调查发现 core 的 mattn 驱动是建 core 时（commit c1bbc01）的未审视默认值，违背 flymail 原始 DESIGN.md「用 glebarez 避免 CGO」的要求。**决定：迁移 core/database 到 `glebarez/sqlite`（纯 Go，无 CGO）**，见前置 Task 0。flymail 仍走 `core.OpenSQLite`（迁移后即无 CGO），与 mail2im 统一。收益：两产品无 CGO + Docker 可 scratch/alpine + FTS5 免构建标签（modernc 内置）。

**前置说明（关于"保留架构、重写实现"）：** `flymail/backend` 现存的是 core 重构之前的旧实现，本里程碑按"重写骨架"处理：旧 `modules/`、`cmd/`、`internal/`、`pkg/` 先用 `git rm` 清出（历史可恢复），保留 `go.mod`/`go.sum`/`data/`。后续里程碑会按模块逐步重建。

---

## 文件结构（本里程碑产出）

```
flymail/backend/
├── go.mod / go.sum                 # 保留（依赖已齐）
├── main.go                         # 新建：调用 cmd.Execute()
├── cmd/
│   └── root.go / server.go / db.go # cobra 命令：root / server / db init / db reset-admin-password
├── internal/
│   ├── config/config.go            # viper 多层级 + 数据目录解析
│   ├── config/config_test.go
│   ├── database/database.go        # 包装 core/database + AutoMigrate
│   ├── app/app.go                  # 服务生命周期：装配 gin → http.Handler，Start/Shutdown
│   └── server/router.go            # 路由装配 + 中间件 + SPA 服务
├── modules/
│   └── auth/
│       ├── model.go                # AdminUser GORM 模型
│       ├── repository.go / repository_test.go
│       ├── service.go / service_test.go   # 登录/初始化/改密 + JWT 签发/刷新
│       ├── handler.go              # /auth/login /refresh /logout
│       └── middleware.go           # JWT 鉴权中间件
├── web/
│   └── embed.go                    # go:embed 前端 dist
└── data/                           # 运行时数据（保留）

flymail/frontend/                    # React 前端（重建）
├── package.json / vite.config.ts / tsconfig*.json / components.json
├── index.html
└── src/
    ├── main.tsx / App.tsx / index.css
    ├── lib/api.ts                  # axios 实例 + 拦截器
    ├── lib/auth.ts                 # token 存取
    ├── pages/Login.tsx
    ├── pages/Shell.tsx             # 受保护空壳
    └── router.tsx                  # 路由 + 守卫
```

---

## Task 0: 迁移 core/database 到 glebarez（含 mail2im 回归）

**Files:**
- Modify: `core/database/sqlite.go`
- Modify: `core/go.mod`（增 glebarez，移除 gorm.io/driver/sqlite 直接依赖）
- Modify: `mail2im/backend/internal/testutil/testutil.go`、`internal/models/models_test.go`、`internal/core/worker_helpers_test.go`（这些测试直接 import `gorm.io/driver/sqlite`，改为 glebarez 或改走 `core.OpenSQLite`）
- 不动：`FlyMailLicenseServer`（按既定「暂不动」）

> 范围说明：本任务是 flymail M1 的前置基础设施，触及 shared core 与 mail2im，故先做、单独提交、单独回归。

- [ ] **Step 1: 改 core/database/sqlite.go 的 dialector**

把 import `"gorm.io/driver/sqlite"` 换成 `"github.com/glebarez/sqlite"`，函数体 `gorm.Open(sqlite.Open(opts.Path), ...)` 不变（glebarez 的包名同为 `sqlite`，`sqlite.Open(path)` 签名一致）。改后 `core/database/sqlite.go` 顶部为：

```go
package database

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)
```

- [ ] **Step 2: 更新 core 依赖**

```bash
cd core
go get github.com/glebarez/sqlite@latest
go mod tidy
```

- [ ] **Step 3: 改 mail2im 三个测试文件的 import**

把 `"gorm.io/driver/sqlite"` 改为 `"github.com/glebarez/sqlite"`（调用 `sqlite.Open(...)` 不变）。涉及：
- `mail2im/backend/internal/testutil/testutil.go:10`
- `mail2im/backend/internal/models/models_test.go:6`
- `mail2im/backend/internal/core/worker_helpers_test.go:8`

然后 `cd mail2im/backend && go mod tidy`。

- [ ] **Step 4: 回归 — core 与 mail2im 全部测试 + 无 CGO 构建**

```bash
# core
cd core && CGO_ENABLED=0 go build ./... && go test ./...
# mail2im 后端（验证迁移后仍通过，且可无 CGO 编译）
cd ../mail2im/backend && CGO_ENABLED=0 go build ./... && go test ./...
```
Expected: 全部 PASS；`CGO_ENABLED=0` 也能编译（证明已脱离 mattn CGO）。
（PowerShell：`$env:CGO_ENABLED=0; go build ./...`）

- [ ] **Step 5: 提交**

```bash
git add core/database/sqlite.go core/go.mod core/go.sum \
  mail2im/backend/internal/testutil/testutil.go \
  mail2im/backend/internal/models/models_test.go \
  mail2im/backend/internal/core/worker_helpers_test.go \
  mail2im/backend/go.mod mail2im/backend/go.sum
git commit -m "refactor(core): SQLite 驱动迁移到 glebarez，恢复无 CGO"
```

---

## Task 1: 清理旧骨架 + 记录 core API 签名

**Files:**
- Delete: `flymail/backend/cmd/`, `flymail/backend/internal/`, `flymail/backend/modules/`, `flymail/backend/pkg/`, `flymail/backend/examples/`
- Create: `flymail/backend/main.go`
- Reference（只读）: `core/config/`, `core/database/`, `core/auth/`, `core/crypto/`, `core/httputil/`

- [ ] **Step 1: 记录 core 关键签名（写进本任务笔记，后续任务引用）**

core 签名已在规划阶段核对完毕（见计划头部「核对到的 core 真实签名」）。实现时若需复查：

```bash
grep -rn "^func [A-Z]\|^type [A-Z]" core/config core/database core/auth core/crypto core/httputil
grep '^module' core/go.mod   # 确认为 flymail-core
```

已确认：`config.LoadConfig(opts, target)`、`database.OpenSQLite(Options{Path,LogMode})`、`auth.HashPassword`/`auth.VerifyPassword(hash,password)`、`crypto.NewAESCrypto`。**DB 决策：core 已在 Task 0 迁移到 glebarez（无 CGO），flymail 走 `core.OpenSQLite` 与 mail2im 统一**。

- [ ] **Step 2: 清理旧骨架**

```bash
git rm -r flymail/backend/cmd flymail/backend/internal flymail/backend/modules flymail/backend/pkg flymail/backend/examples
```

（历史可恢复；保留 `go.mod`/`go.sum`/`data/`/`api/`/`Makefile`。）

- [ ] **Step 3: 写最小 main.go（暂时占位，让模块可编译）**

`flymail/backend/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"flymail/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 4: 提交**

```bash
git add flymail/backend/main.go
git commit -m "chore(flymail): 清理旧骨架，建立重写起点"
```

---

## Task 2: 配置层 internal/config（viper 多层级 + 数据目录解析）

**Files:**
- Create: `flymail/backend/internal/config/config.go`
- Test: `flymail/backend/internal/config/config_test.go`

- [ ] **Step 1: 写失败测试**

`flymail/backend/internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(LoadOptions{DataDir: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("default port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.DataDir != dir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, dir)
	}
	if cfg.DBPath() != filepath.Join(dir, "flymail.db") {
		t.Errorf("DBPath = %q", cfg.DBPath())
	}
}

func TestEnvOverride(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("FLYMAIL_SERVER_PORT", "9090")
	defer os.Unsetenv("FLYMAIL_SERVER_PORT")
	cfg, err := Load(LoadOptions{DataDir: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("env override port = %d, want 9090", cfg.Server.Port)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/config/ -run TestLoad -v`（在 `flymail/backend/` 下）
Expected: 编译失败（`Load`/`LoadOptions`/`Config` 未定义）。

- [ ] **Step 3: 实现 config**

`flymail/backend/internal/config/config.go`:

```go
package config

import (
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type AuthConfig struct {
	JWTSecret       string `mapstructure:"jwt_secret"`
	AccessTokenTTL  int    `mapstructure:"access_token_ttl"`  // 分钟
	RefreshTokenTTL int    `mapstructure:"refresh_token_ttl"` // 小时
}

type Config struct {
	DataDir string       `mapstructure:"-"`
	Server  ServerConfig `mapstructure:"server"`
	Auth    AuthConfig   `mapstructure:"auth"`
}

func (c *Config) DBPath() string         { return filepath.Join(c.DataDir, "flymail.db") }
func (c *Config) AttachmentsDir() string { return filepath.Join(c.DataDir, "attachments") }

type LoadOptions struct {
	DataDir    string // 数据目录；空则按形态解析（见 ResolveDataDir）
	ConfigFile string // 显式配置文件路径；空则在 DataDir 下找 config.yaml
}

func Load(opts LoadOptions) (*Config, error) {
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = ResolveDataDir()
	}

	v := viper.New()
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("auth.access_token_ttl", 15)    // 15 分钟
	v.SetDefault("auth.refresh_token_ttl", 168)  // 7 天

	v.SetEnvPrefix("FLYMAIL")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if opts.ConfigFile != "" {
		v.SetConfigFile(opts.ConfigFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(dataDir)
	}
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
		// 配置文件不存在时用默认值，不报错
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}
	cfg.DataDir = dataDir
	return cfg, nil
}
```

`flymail/backend/internal/config/datadir.go`（数据目录按形态解析）:

```go
package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// ResolveDataDir 返回默认数据目录。
// server/Docker 形态默认 ./data；桌面形态由 desktop 入口显式传入 OS 用户目录。
func ResolveDataDir() string {
	if d := os.Getenv("FLYMAIL_DATA_DIR"); d != "" {
		return d
	}
	return "data"
}

// UserDataDir 返回桌面形态的 OS 用户数据目录（供 cmd/desktop 使用）。
func UserDataDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		if runtime.GOOS == "windows" {
			base = os.Getenv("APPDATA")
		} else {
			home, _ := os.UserHomeDir()
			base = home
		}
	}
	return filepath.Join(base, "FlyMail")
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/config/ -v`
Expected: PASS（两个用例）。

- [ ] **Step 5: 提交**

```bash
git add flymail/backend/internal/config/
git commit -m "feat(flymail): 配置层 viper 多层级 + 数据目录解析"
```

---

## Task 3: 数据库层 + AdminUser 模型 + AutoMigrate

**Files:**
- Create: `flymail/backend/modules/auth/model.go`
- Create: `flymail/backend/internal/database/database.go`
- Test: `flymail/backend/internal/database/database_test.go`

- [ ] **Step 1: 写 AdminUser 模型**

`flymail/backend/modules/auth/model.go`:

```go
package auth

import "time"

// AdminUser 单管理员账户。
type AdminUser struct {
	ID           uint      `gorm:"primaryKey"`
	Username     string    `gorm:"uniqueIndex;not null"`
	PasswordHash string    `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (AdminUser) TableName() string { return "admin_users" }
```

- [ ] **Step 2: 写失败测试**

`flymail/backend/internal/database/database_test.go`:

```go
package database

import (
	"path/filepath"
	"testing"

	"flymail/modules/auth"
)

func TestOpenAndMigrate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !db.Migrator().HasTable(&auth.AdminUser{}) {
		t.Error("admin_users 表未创建")
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./internal/database/ -v`
Expected: 编译失败（`Open`/`Migrate` 未定义）。

- [ ] **Step 4: 实现 database（走 core.OpenSQLite，与 mail2im 统一）**

`flymail/backend/internal/database/database.go`:

```go
package database

import (
	"flymail/modules/auth"

	coredb "flymail-core/database"
	"gorm.io/gorm"
)

// Open 打开 SQLite 数据库（经 core，mattn 驱动）。
func Open(path string) (*gorm.DB, error) {
	return coredb.OpenSQLite(coredb.Options{Path: path})
}

// Migrate 迁移所有 FlyMail 模型。后续里程碑在此追加模型。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&auth.AdminUser{},
	)
}
```

> `flymail-core/database.OpenSQLite(Options{Path,LogMode})` 已确认；core 在 Task 0 已迁移到 glebarez（纯 Go），故 flymail 经 core 即获无 CGO，依赖经 core 间接引入，无需额外 `go get`。

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/database/ -v`
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add flymail/backend/modules/auth/model.go flymail/backend/internal/database/
git commit -m "feat(flymail): 数据库层 + AdminUser 模型 + AutoMigrate"
```

---

## Task 4: auth repository + service（密码哈希 + 初始化 + 改密）

**Files:**
- Create: `flymail/backend/modules/auth/repository.go`
- Create: `flymail/backend/modules/auth/service.go`
- Test: `flymail/backend/modules/auth/service_test.go`

- [ ] **Step 1: 写 repository**

`flymail/backend/modules/auth/repository.go`:

```go
package auth

import (
	"errors"

	"gorm.io/gorm"
)

var ErrAdminNotFound = errors.New("admin user not found")

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) GetByUsername(username string) (*AdminUser, error) {
	var u AdminUser
	err := r.db.Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAdminNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) Count() (int64, error) {
	var n int64
	err := r.db.Model(&AdminUser{}).Count(&n).Error
	return n, err
}

func (r *Repository) Upsert(u *AdminUser) error {
	return r.db.Save(u).Error
}
```

- [ ] **Step 2: 写失败测试**

`flymail/backend/modules/auth/service_test.go`:

```go
package auth

import (
	"path/filepath"
	"testing"

	"flymail/internal/database"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewService(NewRepository(db), Options{JWTSecret: "test-secret", AccessTTLMin: 15, RefreshTTLHour: 168})
}

func TestSetAdminPasswordAndAuthenticate(t *testing.T) {
	s := newTestService(t)
	if err := s.SetAdminPassword("admin", "p@ssw0rd"); err != nil {
		t.Fatalf("SetAdminPassword: %v", err)
	}
	u, err := s.Authenticate("admin", "p@ssw0rd")
	if err != nil {
		t.Fatalf("Authenticate ok case: %v", err)
	}
	if u.Username != "admin" {
		t.Errorf("username = %q", u.Username)
	}
	if _, err := s.Authenticate("admin", "wrong"); err == nil {
		t.Error("Authenticate 应对错误密码报错")
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./modules/auth/ -run TestSetAdmin -v`
Expected: 编译失败（`Service`/`NewService`/`Options` 未定义）。

- [ ] **Step 4: 实现 service（密码部分；JWT 在 Task 5 补）**

> 用 Task 1 记录的 `core/auth` 真实密码函数名（如 `HashPassword`/`ComparePassword`）。

`flymail/backend/modules/auth/service.go`:

```go
package auth

import (
	"errors"

	coreauth "flymail-core/auth" // 按 core 真实 module 名调整
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type Options struct {
	JWTSecret      string
	AccessTTLMin   int
	RefreshTTLHour int
}

type Service struct {
	repo *Repository
	opts Options
}

func NewService(repo *Repository, opts Options) *Service {
	return &Service{repo: repo, opts: opts}
}

// SetAdminPassword 创建或更新管理员（用于 db init / reset-admin-password）。
func (s *Service) SetAdminPassword(username, password string) error {
	hash, err := coreauth.HashPassword(password) // 按真实签名调整
	if err != nil {
		return err
	}
	existing, err := s.repo.GetByUsername(username)
	if err != nil && !errors.Is(err, ErrAdminNotFound) {
		return err
	}
	if existing == nil {
		return s.repo.Upsert(&AdminUser{Username: username, PasswordHash: hash})
	}
	existing.PasswordHash = hash
	return s.repo.Upsert(existing)
}

// Authenticate 校验用户名密码。
func (s *Service) Authenticate(username, password string) (*AdminUser, error) {
	u, err := s.repo.GetByUsername(username)
	if errors.Is(err, ErrAdminNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !coreauth.VerifyPassword(u.PasswordHash, password) { // core 真实签名：VerifyPassword(hash, password) bool
		return nil, ErrInvalidCredentials
	}
	return u, nil
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./modules/auth/ -run TestSetAdmin -v`
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add flymail/backend/modules/auth/repository.go flymail/backend/modules/auth/service.go flymail/backend/modules/auth/service_test.go
git commit -m "feat(flymail): auth service 密码哈希 + 管理员初始化/改密"
```

---

## Task 5: JWT 签发与刷新

**Files:**
- Modify: `flymail/backend/modules/auth/service.go`
- Test: `flymail/backend/modules/auth/token_test.go`

- [ ] **Step 1: 写失败测试**

`flymail/backend/modules/auth/token_test.go`:

```go
package auth

import "testing"

func TestIssueAndVerifyToken(t *testing.T) {
	s := newTestService(t)
	if err := s.SetAdminPassword("admin", "p@ssw0rd"); err != nil {
		t.Fatalf("SetAdminPassword: %v", err)
	}
	pair, err := s.Login("admin", "p@ssw0rd")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("token 不应为空")
	}
	claims, err := s.VerifyAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("VerifyAccessToken: %v", err)
	}
	if claims.Username != "admin" {
		t.Errorf("claims.Username = %q", claims.Username)
	}

	refreshed, err := s.Refresh(pair.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if refreshed.AccessToken == "" {
		t.Error("刷新后 access token 不应为空")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./modules/auth/ -run TestIssueAndVerify -v`
Expected: 编译失败（`Login`/`VerifyAccessToken`/`Refresh`/`TokenPair`/`Claims` 未定义）。

- [ ] **Step 3: 实现 JWT（基于 core/auth；若 core 未暴露所需 JWT 函数，则在此用 golang-jwt/v5 直接实现）**

> 先核对 `core/auth` 的 JWT 能力。若 core 提供 `GenerateToken/ParseToken`，包装之；否则按下方用 `golang-jwt/v5` 自实现（依赖已在 go.mod）。

在 `service.go` 追加：

```go
import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type Claims struct {
	Username string `json:"username"`
	Type     string `json:"type"` // "access" | "refresh"
	jwt.RegisteredClaims
}

func (s *Service) Login(username, password string) (*TokenPair, error) {
	u, err := s.Authenticate(username, password)
	if err != nil {
		return nil, err
	}
	return s.issuePair(u.Username)
}

func (s *Service) issuePair(username string) (*TokenPair, error) {
	access, err := s.signToken(username, "access", time.Duration(s.opts.AccessTTLMin)*time.Minute)
	if err != nil {
		return nil, err
	}
	refresh, err := s.signToken(username, "refresh", time.Duration(s.opts.RefreshTTLHour)*time.Hour)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

func (s *Service) signToken(username, typ string, ttl time.Duration) (string, error) {
	claims := Claims{
		Username: username,
		Type:     typ,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.opts.JWTSecret))
}

func (s *Service) parseToken(tokenStr string) (*Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(tokenStr, &c, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.opts.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) VerifyAccessToken(tokenStr string) (*Claims, error) {
	c, err := s.parseToken(tokenStr)
	if err != nil {
		return nil, err
	}
	if c.Type != "access" {
		return nil, errors.New("not an access token")
	}
	return c, nil
}

func (s *Service) Refresh(refreshToken string) (*TokenPair, error) {
	c, err := s.parseToken(refreshToken)
	if err != nil {
		return nil, err
	}
	if c.Type != "refresh" {
		return nil, errors.New("not a refresh token")
	}
	return s.issuePair(c.Username)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./modules/auth/ -v`
Expected: PASS（service + token 全部用例）。

- [ ] **Step 5: 提交**

```bash
git add flymail/backend/modules/auth/service.go flymail/backend/modules/auth/token_test.go
git commit -m "feat(flymail): JWT 签发/校验/刷新"
```

---

## Task 6: CLI（cobra root + db init + reset-admin-password）

**Files:**
- Create: `flymail/backend/cmd/root.go`
- Create: `flymail/backend/cmd/db.go`
- Test: `flymail/backend/cmd/db_test.go`

- [ ] **Step 1: 写 root 命令**

`flymail/backend/cmd/root.go`:

```go
package cmd

import "github.com/spf13/cobra"

var (
	dataDir    string
	configFile string
)

var rootCmd = &cobra.Command{
	Use:   "flymail",
	Short: "FlyMail 自托管邮箱客户端",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", "", "数据目录（默认 ./data 或 FLYMAIL_DATA_DIR）")
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "配置文件路径")
}

func Execute() error { return rootCmd.Execute() }
```

- [ ] **Step 2: 写失败测试**

`flymail/backend/cmd/db_test.go`:

```go
package cmd

import (
	"path/filepath"
	"testing"

	"flymail/internal/database"
	"flymail/modules/auth"
)

func TestRunDBInitCreatesAdmin(t *testing.T) {
	dir := t.TempDir()
	if err := runDBInit(dir, "", "admin", "secret123"); err != nil {
		t.Fatalf("runDBInit: %v", err)
	}
	db, err := database.Open(filepath.Join(dir, "flymail.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repo := auth.NewRepository(db)
	u, err := repo.GetByUsername("admin")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if u.PasswordHash == "" {
		t.Error("管理员密码未设置")
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run: `go test ./cmd/ -run TestRunDBInit -v`
Expected: 编译失败（`runDBInit` 未定义）。

- [ ] **Step 4: 实现 db 命令**

`flymail/backend/cmd/db.go`:

```go
package cmd

import (
	"fmt"

	"flymail/internal/config"
	"flymail/internal/database"
	"flymail/modules/auth"

	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{Use: "db", Short: "数据库管理"}

var dbInitCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化数据库并设置管理员账户",
	RunE: func(cmd *cobra.Command, args []string) error {
		username, _ := cmd.Flags().GetString("admin-user")
		password, _ := cmd.Flags().GetString("admin-pass")
		if password == "" {
			return fmt.Errorf("--admin-pass 不能为空")
		}
		return runDBInit(dataDir, configFile, username, password)
	},
}

var dbResetPwCmd = &cobra.Command{
	Use:   "reset-admin-password",
	Short: "重置管理员密码",
	RunE: func(cmd *cobra.Command, args []string) error {
		username, _ := cmd.Flags().GetString("admin-user")
		password, _ := cmd.Flags().GetString("admin-pass")
		if password == "" {
			return fmt.Errorf("--admin-pass 不能为空")
		}
		return runResetAdminPassword(dataDir, configFile, username, password)
	},
}

func init() {
	for _, c := range []*cobra.Command{dbInitCmd, dbResetPwCmd} {
		c.Flags().String("admin-user", "admin", "管理员用户名")
		c.Flags().String("admin-pass", "", "管理员密码")
	}
	dbCmd.AddCommand(dbInitCmd, dbResetPwCmd)
	rootCmd.AddCommand(dbCmd)
}

func openForCLI(dir, cfgFile string) (*config.Config, *auth.Service, error) {
	cfg, err := config.Load(config.LoadOptions{DataDir: dir, ConfigFile: cfgFile})
	if err != nil {
		return nil, nil, err
	}
	db, err := database.Open(cfg.DBPath())
	if err != nil {
		return nil, nil, err
	}
	if err := database.Migrate(db); err != nil {
		return nil, nil, err
	}
	svc := auth.NewService(auth.NewRepository(db), auth.Options{
		JWTSecret:      cfg.Auth.JWTSecret,
		AccessTTLMin:   cfg.Auth.AccessTokenTTL,
		RefreshTTLHour: cfg.Auth.RefreshTokenTTL,
	})
	return cfg, svc, nil
}

func runDBInit(dir, cfgFile, username, password string) error {
	_, svc, err := openForCLI(dir, cfgFile)
	if err != nil {
		return err
	}
	if err := svc.SetAdminPassword(username, password); err != nil {
		return err
	}
	fmt.Printf("数据库已初始化，管理员 %q 已创建\n", username)
	return nil
}

func runResetAdminPassword(dir, cfgFile, username, password string) error {
	_, svc, err := openForCLI(dir, cfgFile)
	if err != nil {
		return err
	}
	if err := svc.SetAdminPassword(username, password); err != nil {
		return err
	}
	fmt.Printf("管理员 %q 密码已重置\n", username)
	return nil
}
```

> 注意：`config.Load` 需确保 `DataDir` 目录存在再开库。若 `database.Open` 因目录不存在失败，在 `openForCLI` 里先 `os.MkdirAll(cfg.DataDir, 0o755)`。

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./cmd/ -v`
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add flymail/backend/cmd/
git commit -m "feat(flymail): CLI db init / reset-admin-password"
```

---

## Task 7: HTTP server 装配（internal/app + internal/server）

**Files:**
- Create: `flymail/backend/internal/server/router.go`
- Create: `flymail/backend/internal/app/app.go`
- Test: `flymail/backend/internal/server/router_test.go`

- [ ] **Step 1: 写失败测试（healthz）**

`flymail/backend/internal/server/router_test.go`:

```go
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	h := New(Deps{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/server/ -v`
Expected: 编译失败（`New`/`Deps` 未定义）。

- [ ] **Step 3: 实现 router**

`flymail/backend/internal/server/router.go`:

```go
package server

import (
	"net/http"

	"flymail/modules/auth"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Deps 路由依赖，后续里程碑在此追加 service。
type Deps struct {
	Auth *auth.Service
}

// New 装配 gin 并返回 http.Handler（单一真相源：server 与 desktop 共用）。
func New(deps Deps) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.Default())

	api := r.Group("/api/v1")
	api.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	if deps.Auth != nil {
		auth.RegisterRoutes(api, deps.Auth)
	}

	return r
}
```

> `auth.RegisterRoutes` 在 Task 8 实现；本步骤因 `deps.Auth == nil` 跳过，测试可通过。

- [ ] **Step 4: 实现 app 生命周期**

`flymail/backend/internal/app/app.go`:

```go
package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"flymail/internal/config"
	"flymail/internal/database"
	"flymail/internal/server"
	"flymail/modules/auth"
)

type App struct {
	cfg    *config.Config
	srv    *http.Server
	addr   string
}

// New 构建 App：开库、迁移、装配 handler。
func New(cfg *config.Config) (*App, error) {
	db, err := database.Open(cfg.DBPath())
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(db); err != nil {
		return nil, err
	}
	authSvc := auth.NewService(auth.NewRepository(db), auth.Options{
		JWTSecret:      cfg.Auth.JWTSecret,
		AccessTTLMin:   cfg.Auth.AccessTokenTTL,
		RefreshTTLHour: cfg.Auth.RefreshTokenTTL,
	})
	handler := server.New(server.Deps{Auth: authSvc})
	return &App{cfg: cfg, srv: &http.Server{Handler: handler}}, nil
}

// Start 在指定地址监听（addr 为空则用配置 host:port）。返回实际监听地址。
func (a *App) Start(addr string) (string, error) {
	if addr == "" {
		addr = fmt.Sprintf("%s:%d", a.cfg.Server.Host, a.cfg.Server.Port)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	a.addr = ln.Addr().String()
	go func() { _ = a.srv.Serve(ln) }()
	return a.addr, nil
}

func (a *App) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.srv.Shutdown(ctx)
}
```

- [ ] **Step 5: 运行测试确认通过**

Run: `go test ./internal/server/ ./internal/app/ -v`
Expected: server PASS（app 无测试则编译通过即可）。

- [ ] **Step 6: 提交**

```bash
git add flymail/backend/internal/server/ flymail/backend/internal/app/
git commit -m "feat(flymail): HTTP server 装配 + app 生命周期"
```

---

## Task 8: auth API（login / refresh / logout + 中间件）

**Files:**
- Create: `flymail/backend/modules/auth/handler.go`
- Create: `flymail/backend/modules/auth/middleware.go`
- Test: `flymail/backend/modules/auth/handler_test.go`

- [ ] **Step 1: 写失败测试**

`flymail/backend/modules/auth/handler_test.go`:

```go
package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"flymail/internal/database"
	"flymail/modules/auth"

	"github.com/gin-gonic/gin"
)

func setup(t *testing.T) (*gin.Engine, *auth.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	svc := auth.NewService(auth.NewRepository(db), auth.Options{JWTSecret: "s", AccessTTLMin: 15, RefreshTTLHour: 168})
	if err := svc.SetAdminPassword("admin", "secret123"); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	auth.RegisterRoutes(r.Group("/api/v1"), svc)
	return r, svc
}

func TestLoginSuccessAndFail(t *testing.T) {
	r, _ := setup(t)

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "secret123"})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.AccessToken == "" {
		t.Error("access_token 为空")
	}

	bad, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(bad)))
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("bad login status = %d, want 401", rec2.Code)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./modules/auth/ -run TestLoginSuccess -v`
Expected: 编译失败（`RegisterRoutes` 未定义）。

- [ ] **Step 3: 实现 handler + 中间件**

`flymail/backend/modules/auth/handler.go`:

```go
package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	g := rg.Group("/auth")
	g.POST("/login", h.login)
	g.POST("/refresh", h.refresh)
	g.POST("/logout", h.logout)
}

type handler struct{ svc *Service }

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *handler) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	pair, err := h.svc.Login(req.Username, req.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败"})
		return
	}
	c.JSON(http.StatusOK, pair)
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *handler) refresh(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	pair, err := h.svc.Refresh(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token 无效"})
		return
	}
	c.JSON(http.StatusOK, pair)
}

func (h *handler) logout(c *gin.Context) {
	// 无状态 JWT：登出由前端丢弃 token 实现；此端点保留用于审计/未来黑名单。
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

`flymail/backend/modules/auth/middleware.go`:

```go
package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const ContextUsernameKey = "username"

// Middleware 校验 Authorization: Bearer <access token>。
func Middleware(svc *Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "缺少凭证"})
			return
		}
		claims, err := svc.VerifyAccessToken(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "凭证无效"})
			return
		}
		c.Set(ContextUsernameKey, claims.Username)
		c.Next()
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./modules/auth/ -v`
Expected: PASS（含 handler 用例）。

- [ ] **Step 5: 提交**

```bash
git add flymail/backend/modules/auth/handler.go flymail/backend/modules/auth/middleware.go flymail/backend/modules/auth/handler_test.go
git commit -m "feat(flymail): auth API login/refresh/logout + JWT 中间件"
```

---

## Task 9: SPA embed + server 命令

**Files:**
- Create: `flymail/backend/web/embed.go`
- Create: `flymail/backend/web/dist/index.html`（占位，前端构建后覆盖）
- Modify: `flymail/backend/internal/server/router.go`
- Create: `flymail/backend/cmd/server.go`

- [ ] **Step 1: 建占位 dist 与 embed**

`flymail/backend/web/dist/index.html`:

```html
<!doctype html><html><head><meta charset="utf-8"><title>FlyMail</title></head><body><div id="root"></div></body></html>
```

`flymail/backend/web/embed.go`:

```go
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// DistFS 返回前端构建产物（dist 根）。
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
```

- [ ] **Step 2: router 增加 SPA fallback**

在 `router.go` 的 `New` 末尾（`return r` 前）追加：

```go
	// SPA 静态资源 + history fallback（非 /api 路径回退到 index.html）
	if sub, err := web.DistFS(); err == nil {
		fileServer := http.FileServer(http.FS(sub))
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			// 已存在的静态文件直接服务，否则回退 index.html
			if _, err := fs.Stat(sub, strings.TrimPrefix(c.Request.URL.Path, "/")); err == nil && c.Request.URL.Path != "/" {
				fileServer.ServeHTTP(c.Writer, c.Request)
				return
			}
			c.Request.URL.Path = "/"
			fileServer.ServeHTTP(c.Writer, c.Request)
		})
	}
```

并补充 import：`"io/fs"`、`"strings"`、`"flymail/web"`。

- [ ] **Step 3: 实现 server 命令**

`flymail/backend/cmd/server.go`:

```go
package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"flymail/internal/app"
	"flymail/internal/config"

	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "启动 FlyMail 服务",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(config.LoadOptions{DataDir: dataDir, ConfigFile: configFile})
		if err != nil {
			return err
		}
		if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
			return err
		}
		a, err := app.New(cfg)
		if err != nil {
			return err
		}
		addr, err := a.Start("")
		if err != nil {
			return err
		}
		fmt.Printf("FlyMail 已启动：http://%s\n", addr)

		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		fmt.Println("正在关闭…")
		return a.Shutdown()
	},
}

func init() { rootCmd.AddCommand(serverCmd) }
```

- [ ] **Step 4: 编译并冒烟**

Run（在 `flymail/backend/`）:
```bash
go build ./...
go vet ./...
```
Expected: 无错误。

- [ ] **Step 5: 提交**

```bash
git add flymail/backend/web/ flymail/backend/internal/server/router.go flymail/backend/cmd/server.go
git commit -m "feat(flymail): SPA embed + server 命令"
```

---

## Task 10: 前端骨架（React + Vite + shadcn + 登录页）

**Files:**
- Create: `flymail/frontend/package.json`, `vite.config.ts`, `tsconfig.json`, `tsconfig.app.json`, `tsconfig.node.json`, `index.html`, `components.json`, `.env.example`
- Create: `flymail/frontend/src/main.tsx`, `App.tsx`, `index.css`, `router.tsx`, `lib/api.ts`, `lib/auth.ts`, `pages/Login.tsx`, `pages/Shell.tsx`

> 参考 mail2im 的 `frontend-react` 现有配置以对齐版本与 shadcn 设置（`ls mail2im/frontend-react` 取 vite/tailwind/components.json 模板）。

- [ ] **Step 1: 初始化项目脚手架**

```bash
cd flymail/frontend
pnpm create vite@latest . --template react-ts   # 若目录非空，手动建 package.json（见下）后 pnpm i
```

`flymail/frontend/package.json`（关键依赖；版本对齐 mail2im/frontend-react）:

```json
{
  "name": "flymail-frontend",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "react": "^19.0.0",
    "react-dom": "^19.0.0",
    "react-router-dom": "^7.0.0",
    "@tanstack/react-query": "^5.0.0",
    "axios": "^1.7.0"
  },
  "devDependencies": {
    "@vitejs/plugin-react": "^4.3.0",
    "typescript": "^5.6.0",
    "vite": "^6.0.0",
    "tailwindcss": "^4.0.0",
    "@tailwindcss/vite": "^4.0.0"
  }
}
```

> shadcn/ui 组件按需 `pnpm dlx shadcn@latest add button input card` 引入；本任务先引入登录页所需 `button`/`input`/`card`/`label`。

- [ ] **Step 2: 配置 Vite 构建输出到后端 embed 目录**

`flymail/frontend/vite.config.ts`:

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwind from "@tailwindcss/vite";
import path from "node:path";

export default defineConfig({
  plugins: [react(), tailwind()],
  resolve: { alias: { "@": path.resolve(__dirname, "./src") } },
  build: { outDir: path.resolve(__dirname, "../backend/web/dist"), emptyOutDir: true },
  server: { proxy: { "/api": "http://localhost:8080" } },
});
```

- [ ] **Step 3: API client + token 存取**

`flymail/frontend/src/lib/auth.ts`:

```ts
const ACCESS = "flymail_access";
const REFRESH = "flymail_refresh";

export const tokenStore = {
  get access() { return localStorage.getItem(ACCESS); },
  get refresh() { return localStorage.getItem(REFRESH); },
  set(pair: { access_token: string; refresh_token: string }) {
    localStorage.setItem(ACCESS, pair.access_token);
    localStorage.setItem(REFRESH, pair.refresh_token);
  },
  clear() { localStorage.removeItem(ACCESS); localStorage.removeItem(REFRESH); },
  get isAuthenticated() { return !!localStorage.getItem(ACCESS); },
};
```

`flymail/frontend/src/lib/api.ts`:

```ts
import axios from "axios";
import { tokenStore } from "./auth";

export const api = axios.create({ baseURL: "/api/v1" });

api.interceptors.request.use((cfg) => {
  const t = tokenStore.access;
  if (t) cfg.headers.Authorization = `Bearer ${t}`;
  return cfg;
});

export async function login(username: string, password: string) {
  const { data } = await api.post("/auth/login", { username, password });
  tokenStore.set(data);
  return data;
}
```

- [ ] **Step 4: 登录页 + 受保护壳 + 路由守卫**

`flymail/frontend/src/pages/Login.tsx`:

```tsx
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { login } from "@/lib/api";

export default function Login() {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const nav = useNavigate();

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await login(username, password);
      nav("/");
    } catch {
      setError("用户名或密码错误");
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center">
      <form onSubmit={onSubmit} className="w-80 space-y-4 rounded-lg border p-6">
        <h1 className="text-xl font-semibold">登录 FlyMail</h1>
        <input className="w-full rounded border px-3 py-2" value={username}
          onChange={(e) => setUsername(e.target.value)} placeholder="用户名" />
        <input className="w-full rounded border px-3 py-2" type="password" value={password}
          onChange={(e) => setPassword(e.target.value)} placeholder="密码" />
        {error && <p className="text-sm text-red-500">{error}</p>}
        <button className="w-full rounded bg-black py-2 text-white" type="submit">登录</button>
      </form>
    </div>
  );
}
```

`flymail/frontend/src/pages/Shell.tsx`:

```tsx
import { tokenStore } from "@/lib/auth";
import { useNavigate } from "react-router-dom";

export default function Shell() {
  const nav = useNavigate();
  return (
    <div className="p-8">
      <h1 className="text-2xl font-bold">FlyMail</h1>
      <p className="text-muted-foreground">已登录。邮件功能将在后续里程碑接入。</p>
      <button className="mt-4 rounded border px-4 py-2"
        onClick={() => { tokenStore.clear(); nav("/login"); }}>退出登录</button>
    </div>
  );
}
```

`flymail/frontend/src/router.tsx`:

```tsx
import { createBrowserRouter, Navigate } from "react-router-dom";
import { tokenStore } from "@/lib/auth";
import Login from "@/pages/Login";
import Shell from "@/pages/Shell";

function RequireAuth({ children }: { children: React.ReactNode }) {
  return tokenStore.isAuthenticated ? <>{children}</> : <Navigate to="/login" replace />;
}

export const router = createBrowserRouter([
  { path: "/login", element: <Login /> },
  { path: "/", element: <RequireAuth><Shell /></RequireAuth> },
]);
```

`flymail/frontend/src/main.tsx`:

```tsx
import React from "react";
import ReactDOM from "react-dom/client";
import { RouterProvider } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { router } from "./router";
import "./index.css";

const qc = new QueryClient();

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={qc}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </React.StrictMode>
);
```

`flymail/frontend/src/index.css`:

```css
@import "tailwindcss";
```

- [ ] **Step 5: 构建前端（产物落到后端 embed 目录）**

```bash
cd flymail/frontend
pnpm install
pnpm build
```
Expected: 在 `flymail/backend/web/dist/` 生成 `index.html` + assets。

- [ ] **Step 6: 提交**

```bash
git add flymail/frontend/ flymail/backend/web/dist/
git commit -m "feat(flymail): 前端骨架 React+Vite+shadcn 登录闭环"
```

---

## Task 11: 端到端冒烟 + 收尾

**Files:** 无新增（验证整体闭环）

- [ ] **Step 1: 后端全部测试**

Run（`flymail/backend/`）: `go test ./...`
Expected: 全部 PASS。

- [ ] **Step 2: 构建 server 二进制（验证无 CGO）**

```bash
cd flymail/backend
CGO_ENABLED=0 go build -o ../../bin/flymail ./
```
Expected: 成功产出静态二进制（PowerShell：`$env:CGO_ENABLED=0; go build -o ../../bin/flymail.exe .`）。证明 glebarez 迁移后 flymail 已脱离 CGO。

- [ ] **Step 3: 手动端到端**

```bash
./bin/flymail db init --admin-user admin --admin-pass secret123 --data-dir ./data
./bin/flymail server --data-dir ./data
```
浏览器打开提示的地址 → 登录页 → 用 admin/secret123 登录 → 进入 Shell → 刷新页面仍在 Shell（token 持久化）→ 退出登录回到登录页。

- [ ] **Step 4: 提交（如有收尾改动）**

```bash
git add -A
git commit -m "chore(flymail): 里程碑1 端到端冒烟收尾"
```

---

## 自检（spec 覆盖 / 占位符 / 类型一致性）

**Spec 覆盖（对应 spec §8 里程碑 1）：**
- CLI `server`/`db init`/`db reset-admin-password` → Task 6、Task 9 ✓
- viper 多层级配置 + 数据目录解析 → Task 2 ✓
- GORM/DB（glebarez 无 CGO）→ Task 3 ✓
- 管理员登录 JWT + 刷新 → Task 4/5/8 ✓
- 前端登录页 → Task 10 ✓
- go embed SPA → Task 9 ✓
- 数据目录按形态解析（为 §2 desktop 预留）→ Task 2（`UserDataDir`）✓

**占位符扫描：** 无 TBD/TODO；所有步骤含真实代码与命令。涉及 core 签名处均有"先核对真实签名"的显式步骤，非占位。

**类型一致性：** `config.Load(LoadOptions)→*Config`、`database.Open(path)`/`Migrate(db)`、`auth.NewService(repo, Options)`、`auth.Service.Login/Refresh/VerifyAccessToken`、`auth.RegisterRoutes(rg, svc)`、`auth.Options{JWTSecret,AccessTTLMin,RefreshTTLHour}`、`server.New(Deps{Auth})`、`app.New(cfg)`/`Start(addr)`/`Shutdown()`、`web.DistFS()` 在各 Task 间命名一致。

**已知外部依赖待核对项（实现时第一步处理，见 Task 1）：**
- `core` module 真实名（`grep '^module' core/go.mod`）与各包 import 路径。
- `core/database` 开库函数名/签名、`core/auth` 密码函数名/参数顺序。若 core 未提供 JWT，则按 Task 5 用 golang-jwt/v5 自实现（依赖已具备）。
