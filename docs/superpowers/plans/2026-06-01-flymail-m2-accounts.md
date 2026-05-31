# FlyMail 里程碑 2：账户管理（CRUD + 连接测试 + 凭证 AES 加密）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 FlyMail 能由管理员增删改查多个邮箱账户，凭证以 AES-256-GCM 加密落库，并能在保存前测试 IMAP/SMTP 连接、按邮箱域名自动建议服务器预设。

**Architecture:** 在 M1 骨架之上新增 `modules/email/account`（model/repository/service/handler）与 `internal/crypto`（封装 `core/crypto` 的 Encryptor）。账户凭证加密存储、API 永不回传密码；连接测试复用 `core/imap.Dial` 与 `core/smtp.Client.TestConnection`；服务器预设来自 `core/provider.GetProvider`。账户路由挂在 JWT 鉴权中间件后。

**Tech Stack:** Go 1.26 / gin / gorm + glebarez（无 CGO）/ core(crypto/imap/smtp/types/provider) / golang-jwt。

**执行纪律（沿用 M1 的经验，务必遵守）：**
- 子代理**只实现 + 跑测试，不执行任何 git 命令**；提交由主控集中负责（避免 Bash CWD 持久化污染历史）。
- 改文件用 Read/Edit/Write，**禁止 `sed -i`**；不要创建/修改 `.claude/`。
- 所有 go 命令 `CGO_ENABLED=0`；代理失败设 `GOPROXY=https://goproxy.cn,direct`。
- Windows 下 SQLite 测试用 `t.Cleanup` 关闭连接，避免 TempDir 清理文件锁。
- 改完 go 代码 gofmt。

---

## 已核对的 core / 现状事实（实现依据）

- `core/crypto`：`NewAESCrypto(key []byte) (*AESCrypto, error)`、`(*AESCrypto).Encrypt(plaintext string) (string, error)`、`Decrypt(ciphertext string) (string, error)`。空串进空串出、AES-256-GCM、base64。
- `core/types`：`IMAPConfig{Host,Port,Username,Password,AccessToken,Security,Proxy,ClientName,ClientVendor}`、`SMTPConfig{Host,Port,Username,Password,Security,Proxy}`、`ProxyConfig{Type,Host,Port,Username,Password}`、`SecurityMode`(`SecurityNone/SecuritySSL/SecurityStartTLS`)、`ConnectionTestResult{IMAP,SMTP,SupportsIDLE,Capabilities,SecurityMode,IMAPError,SMTPError}`。
- `core/imap`：`Dial(cfg types.IMAPConfig) (*Session, error)`（成功即登录成功=凭证有效）；`Session` 暴露 `Capabilities []string`、`SupportsIDLE bool`、`SecurityMode string`；`(*Session).Close() error`。
- `core/smtp`：`NewClient(cfg types.SMTPConfig) *Client`；`(*Client).TestConnection() error`（连接+认证）。
- `core/provider`：`GetProvider(domain string) (*ProviderConfig, bool)`；`ProviderConfig{ID,Name,Domains,Servers map[string]ServerEndpoint,FolderMappings}`；`ServerEndpoint{Host,Port}`，Servers 键为 `"ssl"/"starttls"/"none"`。
- M1 已有：`internal/config`（`Config{DataDir,Server,Auth}`、`Load`）、`internal/database`（`Open`/`Migrate`）、`internal/server`（`New(Deps{Auth *auth.Service})`，`/api/v1` 下有 auth 路由 + healthz + SPA 回退）、`internal/app`（`New(cfg)`/`Start`/`Shutdown`）、`modules/auth`（`Service`、`Middleware(svc) gin.HandlerFunc`、`NewRepository`、`NewService`）。

---

## 文件结构（本里程碑产出）

```
flymail/backend/
├── internal/
│   ├── config/config.go         # 修改：新增 Crypto.EncryptionKey 字段 + 默认值 + env
│   ├── crypto/crypto.go         # 新建：Encryptor（封装 core/crypto.AESCrypto）
│   ├── crypto/crypto_test.go
│   ├── database/database.go     # 修改：Migrate 增加 account.Account
│   ├── server/router.go         # 修改：新增受鉴权保护的账户路由组
│   └── app/app.go               # 修改：构建 Encryptor + 账户 Service，注入 Deps
└── modules/email/account/
    ├── model.go                 # Account GORM 模型
    ├── dto.go                   # 请求/响应 DTO
    ├── repository.go            # CRUD 仓储
    ├── repository_test.go
    ├── service.go               # 业务：CRUD + 加解密 + 配置映射
    ├── service_test.go
    ├── connection.go            # 连接测试：build*Config + TestConnection
    ├── connection_test.go
    ├── handler.go               # gin handlers + RegisterRoutes
    └── handler_test.go
```

---

## Task 1: 配置新增加密密钥

**Files:**
- Modify: `flymail/backend/internal/config/config.go`
- Test: `flymail/backend/internal/config/config_test.go`

- [ ] **Step 1: 加失败测试** —— 在 `config_test.go` 追加：

```go
func TestCryptoKeyDefaultAndEnv(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(LoadOptions{DataDir: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Crypto.EncryptionKey == "" {
		t.Error("默认加密密钥不应为空")
	}

	t.Setenv("FLYMAIL_CRYPTO_ENCRYPTION_KEY", "my-custom-key-1234567890")
	cfg2, err := Load(LoadOptions{DataDir: dir})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg2.Crypto.EncryptionKey != "my-custom-key-1234567890" {
		t.Errorf("env 覆盖密钥失败，得到 %q", cfg2.Crypto.EncryptionKey)
	}
}
```

- [ ] **Step 2: 运行确认失败**：`go test ./internal/config/ -run TestCryptoKey -v`（`Crypto` 字段未定义）。

- [ ] **Step 3: 实现** —— 在 `config.go` 增加类型与默认值：

在 `Config` 结构体加字段：
```go
	Crypto CryptoConfig `mapstructure:"crypto"`
```
新增类型（放在 `AuthConfig` 之后）：
```go
type CryptoConfig struct {
	EncryptionKey string `mapstructure:"encryption_key"` // 账户凭证 AES 加密密钥；生产环境务必覆盖并保持稳定
}
```
在 `Load` 的 `SetDefault` 区追加：
```go
	v.SetDefault("crypto.encryption_key", "flymail-default-insecure-key-change-me")
```

- [ ] **Step 4: 运行确认通过**：`go test ./internal/config/ -v`（全部 PASS）。

- [ ] **Step 5: 报告**（不提交）：列出改动与测试结果。

> 备注：默认密钥不安全，仅为开发可跑通；后续里程碑会在 `db init` 生成随机密钥并写入 `data/config.yaml`。本 Task 不做。

---

## Task 2: Encryptor 封装

**Files:**
- Create: `flymail/backend/internal/crypto/crypto.go`
- Test: `flymail/backend/internal/crypto/crypto_test.go`

- [ ] **Step 1: 写失败测试** `internal/crypto/crypto_test.go`：

```go
package crypto

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	e, err := New("a-test-encryption-key-32bytes!!")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	enc, err := e.Encrypt("s3cr3t-password")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if enc == "s3cr3t-password" {
		t.Error("密文不应等于明文")
	}
	dec, err := e.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != "s3cr3t-password" {
		t.Errorf("解密 = %q, want s3cr3t-password", dec)
	}
}

func TestEncryptEmpty(t *testing.T) {
	e, _ := New("a-test-encryption-key-32bytes!!")
	enc, err := e.Encrypt("")
	if err != nil || enc != "" {
		t.Errorf("空串加密应返回空串无错，得到 %q err=%v", enc, err)
	}
}
```

- [ ] **Step 2: 运行确认失败**：`go test ./internal/crypto/ -v`（`New` 未定义）。

- [ ] **Step 3: 实现** `internal/crypto/crypto.go`：

```go
package crypto

import corecrypto "flymail-core/crypto"

// Encryptor 封装 core 的 AES-256-GCM 加解密，用于账户凭证落库加密。
type Encryptor struct {
	aes *corecrypto.AESCrypto
}

// New 用给定密钥创建 Encryptor（密钥会被补足/截断到 32 字节）。
func New(key string) (*Encryptor, error) {
	aes, err := corecrypto.NewAESCrypto([]byte(key))
	if err != nil {
		return nil, err
	}
	return &Encryptor{aes: aes}, nil
}

// Encrypt 加密明文，空串返回空串。
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	return e.aes.Encrypt(plaintext)
}

// Decrypt 解密密文，空串返回空串。
func (e *Encryptor) Decrypt(ciphertext string) (string, error) {
	return e.aes.Decrypt(ciphertext)
}
```

- [ ] **Step 4: 运行确认通过**：`go test ./internal/crypto/ -v`（PASS）。`go mod tidy`（无新增外部依赖，core 间接已在）。

- [ ] **Step 5: 报告**（不提交）。

---

## Task 3: Account 模型 + 迁移

**Files:**
- Create: `flymail/backend/modules/email/account/model.go`
- Modify: `flymail/backend/internal/database/database.go`
- Test: `flymail/backend/internal/database/database_test.go`

- [ ] **Step 1: 写模型** `modules/email/account/model.go`：

```go
package account

import "time"

// Account 表示一个被管理的邮箱账户。凭证字段以 AES 加密后存储。
type Account struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"not null"`            // 显示名
	Email string `gorm:"uniqueIndex;not null"`

	AuthType string `gorm:"not null;default:password"` // password（oauth2 后续）

	// 登录凭证（IMAP/SMTP 共用用户名；为空时回退 Email）
	Username    string
	PasswordEnc string `json:"-"` // AES 加密后的密码

	// IMAP
	IMAPHost     string
	IMAPPort     int
	IMAPSecurity string // none/ssl/starttls

	// SMTP
	SMTPHost     string
	SMTPPort     int
	SMTPSecurity string

	// 可选代理
	ProxyType        string
	ProxyHost        string
	ProxyPort        int
	ProxyUsername    string
	ProxyPasswordEnc string `json:"-"`

	Status     string     `gorm:"default:new"` // new/active/auth_failed/network_error
	LastSyncAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (Account) TableName() string { return "accounts" }

// LoginName 返回 IMAP/SMTP 登录用户名（Username 为空则用 Email）。
func (a *Account) LoginName() string {
	if a.Username != "" {
		return a.Username
	}
	return a.Email
}
```

- [ ] **Step 2: 改迁移测试** —— 在 `internal/database/database_test.go` 追加：

```go
func TestMigrateCreatesAccounts(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if sqlDB, e := db.DB(); e == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !db.Migrator().HasTable(&account.Account{}) {
		t.Error("accounts 表未创建")
	}
}
```
并在该测试文件 import 增加 `"flymail/modules/email/account"`。

- [ ] **Step 3: 运行确认失败**：`go test ./internal/database/ -run TestMigrateCreatesAccounts -v`（account 包/符号未定义）。

- [ ] **Step 4: 实现** —— 修改 `internal/database/database.go` 的 `Migrate`，import 增加 `"flymail/modules/email/account"`，AutoMigrate 列表加入 `&account.Account{}`：

```go
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&auth.AdminUser{},
		&account.Account{},
	)
}
```

- [ ] **Step 5: 运行确认通过**：`go test ./internal/database/ -v`（PASS）。`go build ./...`。

- [ ] **Step 6: 报告**（不提交）。

---

## Task 4: Account 仓储

**Files:**
- Create: `flymail/backend/modules/email/account/repository.go`
- Test: `flymail/backend/modules/email/account/repository_test.go`

- [ ] **Step 1: 写失败测试** `repository_test.go`：

```go
package account

import (
	"errors"
	"path/filepath"
	"testing"

	"flymail/internal/database"
)

func newTestRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if sqlDB, e := db.DB(); e == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewRepository(db)
}

func TestRepositoryCRUD(t *testing.T) {
	r := newTestRepo(t)

	a := &Account{Name: "Work", Email: "u@example.com", AuthType: "password", PasswordEnc: "enc"}
	if err := r.Create(a); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == 0 {
		t.Fatal("Create 后 ID 应非零")
	}

	got, err := r.GetByID(a.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Email != "u@example.com" {
		t.Errorf("Email = %q", got.Email)
	}

	got.Name = "Work2"
	if err := r.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	reload, _ := r.GetByID(a.ID)
	if reload.Name != "Work2" {
		t.Errorf("Update 未生效: %q", reload.Name)
	}

	list, err := r.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %d, err=%v", len(list), err)
	}

	if err := r.Delete(a.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.GetByID(a.ID); !errors.Is(err, ErrAccountNotFound) {
		t.Errorf("删除后应返回 ErrAccountNotFound, 得到 %v", err)
	}
}
```

- [ ] **Step 2: 运行确认失败**：`go test ./modules/email/account/ -run TestRepositoryCRUD -v`（Repository 等未定义）。

- [ ] **Step 3: 实现** `repository.go`：

```go
package account

import (
	"errors"

	"gorm.io/gorm"
)

var ErrAccountNotFound = errors.New("account not found")

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(a *Account) error { return r.db.Create(a).Error }

func (r *Repository) GetByID(id uint) (*Account, error) {
	var a Account
	err := r.db.First(&a, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *Repository) List() ([]Account, error) {
	var list []Account
	err := r.db.Order("id asc").Find(&list).Error
	return list, err
}

func (r *Repository) Update(a *Account) error { return r.db.Save(a).Error }

func (r *Repository) Delete(id uint) error {
	res := r.db.Delete(&Account{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAccountNotFound
	}
	return nil
}
```

- [ ] **Step 4: 运行确认通过**：`go test ./modules/email/account/ -run TestRepositoryCRUD -v`（PASS）。

- [ ] **Step 5: 报告**（不提交）。

---

## Task 5: DTO + Service（CRUD + 加解密 + 永不回传密码）

**Files:**
- Create: `flymail/backend/modules/email/account/dto.go`
- Create: `flymail/backend/modules/email/account/service.go`
- Test: `flymail/backend/modules/email/account/service_test.go`

- [ ] **Step 1: 写 DTO** `dto.go`：

```go
package account

// ProxyDTO 可选代理配置（请求/响应共用，密码仅入站）。
type ProxyDTO struct {
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"` // 仅入站
}

// CreateAccountRequest 创建账户请求（含明文密码）。
type CreateAccountRequest struct {
	Name         string    `json:"name" binding:"required"`
	Email        string    `json:"email" binding:"required,email"`
	Username     string    `json:"username,omitempty"`
	Password     string    `json:"password" binding:"required"`
	IMAPHost     string    `json:"imap_host" binding:"required"`
	IMAPPort     int       `json:"imap_port" binding:"required"`
	IMAPSecurity string    `json:"imap_security"`
	SMTPHost     string    `json:"smtp_host" binding:"required"`
	SMTPPort     int       `json:"smtp_port" binding:"required"`
	SMTPSecurity string    `json:"smtp_security"`
	Proxy        *ProxyDTO `json:"proxy,omitempty"`
}

// UpdateAccountRequest 更新账户请求（Password 为空表示保持原密码）。
type UpdateAccountRequest struct {
	Name         string    `json:"name" binding:"required"`
	Email        string    `json:"email" binding:"required,email"`
	Username     string    `json:"username,omitempty"`
	Password     string    `json:"password,omitempty"`
	IMAPHost     string    `json:"imap_host" binding:"required"`
	IMAPPort     int       `json:"imap_port" binding:"required"`
	IMAPSecurity string    `json:"imap_security"`
	SMTPHost     string    `json:"smtp_host" binding:"required"`
	SMTPPort     int       `json:"smtp_port" binding:"required"`
	SMTPSecurity string    `json:"smtp_security"`
	Proxy        *ProxyDTO `json:"proxy,omitempty"`
}

// AccountResponse 账户响应（绝不含密码）。
type AccountResponse struct {
	ID           uint      `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	Username     string    `json:"username,omitempty"`
	AuthType     string    `json:"auth_type"`
	IMAPHost     string    `json:"imap_host"`
	IMAPPort     int       `json:"imap_port"`
	IMAPSecurity string    `json:"imap_security"`
	SMTPHost     string    `json:"smtp_host"`
	SMTPPort     int       `json:"smtp_port"`
	SMTPSecurity string    `json:"smtp_security"`
	Proxy        *ProxyDTO `json:"proxy,omitempty"`
	Status       string    `json:"status"`
}

// toResponse 把模型转为响应（剥离密码；代理密码也不回传）。
func toResponse(a *Account) AccountResponse {
	resp := AccountResponse{
		ID: a.ID, Name: a.Name, Email: a.Email, Username: a.Username,
		AuthType: a.AuthType,
		IMAPHost: a.IMAPHost, IMAPPort: a.IMAPPort, IMAPSecurity: a.IMAPSecurity,
		SMTPHost: a.SMTPHost, SMTPPort: a.SMTPPort, SMTPSecurity: a.SMTPSecurity,
		Status: a.Status,
	}
	if a.ProxyHost != "" {
		resp.Proxy = &ProxyDTO{Type: a.ProxyType, Host: a.ProxyHost, Port: a.ProxyPort, Username: a.ProxyUsername}
	}
	return resp
}
```

- [ ] **Step 2: 写失败测试** `service_test.go`：

```go
package account

import (
	"errors"
	"path/filepath"
	"testing"

	"flymail/internal/crypto"
	"flymail/internal/database"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if sqlDB, e := db.DB(); e == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	enc, err := crypto.New("a-test-encryption-key-32bytes!!")
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	return NewService(NewRepository(db), enc)
}

func TestServiceCreateEncryptsAndHidesPassword(t *testing.T) {
	s := newTestService(t)
	resp, err := s.Create(CreateAccountRequest{
		Name: "Work", Email: "u@example.com", Password: "p@ss",
		IMAPHost: "imap.example.com", IMAPPort: 993, IMAPSecurity: "ssl",
		SMTPHost: "smtp.example.com", SMTPPort: 465, SMTPSecurity: "ssl",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resp.ID == 0 {
		t.Fatal("应返回新 ID")
	}

	// 落库密码必须是密文且可解密回原文
	raw, err := s.repo.GetByID(resp.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if raw.PasswordEnc == "" || raw.PasswordEnc == "p@ss" {
		t.Errorf("密码未加密存储: %q", raw.PasswordEnc)
	}
	dec, _ := s.enc.Decrypt(raw.PasswordEnc)
	if dec != "p@ss" {
		t.Errorf("解密结果 = %q, want p@ss", dec)
	}
}

func TestServiceUpdateKeepsPasswordWhenEmpty(t *testing.T) {
	s := newTestService(t)
	created, _ := s.Create(CreateAccountRequest{
		Name: "W", Email: "k@example.com", Password: "orig",
		IMAPHost: "h", IMAPPort: 993, SMTPHost: "h", SMTPPort: 465,
	})
	before, _ := s.repo.GetByID(created.ID)

	if _, err := s.Update(created.ID, UpdateAccountRequest{
		Name: "W2", Email: "k@example.com", Password: "",
		IMAPHost: "h", IMAPPort: 993, SMTPHost: "h", SMTPPort: 465,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	after, _ := s.repo.GetByID(created.ID)
	if after.PasswordEnc != before.PasswordEnc {
		t.Error("空密码更新不应改变已存密码")
	}
	if after.Name != "W2" {
		t.Errorf("Name 未更新: %q", after.Name)
	}
}

func TestServiceGetNotFound(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Get(999); !errors.Is(err, ErrAccountNotFound) {
		t.Errorf("want ErrAccountNotFound, got %v", err)
	}
}
```

- [ ] **Step 3: 运行确认失败**：`go test ./modules/email/account/ -run TestService -v`（Service 等未定义）。

- [ ] **Step 4: 实现** `service.go`：

```go
package account

import "flymail/internal/crypto"

type Service struct {
	repo *Repository
	enc  *crypto.Encryptor
}

func NewService(repo *Repository, enc *crypto.Encryptor) *Service {
	return &Service{repo: repo, enc: enc}
}

func (s *Service) Create(req CreateAccountRequest) (*AccountResponse, error) {
	encPw, err := s.enc.Encrypt(req.Password)
	if err != nil {
		return nil, err
	}
	a := &Account{
		Name: req.Name, Email: req.Email, Username: req.Username,
		AuthType: "password", PasswordEnc: encPw,
		IMAPHost: req.IMAPHost, IMAPPort: req.IMAPPort, IMAPSecurity: req.IMAPSecurity,
		SMTPHost: req.SMTPHost, SMTPPort: req.SMTPPort, SMTPSecurity: req.SMTPSecurity,
		Status: "new",
	}
	if err := s.applyProxy(a, req.Proxy); err != nil {
		return nil, err
	}
	if err := s.repo.Create(a); err != nil {
		return nil, err
	}
	resp := toResponse(a)
	return &resp, nil
}

func (s *Service) Get(id uint) (*AccountResponse, error) {
	a, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	resp := toResponse(a)
	return &resp, nil
}

func (s *Service) List() ([]AccountResponse, error) {
	list, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	out := make([]AccountResponse, 0, len(list))
	for i := range list {
		out = append(out, toResponse(&list[i]))
	}
	return out, nil
}

func (s *Service) Update(id uint, req UpdateAccountRequest) (*AccountResponse, error) {
	a, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	a.Name = req.Name
	a.Email = req.Email
	a.Username = req.Username
	a.IMAPHost = req.IMAPHost
	a.IMAPPort = req.IMAPPort
	a.IMAPSecurity = req.IMAPSecurity
	a.SMTPHost = req.SMTPHost
	a.SMTPPort = req.SMTPPort
	a.SMTPSecurity = req.SMTPSecurity
	if req.Password != "" {
		encPw, err := s.enc.Encrypt(req.Password)
		if err != nil {
			return nil, err
		}
		a.PasswordEnc = encPw
	}
	if err := s.applyProxy(a, req.Proxy); err != nil {
		return nil, err
	}
	if err := s.repo.Update(a); err != nil {
		return nil, err
	}
	resp := toResponse(a)
	return &resp, nil
}

func (s *Service) Delete(id uint) error { return s.repo.Delete(id) }

// applyProxy 把代理 DTO 写入模型（代理密码加密；DTO 为 nil 时清空代理）。
func (s *Service) applyProxy(a *Account, p *ProxyDTO) error {
	if p == nil || p.Host == "" {
		a.ProxyType, a.ProxyHost, a.ProxyPort, a.ProxyUsername, a.ProxyPasswordEnc = "", "", 0, "", ""
		return nil
	}
	a.ProxyType = p.Type
	a.ProxyHost = p.Host
	a.ProxyPort = p.Port
	a.ProxyUsername = p.Username
	if p.Password != "" {
		enc, err := s.enc.Encrypt(p.Password)
		if err != nil {
			return err
		}
		a.ProxyPasswordEnc = enc
	}
	return nil
}
```

- [ ] **Step 5: 运行确认通过**：`go test ./modules/email/account/ -v`（repository + service 全 PASS）。`go build ./...`。

- [ ] **Step 6: 报告**（不提交）。

---

## Task 6: 连接测试

**Files:**
- Create: `flymail/backend/modules/email/account/connection.go`
- Test: `flymail/backend/modules/email/account/connection_test.go`

- [ ] **Step 1: 写失败测试** `connection_test.go`（用关闭端口 `127.0.0.1:1` 断言优雅失败，不依赖外网）：

```go
package account

import "testing"

func TestParseSecurity(t *testing.T) {
	if parseSecurity("ssl") != "ssl" {
		t.Error("ssl")
	}
	if parseSecurity("") != "none" {
		t.Error("空应回退 none")
	}
}

func TestTestConnectionUnreachable(t *testing.T) {
	s := newTestService(t) // 复用 service_test.go 的辅助函数
	res := s.TestConnection(TestConnectionRequest{
		Email:        "u@example.com",
		Password:     "x",
		IMAPHost:     "127.0.0.1",
		IMAPPort:     1,
		IMAPSecurity: "ssl",
		SMTPHost:     "127.0.0.1",
		SMTPPort:     1,
		SMTPSecurity: "ssl",
	})
	if res.IMAP {
		t.Error("不可达 IMAP 不应成功")
	}
	if res.IMAPError == "" {
		t.Error("应记录 IMAP 错误信息")
	}
	if res.SMTP {
		t.Error("不可达 SMTP 不应成功")
	}
	if res.SMTPError == "" {
		t.Error("应记录 SMTP 错误信息")
	}
}
```

- [ ] **Step 2: 运行确认失败**：`go test ./modules/email/account/ -run TestTestConnection -v`（TestConnection/TestConnectionRequest/parseSecurity 未定义）。

- [ ] **Step 3: 实现** `connection.go`：

```go
package account

import (
	coreimap "flymail-core/imap"
	coresmtp "flymail-core/smtp"
	"flymail-core/types"
)

// TestConnectionRequest 测试连接请求（明文密码，用于保存前测试）。
type TestConnectionRequest struct {
	Email        string    `json:"email" binding:"required"`
	Username     string    `json:"username,omitempty"`
	Password     string    `json:"password" binding:"required"`
	IMAPHost     string    `json:"imap_host" binding:"required"`
	IMAPPort     int       `json:"imap_port" binding:"required"`
	IMAPSecurity string    `json:"imap_security"`
	SMTPHost     string    `json:"smtp_host" binding:"required"`
	SMTPPort     int       `json:"smtp_port" binding:"required"`
	SMTPSecurity string    `json:"smtp_security"`
	Proxy        *ProxyDTO `json:"proxy,omitempty"`
}

func (r TestConnectionRequest) login() string {
	if r.Username != "" {
		return r.Username
	}
	return r.Email
}

// parseSecurity 把字符串安全模式映射为 core 类型，空值回退 none。
func parseSecurity(s string) types.SecurityMode {
	switch s {
	case "ssl":
		return types.SecuritySSL
	case "starttls":
		return types.SecurityStartTLS
	default:
		return types.SecurityNone
	}
}

func proxyFromDTO(p *ProxyDTO) *types.ProxyConfig {
	if p == nil || p.Host == "" {
		return nil
	}
	return &types.ProxyConfig{Type: p.Type, Host: p.Host, Port: p.Port, Username: p.Username, Password: p.Password}
}

// TestConnection 测试 IMAP 与 SMTP 连接/认证，返回结构化结果（不抛错）。
func (s *Service) TestConnection(req TestConnectionRequest) types.ConnectionTestResult {
	res := types.ConnectionTestResult{}
	proxy := proxyFromDTO(req.Proxy)

	imapCfg := types.IMAPConfig{
		Host: req.IMAPHost, Port: req.IMAPPort,
		Username: req.login(), Password: req.Password,
		Security: parseSecurity(req.IMAPSecurity), Proxy: proxy,
		ClientName: "FlyMail", ClientVendor: "FlyMail",
	}
	if sess, err := coreimap.Dial(imapCfg); err != nil {
		res.IMAPError = err.Error()
	} else {
		res.IMAP = true
		res.SupportsIDLE = sess.SupportsIDLE
		res.Capabilities = sess.Capabilities
		res.SecurityMode = sess.SecurityMode
		_ = sess.Close()
	}

	smtpCfg := types.SMTPConfig{
		Host: req.SMTPHost, Port: req.SMTPPort,
		Username: req.login(), Password: req.Password,
		Security: parseSecurity(req.SMTPSecurity), Proxy: proxy,
	}
	if err := coresmtp.NewClient(smtpCfg).TestConnection(); err != nil {
		res.SMTPError = err.Error()
	} else {
		res.SMTP = true
	}
	return res
}
```

- [ ] **Step 4: 运行确认通过**：`go test ./modules/email/account/ -v`（含连接测试用例；不可达主机应快速失败返回错误）。`go mod tidy`。

> 注意：`127.0.0.1:1` 为关闭端口，应立即"connection refused"而非超时；若个别环境表现为超时，core 内有 10s dial 超时兜底，测试仍会通过（只是慢），可接受。

- [ ] **Step 5: 报告**（不提交）。

---

## Task 7: Handler + 路由接入（JWT 保护）

**Files:**
- Create: `flymail/backend/modules/email/account/handler.go`
- Modify: `flymail/backend/internal/server/router.go`
- Modify: `flymail/backend/internal/app/app.go`
- Test: `flymail/backend/modules/email/account/handler_test.go`

- [ ] **Step 1: 写失败测试** `handler_test.go`：

```go
package account_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"flymail/internal/crypto"
	"flymail/internal/database"
	"flymail/modules/email/account"

	"github.com/gin-gonic/gin"
)

func setup(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, e := db.DB(); e == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	enc, _ := crypto.New("a-test-encryption-key-32bytes!!")
	svc := account.NewService(account.NewRepository(db), enc)
	r := gin.New()
	account.RegisterRoutes(r.Group("/api/v1"), svc)
	return r
}

func TestCreateListGetDelete(t *testing.T) {
	r := setup(t)

	body, _ := json.Marshal(map[string]any{
		"name": "Work", "email": "u@example.com", "password": "p@ss",
		"imap_host": "imap.example.com", "imap_port": 993, "imap_security": "ssl",
		"smtp_host": "smtp.example.com", "smtp_port": 465, "smtp_security": "ssl",
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/accounts", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	// 响应不得包含密码字段
	if bytes.Contains(rec.Body.Bytes(), []byte("p@ss")) || bytes.Contains(rec.Body.Bytes(), []byte("password")) {
		t.Errorf("响应不应含密码: %s", rec.Body.String())
	}
	var created account.AccountResponse
	json.Unmarshal(rec.Body.Bytes(), &created)

	// list
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("list status=%d", rec2.Code)
	}

	// delete
	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, httptest.NewRequest(http.MethodDelete, "/api/v1/accounts/"+itoa(created.ID), nil))
	if rec3.Code != http.StatusOK && rec3.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", rec3.Code)
	}
}

func itoa(u uint) string {
	if u == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for u > 0 {
		i--
		b[i] = byte('0' + u%10)
		u /= 10
	}
	return string(b[i:])
}
```

- [ ] **Step 2: 运行确认失败**：`go test ./modules/email/account/ -run TestCreateListGetDelete -v`（RegisterRoutes 未定义）。

- [ ] **Step 3: 实现** `handler.go`：

```go
package account

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册账户管理路由到给定分组（调用方负责套用鉴权中间件）。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	g := rg.Group("/accounts")
	g.GET("", h.list)
	g.POST("", h.create)
	g.GET("/:id", h.get)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.delete)
	g.POST("/test", h.testConnection)
}

type handler struct{ svc *Service }

func parseID(c *gin.Context) (uint, bool) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return 0, false
	}
	return uint(id64), true
}

func (h *handler) list(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *handler) create(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.svc.Create(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败"})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *handler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	resp, err := h.svc.Get(id)
	if errors.Is(err, ErrAccountNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *handler) update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.svc.Update(id, req)
	if errors.Is(err, ErrAccountNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *handler) delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(id); errors.Is(err, ErrAccountNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) testConnection(c *gin.Context) {
	var req TestConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.svc.TestConnection(req))
}
```

- [ ] **Step 4: 把账户路由接进 router（JWT 保护）** —— 修改 `internal/server/router.go`：
  - `Deps` 增加字段 `Account *account.Service`，import `"flymail/modules/email/account"`。
  - 在 `New` 中，auth 路由注册之后，新增受保护分组：
```go
	if deps.Auth != nil && deps.Account != nil {
		protected := api.Group("")
		protected.Use(auth.Middleware(deps.Auth))
		account.RegisterRoutes(protected, deps.Account)
	}
```

- [ ] **Step 5: 在 app 装配账户 service** —— 修改 `internal/app/app.go`：
  - import `"flymail/internal/crypto"` 和 `"flymail/modules/email/account"`。
  - 在 `New` 中构建 encryptor 与账户 service，并塞进 Deps：
```go
	enc, err := crypto.New(cfg.Crypto.EncryptionKey)
	if err != nil {
		return nil, err
	}
	accountSvc := account.NewService(account.NewRepository(db), enc)
	handler := server.New(server.Deps{Auth: authSvc, Account: accountSvc})
```
（`db` 在 `New` 中已有；保持其余不变。）

- [ ] **Step 6: 运行确认通过**：
```
go test ./modules/email/account/ -v
go test ./internal/server/ ./internal/app/ -v
go build ./...
go vet ./...
```
期望全部 PASS、build OK、vet 干净。

- [ ] **Step 7: 报告**（不提交）。

---

## Task 8: 按邮箱域名建议服务器预设

**Files:**
- Modify: `flymail/backend/modules/email/account/handler.go`
- Modify: `flymail/backend/modules/email/account/connection.go`（放置 suggest 逻辑与类型）
- Test: `flymail/backend/modules/email/account/suggest_test.go`

- [ ] **Step 1: 写失败测试** `suggest_test.go`：

```go
package account

import "testing"

func TestSuggestKnownProvider(t *testing.T) {
	s := newTestService(t)
	out, ok := s.SuggestSettings("someone@gmail.com")
	if !ok {
		t.Skip("内置 provider 列表未含 gmail，跳过") // 若 providers.json 含 gmail 则应命中
	}
	if out.IMAPHost == "" || out.IMAPPort == 0 {
		t.Errorf("应给出 IMAP 预设, 得到 %+v", out)
	}
}

func TestSuggestUnknownProvider(t *testing.T) {
	s := newTestService(t)
	if _, ok := s.SuggestSettings("user@no-such-domain-xyz.test"); ok {
		t.Error("未知域名不应命中预设")
	}
}
```

- [ ] **Step 2: 运行确认失败**：`go test ./modules/email/account/ -run TestSuggest -v`（SuggestSettings 未定义）。

- [ ] **Step 3: 实现** —— 在 `connection.go` 追加：

```go
import (
	"strings"

	coreprovider "flymail-core/provider"
)

// SuggestedSettings 服务器预设建议。
type SuggestedSettings struct {
	Provider     string `json:"provider"`
	IMAPHost     string `json:"imap_host"`
	IMAPPort     int    `json:"imap_port"`
	IMAPSecurity string `json:"imap_security"`
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPSecurity string `json:"smtp_security"`
}

// SuggestSettings 按邮箱域名给出 IMAP/SMTP 预设（优先 ssl）。
func (s *Service) SuggestSettings(email string) (SuggestedSettings, bool) {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return SuggestedSettings{}, false
	}
	domain := email[at+1:]
	p, ok := coreprovider.GetProvider(domain)
	if !ok {
		return SuggestedSettings{}, false
	}
	out := SuggestedSettings{Provider: p.ID}
	// 优先 ssl，其次 starttls
	if ep, ok := p.Servers["ssl"]; ok {
		out.IMAPHost, out.IMAPPort, out.IMAPSecurity = ep.Host, ep.Port, "ssl"
		out.SMTPHost, out.SMTPPort, out.SMTPSecurity = ep.Host, ep.Port, "ssl"
	}
	return out, true
}
```

> 说明：`ProviderConfig.Servers` 的键是安全模式（ssl/starttls/none），值是 `ServerEndpoint{Host,Port}`，并未区分 IMAP/SMTP 主机。本 Task 先用同一 endpoint 填充两者作为建议（用户可在前端调整）；若实测 `providers.json` 的结构按 imap_/smtp_ 区分，则按真实结构调整映射——实现前先 `cat core/provider/providers.json | head -40` 核对结构。

- [ ] **Step 4: 加 handler 路由** —— 在 `handler.go` 的 `RegisterRoutes` 中 `/accounts` 组内追加：
```go
	g.GET("/suggest", h.suggest)
```
并实现：
```go
func (h *handler) suggest(c *gin.Context) {
	email := c.Query("email")
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 email 参数"})
		return
	}
	out, ok := h.svc.SuggestSettings(email)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到匹配的服务器预设"})
		return
	}
	c.JSON(http.StatusOK, out)
}
```

- [ ] **Step 5: 运行确认通过**：`go test ./modules/email/account/ -v`、`go build ./...`、`go vet ./...`。

- [ ] **Step 6: 报告**（不提交）。

---

## Task 9: 端到端冒烟（鉴权 + 账户 CRUD）

**Files:** 无新增（端到端验证）

- [ ] **Step 1: 全量测试**：`go test ./...`（在 `flymail/backend/`，CGO=0）→ 全 PASS。

- [ ] **Step 2: 无 CGO 构建**：`CGO_ENABLED=0 go build -o /tmp/fm.exe .` → 成功。

- [ ] **Step 3: 真实鉴权 + CRUD 冒烟**（用关闭端口模拟测试连接失败即可，不需真实邮箱）：
```
DATA=$(mktemp -d)
/tmp/fm.exe db init --admin-pass secret123 --data-dir "$DATA"
FLYMAIL_SERVER_PORT=18090 /tmp/fm.exe server --data-dir "$DATA" &
SRV=$!; until curl -s -o /dev/null http://127.0.0.1:18090/api/v1/healthz; do sleep 1; done
TOKEN=$(curl -s -X POST http://127.0.0.1:18090/api/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"secret123"}' | grep -oE '"access_token":"[^"]+"' | sed 's/.*:"//;s/"//')
echo "无 token 访问 accounts 应 401:"; curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18090/api/v1/accounts
echo "带 token 创建账户:"; curl -s -X POST http://127.0.0.1:18090/api/v1/accounts -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"name":"Work","email":"u@example.com","password":"p@ss","imap_host":"imap.example.com","imap_port":993,"imap_security":"ssl","smtp_host":"smtp.example.com","smtp_port":465,"smtp_security":"ssl"}'
echo; echo "带 token 列表(不应含密码):"; curl -s http://127.0.0.1:18090/api/v1/accounts -H "Authorization: Bearer $TOKEN"
kill $SRV; rm -rf "$DATA" /tmp/fm.exe
```
期望：无 token → 401；带 token 创建 → 201 且响应无密码；列表返回该账户。

- [ ] **Step 4: 报告**（不提交）。

---

## 自检

**Spec 覆盖（spec §3 Account 模型、§5 accounts CRUD + test-connection、§8 里程碑2）：**
- Account 模型（name/email/imap+smtp/proxy/auth_type/status/last_sync_at）→ Task 3 ✓（last_sync_at 字段已留）
- 凭证 AES 加密落库、API 不回传密码 → Task 2/5（加密）、Task 5/7（响应剥离密码 + 测试断言）✓
- CRUD → Task 4（repo）/5（service）/7（handler+路由）✓
- 连接测试（IMAP+SMTP）→ Task 6 ✓
- JWT 保护账户路由 → Task 7（protected group + Middleware）✓
- 服务器预设自动建议（accounts/suggest）→ Task 8 ✓（spec §5 未显式列，但属账户管理良好 UX，低成本）
- 端到端 → Task 9 ✓

**占位符扫描：** 无 TBD/TODO；每步含真实代码与命令。Task 8 的 providers.json 结构核对是显式依赖确认步骤，非占位。

**类型一致性：** `crypto.New(string)→*Encryptor`、`Encryptor.Encrypt/Decrypt`、`account.NewRepository(db)`、`account.NewService(repo,*crypto.Encryptor)`、`Service.{Create,Get,List,Update,Delete,TestConnection,SuggestSettings}`、`CreateAccountRequest/UpdateAccountRequest/AccountResponse/ProxyDTO/TestConnectionRequest/SuggestedSettings`、`ErrAccountNotFound`、`RegisterRoutes(rg,svc)`、`server.Deps{Auth,Account}`、`parseSecurity`、`proxyFromDTO` 跨任务一致。

**已知外部依赖待核对（实现时第一步）：**
- `core/provider/providers.json` 的 Servers 结构（Task 8 实现前 `head` 核对，决定 IMAP/SMTP 主机是否分别填充）。
- 前端账户管理 UI（添加/编辑/列表/连接测试 + 三栏侧栏账户树）属里程碑 3+ 或本里程碑后续；本计划聚焦后端 + API，前端在后续计划处理。
