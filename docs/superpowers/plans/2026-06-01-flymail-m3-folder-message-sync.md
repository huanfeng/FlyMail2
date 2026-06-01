# FlyMail M3：文件夹同步 + 首次元数据同步 → 邮件列表 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现"账户 → 列出全部文件夹（分类落库）→ 后台异步首同步 INBOX 最近 ~1000 封元数据 → 三栏前端展示邮件列表"的端到端闭环。

**Architecture:** 后端新增 `folder` / `message` / `sync` 三个模块（沿用 `account` 的 model/repository/service/handler/dto 分层）。`folder`、`message` 服务通过最小 IMAP 能力接口（便于测试 mock）操作 `core/imap` 会话；`sync` 服务在后台 goroutine 中按账户串行编排（IMAP Session 非并发安全），用内存维护同步进度供前端轮询。前端新增 React 三栏布局（AccountSidebar / MailList / Reader 占位），TanStack Query 拉取数据，URL 驱动当前账户/文件夹选择，新增 i18n 基础设施。

**Tech Stack:** Go 1.26 / gin / gorm + glebarez(纯 Go SQLite) / `flymail-core`(imap/types/parser) ；React 19 + Vite 8 + TS + Tailwind 4 + radix-ui + TanStack Query 5 + react-i18next。

---

## 背景与关键设计决策（来自 spec + 跨来源踩坑）

> 详见 `docs/superpowers/specs/2026-05-31-flymail-design.md` §3/§4，及记忆 `flymail-imap-pitfalls`。

**范围（已与用户确认）：**
- 首同步深度：每文件夹**最近 ~1000 封**，用 `from = max(1, UIDNEXT - 1000)` 近似。
- 执行模型：**后台异步 + 进度查询**（`POST /sync` 立即返回，`GET /sync/status` 轮询）。
- 文件夹：**全部列出并分类落库**；首同步**仅 INBOX** 抓元数据（其它文件夹留后续）。
- 前端：**最小可用三栏**（账户树 / 文件夹 / 邮件列表行）；M3 **不做邮件详情**（Reader 为欢迎占位）。

**core 已解决、本计划直接复用（不重复实现）：**
- `coreimap.Dial(types.IMAPConfig)` 已对网易邮箱发 IMAP ID；`Session.ListFolders()` 已 UTF-7 解码（`FolderInfo.Name` 解码名 / `.Path` 原始路径）；`fetch.go` 已对 ENVELOPE 主题/地址名做 `DecodeMIMEHeader`；`types.ClassifyFolder` 已做三层文件夹类型判定。

**本计划必须处理的坑（旧 flymail 原型实证）：**
- **UIDVALIDITY 兜底**：`SelectFolder` 返回的 `UIDValidity==0` 时，用 `FolderStatus(path, coreimap.StatusUIDValidity)` 再取一次（部分服务商 SELECT 不返回 UIDVALIDITY）。
- **分批 FETCH**：首同步按 **批大小 200** 切分 UID 区间逐批抓取（旧原型注释：一次抓 500 在慢连接会超时，降到 100），既稳又能报进度。
- **去重**：`(folder_id, uid)` 唯一索引 + upsert；`MessageID` 仅索引不唯一。
- **正文非标编码（euc-cn 等）**：M3 不抓正文，留 M4 验证。

**已知 core 限制 → M3 接受的取舍：**
- `types.ParsedEmail` 不暴露 In-Reply-To / References → Message 这两列保留为预留空列（spec 要求 schema 预留 thread 字段）。
- 首同步 `FetchBody:false` → `HasAttachment`/`Snippet`/`BodySynced` 保持默认（false/空），M4 抓正文时回填。

---

## 文件结构地图

**后端（module `flymail`，根目录 `flymail/backend/`）：**
- 改 `modules/email/account/credentials.go`（新建）：`Service.IMAPConfig(id)` + `Service.TouchLastSync(id,t)`。
- 新 `modules/email/folder/{model,repository,service,handler,dto}.go` + 测试。
- 新 `modules/email/message/{model,repository,service,handler,dto}.go` + 测试。
- 新 `modules/email/sync/{service,handler}.go` + 测试。
- 改 `internal/database/database.go`：`Migrate` 追加 `folder.Folder`、`message.Message`。
- 改 `internal/server/router.go`：`Deps` 追加 `Folder/Message/Sync`，挂受保护路由。
- 改 `internal/app/app.go`：装配新 service。

**前端（`flymail/frontend/`）：**
- 新 `src/lib/i18n.ts` + `src/locales/{zh,en}.json`；改 `src/main.tsx` 引入。
- 改 `src/index.css`：补主题 CSS 变量（布局/配色 token，中文友好字体）。
- 新 `src/lib/types.ts`（API 类型）、`src/lib/queries.ts`（TanStack Query hooks）。
- 新 `src/components/mail/{AppLayout,AccountSidebar,MailList,Reader}.tsx`。
- 改 `src/pages/Shell.tsx`：渲染三栏布局；`src/router.tsx`：URL 驱动选择（query 参数 `account`/`folder`）。

---

# 后端

## Task 1：account 暴露 IMAP 配置与 LastSync 更新

`sync` 需要拿到账户解密后的 IMAP 配置，并在同步完成后更新 `last_sync_at`。

**Files:**
- Create: `flymail/backend/modules/email/account/credentials.go`
- Create: `flymail/backend/modules/email/account/credentials_test.go`

- [ ] **Step 1: 写失败测试**

`flymail/backend/modules/email/account/credentials_test.go`:
```go
package account_test

import (
	"path/filepath"
	"testing"

	"flymail/internal/crypto"
	"flymail/internal/database"
	"flymail/modules/email/account"

	"flymail-core/types"
)

func newAccountService(t *testing.T) *account.Service {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	enc, err := crypto.New("test-key-test-key-test-key-32byt")
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	return account.NewService(account.NewRepository(db), enc)
}

func TestIMAPConfigDecryptsPassword(t *testing.T) {
	svc := newAccountService(t)
	created, err := svc.Create(account.CreateAccountRequest{
		Name: "T", Email: "u@example.com", Password: "secret-pw",
		IMAPHost: "imap.example.com", IMAPPort: 993, IMAPSecurity: "ssl",
		SMTPHost: "smtp.example.com", SMTPPort: 465, SMTPSecurity: "ssl",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	cfg, err := svc.IMAPConfig(created.ID)
	if err != nil {
		t.Fatalf("IMAPConfig: %v", err)
	}
	if cfg.Password != "secret-pw" {
		t.Errorf("password not decrypted: got %q", cfg.Password)
	}
	if cfg.Username != "u@example.com" {
		t.Errorf("username fallback to email expected, got %q", cfg.Username)
	}
	if cfg.Security != types.SecuritySSL {
		t.Errorf("security = %v, want ssl", cfg.Security)
	}
}

func TestTouchLastSyncSetsTimestamp(t *testing.T) {
	svc := newAccountService(t)
	created, err := svc.Create(account.CreateAccountRequest{
		Name: "T", Email: "u2@example.com", Password: "p",
		IMAPHost: "h", IMAPPort: 993, SMTPHost: "h", SMTPPort: 465,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.TouchLastSync(created.ID, timeNow()); err != nil {
		t.Fatalf("touch: %v", err)
	}
	got, _ := svc.Get(created.ID)
	if got.LastSyncAt == nil {
		t.Errorf("LastSyncAt should be set")
	}
}
```

> 注：若 `AccountResponse` 当前不含 `LastSyncAt` 字段，本测试的 `got.LastSyncAt` 断言改为通过 `svc.IMAPConfig` 不报错来间接验证；并在 dto 的 `toResponse` 中补 `LastSyncAt` 字段（见 Step 3 附注）。`timeNow()` 用下方 helper。

在测试文件末尾追加：
```go
import "time"

func timeNow() time.Time { return time.Now() }
```
（若 import 块已存在，合并 `time` 即可。）

- [ ] **Step 2: 运行验证失败**

Run: `cd flymail/backend && go test ./modules/email/account/ -run TestIMAPConfig -v`
Expected: 编译失败（`svc.IMAPConfig` undefined）。

- [ ] **Step 3: 实现**

`flymail/backend/modules/email/account/credentials.go`:
```go
package account

import (
	"time"

	"flymail-core/types"
)

// IMAPConfig 取出账户并解密凭证，构建 core 的 IMAP 配置（供同步引擎使用）。
func (s *Service) IMAPConfig(id uint) (types.IMAPConfig, error) {
	a, err := s.repo.GetByID(id)
	if err != nil {
		return types.IMAPConfig{}, err
	}
	pw, err := s.enc.Decrypt(a.PasswordEnc)
	if err != nil {
		return types.IMAPConfig{}, err
	}
	var proxy *types.ProxyConfig
	if a.ProxyHost != "" {
		ppw, err := s.enc.Decrypt(a.ProxyPasswordEnc)
		if err != nil {
			return types.IMAPConfig{}, err
		}
		proxy = &types.ProxyConfig{
			Type: a.ProxyType, Host: a.ProxyHost, Port: a.ProxyPort,
			Username: a.ProxyUsername, Password: ppw,
		}
	}
	login := a.Username
	if login == "" {
		login = a.Email
	}
	return types.IMAPConfig{
		Host:         a.IMAPHost,
		Port:         a.IMAPPort,
		Username:     login,
		Password:     pw,
		Security:     parseSecurity(a.IMAPSecurity),
		Proxy:        proxy,
		ClientName:   "FlyMail",
		ClientVendor: "FlyMail",
	}, nil
}

// TouchLastSync 更新账户的最后同步时间。
func (s *Service) TouchLastSync(id uint, t time.Time) error {
	a, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	a.LastSyncAt = &t
	return s.repo.Update(a)
}
```

附注：确认 `dto.go` 的 `AccountResponse` 含 `LastSyncAt *time.Time` 且 `toResponse` 赋值；若缺则补上（字段 `json:"last_sync_at"`）。

- [ ] **Step 4: 运行验证通过**

Run: `cd flymail/backend && go test ./modules/email/account/ -v`
Expected: PASS。

- [ ] **Step 5: gofmt + 提交**

```bash
cd flymail/backend && gofmt -w modules/email/account/credentials.go modules/email/account/credentials_test.go
git add flymail/backend/modules/email/account/credentials.go flymail/backend/modules/email/account/credentials_test.go flymail/backend/modules/email/account/dto.go
git commit -m "feat(flymail): account 暴露 IMAPConfig + TouchLastSync（供同步引擎）"
```

---

## Task 2：Folder 模型 + 迁移

**Files:**
- Create: `flymail/backend/modules/email/folder/model.go`
- Modify: `flymail/backend/internal/database/database.go`

- [ ] **Step 1: 写模型**

`flymail/backend/modules/email/folder/model.go`:
```go
package folder

import "time"

// Folder 是账户下的一个 IMAP 文件夹（邮箱）。
// DisplayName 为 UTF-7 解码后的展示名；Path 为原始 IMAP 路径，所有 IMAP 操作必须用 Path。
type Folder struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	AccountID   uint   `gorm:"index;uniqueIndex:idx_folder_account_path;not null" json:"account_id"`
	Path        string `gorm:"uniqueIndex:idx_folder_account_path;not null" json:"path"`
	DisplayName string `gorm:"not null" json:"display_name"`
	Delimiter   string `json:"delimiter"`
	Type        string `gorm:"not null;default:custom" json:"type"` // inbox/sent/drafts/trash/junk/archive/custom/unknown
	Attributes  string `json:"attributes"`                          // 逗号分隔的 IMAP 属性
	Selectable  bool   `gorm:"not null;default:true" json:"selectable"`

	// 同步锚点（旧 flymail 实证必要字段）
	UIDValidity   uint32     `json:"uid_validity"`
	UIDNext       uint32     `json:"uid_next"`
	LastSyncedUID uint32     `json:"last_synced_uid"` // 预留，M6 增量同步用
	LastSyncedAt  *time.Time `json:"last_synced_at"`

	// 缓存计数 + 排序
	TotalCount  int `gorm:"not null;default:0" json:"total_count"`
	UnreadCount int `gorm:"not null;default:0" json:"unread_count"`
	SortOrder   int `gorm:"not null;default:100" json:"sort_order"` // 系统 1-99 按类型；自定义 100+

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Folder) TableName() string { return "folders" }

// SortOrderForType 给系统文件夹固定排序，自定义返回 100（由调用方按名称微调）。
func SortOrderForType(folderType string) int {
	switch folderType {
	case "inbox":
		return 1
	case "sent":
		return 10
	case "drafts":
		return 11
	case "trash":
		return 12
	case "junk":
		return 13
	case "archive":
		return 14
	default:
		return 100
	}
}
```

- [ ] **Step 2: 追加迁移**

`flymail/backend/internal/database/database.go` 的 `Migrate`：
```go
import (
	"flymail/modules/auth"
	"flymail/modules/email/account"
	"flymail/modules/email/folder"

	coredb "flymail-core/database"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&auth.AdminUser{},
		&account.Account{},
		&folder.Folder{},
	)
}
```

- [ ] **Step 3: 验证编译 + 迁移建表**

Run: `cd flymail/backend && go build ./... && go vet ./modules/email/folder/`
Expected: 无错误。

- [ ] **Step 4: gofmt + 提交**

```bash
cd flymail/backend && gofmt -w modules/email/folder/model.go internal/database/database.go
git add flymail/backend/modules/email/folder/model.go flymail/backend/internal/database/database.go
git commit -m "feat(flymail): Folder 模型 + AutoMigrate"
```

---

## Task 3：Folder 仓储

**Files:**
- Create: `flymail/backend/modules/email/folder/repository.go`
- Create: `flymail/backend/modules/email/folder/repository_test.go`

- [ ] **Step 1: 写失败测试**

`flymail/backend/modules/email/folder/repository_test.go`:
```go
package folder_test

import (
	"path/filepath"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/modules/email/folder"

	"gorm.io/gorm"
)

func newRepo(t *testing.T) (*folder.Repository, *gorm.DB) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return folder.NewRepository(db), db
}

func TestUpsertByPathInsertThenUpdate(t *testing.T) {
	repo, _ := newRepo(t)
	f := &folder.Folder{AccountID: 1, Path: "INBOX", DisplayName: "收件箱", Type: "inbox", SortOrder: 1}
	if err := repo.UpsertByPath(f); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if f.ID == 0 {
		t.Fatal("ID should be set after insert")
	}
	// 再次 upsert 同 path：应更新而非新增
	f2 := &folder.Folder{AccountID: 1, Path: "INBOX", DisplayName: "Inbox", Type: "inbox", SortOrder: 1}
	if err := repo.UpsertByPath(f2); err != nil {
		t.Fatalf("update: %v", err)
	}
	list, _ := repo.ListByAccount(1)
	if len(list) != 1 {
		t.Fatalf("want 1 folder, got %d", len(list))
	}
	if list[0].DisplayName != "Inbox" {
		t.Errorf("display name not updated: %q", list[0].DisplayName)
	}
}

func TestUpsertPreservesSyncAnchors(t *testing.T) {
	repo, _ := newRepo(t)
	now := time.Now()
	f := &folder.Folder{AccountID: 1, Path: "INBOX", DisplayName: "X", Type: "inbox", UIDValidity: 42, UIDNext: 100, LastSyncedAt: &now}
	if err := repo.UpsertByPath(f); err != nil {
		t.Fatal(err)
	}
	// 模拟一次 LIST：不带 UID 信息
	f2 := &folder.Folder{AccountID: 1, Path: "INBOX", DisplayName: "X", Type: "inbox"}
	if err := repo.UpsertByPath(f2); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetByPath(1, "INBOX")
	if got.UIDValidity != 42 || got.UIDNext != 100 {
		t.Errorf("sync anchors not preserved: validity=%d next=%d", got.UIDValidity, got.UIDNext)
	}
}

func TestFindInboxByType(t *testing.T) {
	repo, _ := newRepo(t)
	_ = repo.UpsertByPath(&folder.Folder{AccountID: 1, Path: "Sent", DisplayName: "Sent", Type: "sent"})
	_ = repo.UpsertByPath(&folder.Folder{AccountID: 1, Path: "INBOX", DisplayName: "Inbox", Type: "inbox"})
	inbox, err := repo.FindInbox(1)
	if err != nil {
		t.Fatalf("find inbox: %v", err)
	}
	if inbox == nil || inbox.Path != "INBOX" {
		t.Errorf("inbox not found correctly: %+v", inbox)
	}
}
```

- [ ] **Step 2: 运行验证失败**

Run: `cd flymail/backend && go test ./modules/email/folder/ -v`
Expected: 编译失败（`folder.NewRepository` undefined）。

- [ ] **Step 3: 实现**

`flymail/backend/modules/email/folder/repository.go`:
```go
package folder

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrFolderNotFound = errors.New("folder not found")

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// UpsertByPath 按 (account_id, path) 唯一键插入或更新；
// 不覆盖已有的同步锚点（UIDValidity/UIDNext/LastSyncedUID/LastSyncedAt/计数）——
// 这些由同步流程单独更新（UpdateSyncState），避免一次 LIST 把它们清零。
func (r *Repository) UpsertByPath(f *Folder) error {
	var existing Folder
	err := r.db.Where("account_id = ? AND path = ?", f.AccountID, f.Path).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(f).Error
	}
	if err != nil {
		return err
	}
	f.ID = existing.ID
	f.CreatedAt = existing.CreatedAt
	if f.UIDValidity == 0 {
		f.UIDValidity = existing.UIDValidity
	}
	if f.UIDNext == 0 {
		f.UIDNext = existing.UIDNext
	}
	if f.LastSyncedUID == 0 {
		f.LastSyncedUID = existing.LastSyncedUID
	}
	if f.LastSyncedAt == nil {
		f.LastSyncedAt = existing.LastSyncedAt
	}
	if f.TotalCount == 0 {
		f.TotalCount = existing.TotalCount
	}
	if f.UnreadCount == 0 {
		f.UnreadCount = existing.UnreadCount
	}
	return r.db.Save(f).Error
}

func (r *Repository) ListByAccount(accountID uint) ([]Folder, error) {
	var list []Folder
	err := r.db.Where("account_id = ?", accountID).
		Order("sort_order, display_name").Find(&list).Error
	return list, err
}

func (r *Repository) GetByID(id uint) (*Folder, error) {
	var f Folder
	err := r.db.First(&f, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrFolderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *Repository) GetByPath(accountID uint, path string) (*Folder, error) {
	var f Folder
	err := r.db.Where("account_id = ? AND path = ?", accountID, path).First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrFolderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// FindInbox 返回账户的收件箱（type=inbox），无则返回 (nil, nil)。
func (r *Repository) FindInbox(accountID uint) (*Folder, error) {
	var f Folder
	err := r.db.Where("account_id = ? AND type = ?", accountID, "inbox").First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// UpdateSyncState 更新文件夹的同步锚点与计数。
func (r *Repository) UpdateSyncState(id uint, uidValidity, uidNext uint32, total, unread int, syncedAt time.Time) error {
	return r.db.Model(&Folder{}).Where("id = ?", id).Updates(map[string]any{
		"uid_validity":   uidValidity,
		"uid_next":       uidNext,
		"total_count":    total,
		"unread_count":   unread,
		"last_synced_at": syncedAt,
	}).Error
}
```

- [ ] **Step 4: 运行验证通过**

Run: `cd flymail/backend && go test ./modules/email/folder/ -v`
Expected: PASS。

- [ ] **Step 5: gofmt + 提交**

```bash
cd flymail/backend && gofmt -w modules/email/folder/repository.go modules/email/folder/repository_test.go
git add flymail/backend/modules/email/folder/repository.go flymail/backend/modules/email/folder/repository_test.go
git commit -m "feat(flymail): Folder 仓储（upsert 保留同步锚点 + FindInbox）"
```

---

## Task 4：Folder 服务（SyncFolders + List）

**Files:**
- Create: `flymail/backend/modules/email/folder/service.go`
- Create: `flymail/backend/modules/email/folder/service_test.go`

- [ ] **Step 1: 写失败测试**

`flymail/backend/modules/email/folder/service_test.go`:
```go
package folder_test

import (
	"path/filepath"
	"testing"

	"flymail/internal/database"
	"flymail/modules/email/folder"

	"flymail-core/types"
)

// fakeLister 实现 folder.IMAPLister。
type fakeLister struct{ infos []types.FolderInfo }

func (f fakeLister) ListFolders() ([]types.FolderInfo, error) { return f.infos, nil }

func newService(t *testing.T) *folder.Service {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return folder.NewService(folder.NewRepository(db))
}

func TestSyncFoldersClassifiesAndStores(t *testing.T) {
	svc := newService(t)
	lister := fakeLister{infos: []types.FolderInfo{
		{Name: "收件箱", Path: "INBOX", Delimiter: "/", Attributes: []string{"\\Inbox"}},
		{Name: "已发送", Path: "Sent", Delimiter: "/", Attributes: []string{"\\Sent"}},
		{Name: "Containers", Path: "Containers", Attributes: []string{"\\Noselect"}},
	}}
	if err := svc.SyncFolders(1, lister); err != nil {
		t.Fatalf("sync: %v", err)
	}
	list, _ := svc.List(1)
	if len(list) != 3 {
		t.Fatalf("want 3 folders, got %d", len(list))
	}
	byPath := map[string]folder.Folder{}
	for _, f := range list {
		byPath[f.Path] = f
	}
	if byPath["INBOX"].Type != "inbox" {
		t.Errorf("INBOX type = %q, want inbox", byPath["INBOX"].Type)
	}
	if byPath["INBOX"].SortOrder != 1 {
		t.Errorf("INBOX sort = %d, want 1", byPath["INBOX"].SortOrder)
	}
	if byPath["Containers"].Selectable {
		t.Errorf("\\Noselect folder should not be selectable")
	}
	if byPath["INBOX"].DisplayName != "收件箱" {
		t.Errorf("display name = %q, want 收件箱", byPath["INBOX"].DisplayName)
	}
}
```

- [ ] **Step 2: 运行验证失败**

Run: `cd flymail/backend && go test ./modules/email/folder/ -run TestSyncFolders -v`
Expected: 编译失败（`folder.NewService` / `folder.IMAPLister` undefined）。

- [ ] **Step 3: 实现**

`flymail/backend/modules/email/folder/service.go`:
```go
package folder

import (
	"strings"
	"time"

	"flymail-core/types"
)

// IMAPLister 是文件夹同步所需的最小 IMAP 能力（便于测试 mock）。
// *coreimap.Session 满足此接口。
type IMAPLister interface {
	ListFolders() ([]types.FolderInfo, error)
}

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

// SyncFolders 列出账户全部文件夹，分类后落库（保留已有同步锚点）。
func (s *Service) SyncFolders(accountID uint, lister IMAPLister) error {
	infos, err := lister.ListFolders()
	if err != nil {
		return err
	}
	for _, info := range infos {
		ft := types.ClassifyFolder(info.Name, info.Path, info.Attributes).String()
		selectable := true
		for _, a := range info.Attributes {
			if strings.EqualFold(a, "\\Noselect") {
				selectable = false
				break
			}
		}
		f := &Folder{
			AccountID:   accountID,
			Path:        info.Path,
			DisplayName: info.Name,
			Delimiter:   info.Delimiter,
			Type:        ft,
			Attributes:  strings.Join(info.Attributes, ","),
			Selectable:  selectable,
			SortOrder:   SortOrderForType(ft),
		}
		if err := s.repo.UpsertByPath(f); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) List(accountID uint) ([]Folder, error) { return s.repo.ListByAccount(accountID) }

func (s *Service) FindInbox(accountID uint) (*Folder, error) { return s.repo.FindInbox(accountID) }

func (s *Service) GetByID(id uint) (*Folder, error) { return s.repo.GetByID(id) }

// UpdateSyncState 透传到仓储（供 sync 编排在同步完单文件夹后写回锚点）。
func (s *Service) UpdateSyncState(id uint, uidValidity, uidNext uint32, total, unread int, syncedAt time.Time) error {
	return s.repo.UpdateSyncState(id, uidValidity, uidNext, total, unread, syncedAt)
}
```

- [ ] **Step 4: 运行验证通过**

Run: `cd flymail/backend && go test ./modules/email/folder/ -v`
Expected: PASS。

- [ ] **Step 5: gofmt + 提交**

```bash
cd flymail/backend && gofmt -w modules/email/folder/service.go modules/email/folder/service_test.go
git add flymail/backend/modules/email/folder/service.go flymail/backend/modules/email/folder/service_test.go
git commit -m "feat(flymail): Folder 服务（SyncFolders 分类落库 + 跳过 Noselect）"
```

---

## Task 5：Folder 路由 + DTO

**Files:**
- Create: `flymail/backend/modules/email/folder/dto.go`
- Create: `flymail/backend/modules/email/folder/handler.go`

- [ ] **Step 1: 实现 DTO**

`flymail/backend/modules/email/folder/dto.go`:
```go
package folder

// FolderResponse 是文件夹的对外表示。
type FolderResponse struct {
	ID          uint   `json:"id"`
	AccountID   uint   `json:"account_id"`
	Path        string `json:"path"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	Selectable  bool   `json:"selectable"`
	TotalCount  int    `json:"total_count"`
	UnreadCount int    `json:"unread_count"`
	SortOrder   int    `json:"sort_order"`
}

func toResponse(f *Folder) FolderResponse {
	return FolderResponse{
		ID: f.ID, AccountID: f.AccountID, Path: f.Path, DisplayName: f.DisplayName,
		Type: f.Type, Selectable: f.Selectable, TotalCount: f.TotalCount,
		UnreadCount: f.UnreadCount, SortOrder: f.SortOrder,
	}
}
```

- [ ] **Step 2: 实现 Handler**

`flymail/backend/modules/email/folder/handler.go`:
```go
package folder

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 在受保护组下挂载文件夹路由：GET /accounts/:id/folders。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET("/accounts/:id/folders", h.list)
}

type handler struct{ svc *Service }

func (h *handler) list(c *gin.Context) {
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}
	folders, err := h.svc.List(uint(accountID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]FolderResponse, 0, len(folders))
	for i := range folders {
		out = append(out, toResponse(&folders[i]))
	}
	c.JSON(http.StatusOK, gin.H{"folders": out})
}
```

- [ ] **Step 3: 验证编译**

Run: `cd flymail/backend && go build ./modules/email/folder/`
Expected: 无错误。

- [ ] **Step 4: gofmt + 提交**

```bash
cd flymail/backend && gofmt -w modules/email/folder/dto.go modules/email/folder/handler.go
git add flymail/backend/modules/email/folder/dto.go flymail/backend/modules/email/folder/handler.go
git commit -m "feat(flymail): Folder 路由 + DTO（GET /accounts/:id/folders）"
```

---

## Task 6：Message 模型 + 迁移

**Files:**
- Create: `flymail/backend/modules/email/message/model.go`
- Modify: `flymail/backend/internal/database/database.go`

- [ ] **Step 1: 写模型**

`flymail/backend/modules/email/message/model.go`:
```go
package message

import "time"

// Message 是一封邮件的元数据（M3 不含正文；正文在 M4 按需抓取另存）。
type Message struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	AccountID uint   `gorm:"index;not null" json:"account_id"`
	FolderID  uint   `gorm:"uniqueIndex:idx_msg_folder_uid;not null" json:"folder_id"`
	UID       uint32 `gorm:"uniqueIndex:idx_msg_folder_uid;not null" json:"uid"`

	MessageID  string `gorm:"index" json:"message_id"`
	InReplyTo  string `json:"in_reply_to"` // 预留：core 暂未暴露 ENVELOPE 的 in-reply-to
	References string `json:"references"`  // 预留
	ThreadID   string `gorm:"index" json:"thread_id"` // 预留（MVP 平铺，不聚合）

	Subject  string `json:"subject"`
	FromName string `json:"from_name"`
	FromAddr string `json:"from_addr"`
	ToJSON   string `json:"-"` // JSON 序列化的收件人地址数组
	CcJSON   string `json:"-"`

	Date time.Time `gorm:"index" json:"date"`
	Size int64     `json:"size"`

	Seen     bool `gorm:"not null;default:false" json:"seen"`
	Flagged  bool `gorm:"not null;default:false" json:"flagged"`
	Answered bool `gorm:"not null;default:false" json:"answered"`
	Deleted  bool `gorm:"not null;default:false" json:"deleted"`

	HasAttachment bool   `gorm:"not null;default:false" json:"has_attachment"` // M3 默认 false，M4 回填
	Snippet       string `json:"snippet"`                                      // M3 留空，M4 回填
	BodySynced    bool   `gorm:"not null;default:false" json:"body_synced"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Message) TableName() string { return "messages" }
```

- [ ] **Step 2: 追加迁移**

`flymail/backend/internal/database/database.go` 的 `Migrate` 追加 `&message.Message{}`（并 import `"flymail/modules/email/message"`）：
```go
	return db.AutoMigrate(
		&auth.AdminUser{},
		&account.Account{},
		&folder.Folder{},
		&message.Message{},
	)
```

- [ ] **Step 3: 验证编译**

Run: `cd flymail/backend && go build ./...`
Expected: 无错误。

- [ ] **Step 4: gofmt + 提交**

```bash
cd flymail/backend && gofmt -w modules/email/message/model.go internal/database/database.go
git add flymail/backend/modules/email/message/model.go flymail/backend/internal/database/database.go
git commit -m "feat(flymail): Message 模型 + AutoMigrate（(folder_id,uid) 唯一）"
```

---

## Task 7：Message 仓储（upsert + 列表分页 + 计数）

**Files:**
- Create: `flymail/backend/modules/email/message/repository.go`
- Create: `flymail/backend/modules/email/message/repository_test.go`

- [ ] **Step 1: 写失败测试**

`flymail/backend/modules/email/message/repository_test.go`:
```go
package message_test

import (
	"path/filepath"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/modules/email/message"
)

func newRepo(t *testing.T) *message.Repository {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return message.NewRepository(db)
}

func TestUpsertIsIdempotentByFolderUID(t *testing.T) {
	repo := newRepo(t)
	m := &message.Message{AccountID: 1, FolderID: 1, UID: 10, Subject: "A", Seen: false, Date: time.Now()}
	if err := repo.Upsert(m); err != nil {
		t.Fatalf("upsert1: %v", err)
	}
	// 同 (folder,uid) 再 upsert：更新 Seen + Subject，不新增
	m2 := &message.Message{AccountID: 1, FolderID: 1, UID: 10, Subject: "A-updated", Seen: true, Date: time.Now()}
	if err := repo.Upsert(m2); err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	n, _ := repo.CountByFolder(1)
	if n != 1 {
		t.Fatalf("want 1 row, got %d", n)
	}
	list, _ := repo.ListByFolder(1, 0, 50)
	if list[0].Subject != "A-updated" || !list[0].Seen {
		t.Errorf("upsert did not update: %+v", list[0])
	}
}

func TestListByFolderUIDCursorDesc(t *testing.T) {
	repo := newRepo(t)
	for _, uid := range []uint32{1, 2, 3, 4, 5} {
		_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: uid, Date: time.Now()})
	}
	// 第一页：最新 2 封（uid desc）→ 5,4
	page1, _ := repo.ListByFolder(1, 0, 2)
	if len(page1) != 2 || page1[0].UID != 5 || page1[1].UID != 4 {
		t.Fatalf("page1 wrong: %+v", page1)
	}
	// 第二页：before uid=4 → 3,2
	page2, _ := repo.ListByFolder(1, 4, 2)
	if len(page2) != 2 || page2[0].UID != 3 || page2[1].UID != 2 {
		t.Fatalf("page2 wrong: %+v", page2)
	}
}

func TestDeleteByFolder(t *testing.T) {
	repo := newRepo(t)
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 1, Date: time.Now()})
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 2, UID: 1, Date: time.Now()})
	if err := repo.DeleteByFolder(1); err != nil {
		t.Fatal(err)
	}
	n1, _ := repo.CountByFolder(1)
	n2, _ := repo.CountByFolder(2)
	if n1 != 0 || n2 != 1 {
		t.Errorf("delete scope wrong: f1=%d f2=%d", n1, n2)
	}
}
```

- [ ] **Step 2: 运行验证失败**

Run: `cd flymail/backend && go test ./modules/email/message/ -v`
Expected: 编译失败（`message.NewRepository` undefined）。

- [ ] **Step 3: 实现**

`flymail/backend/modules/email/message/repository.go`:
```go
package message

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

// Upsert 按 (folder_id, uid) 唯一键插入或更新元数据。
// 不更新 body_synced/snippet/has_attachment（正文相关，由 M4 流程维护）。
func (r *Repository) Upsert(m *Message) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "folder_id"}, {Name: "uid"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"account_id", "message_id", "in_reply_to", "references", "subject",
			"from_name", "from_addr", "to_json", "cc_json", "date", "size",
			"seen", "flagged", "answered", "deleted", "updated_at",
		}),
	}).Create(m).Error
}

// DeleteByFolder 删除某文件夹的全部邮件（UIDVALIDITY 变化时重建用）。
func (r *Repository) DeleteByFolder(folderID uint) error {
	return r.db.Where("folder_id = ?", folderID).Delete(&Message{}).Error
}

// ListByFolder 按 UID 倒序分页；beforeUID=0 表示从最新开始。
func (r *Repository) ListByFolder(folderID uint, beforeUID uint32, limit int) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := r.db.Where("folder_id = ?", folderID)
	if beforeUID > 0 {
		q = q.Where("uid < ?", beforeUID)
	}
	var list []Message
	err := q.Order("uid DESC").Limit(limit).Find(&list).Error
	return list, err
}

func (r *Repository) CountByFolder(folderID uint) (int64, error) {
	var n int64
	err := r.db.Model(&Message{}).Where("folder_id = ?", folderID).Count(&n).Error
	return n, err
}

func (r *Repository) UnreadCountByFolder(folderID uint) (int64, error) {
	var n int64
	err := r.db.Model(&Message{}).Where("folder_id = ? AND seen = ?", folderID, false).Count(&n).Error
	return n, err
}
```

> 注：`references` 是 SQLite 关键字，作为列名需确认 gorm/glebarez 不报错；如有问题，把模型字段 gorm 列名改为 `gorm:"column:references_hdr"` 并同步此处 AssignmentColumns。执行时若 Step 4 报 SQL 语法错误，按此调整。

- [ ] **Step 4: 运行验证通过**

Run: `cd flymail/backend && go test ./modules/email/message/ -v`
Expected: PASS。

- [ ] **Step 5: gofmt + 提交**

```bash
cd flymail/backend && gofmt -w modules/email/message/repository.go modules/email/message/repository_test.go
git add flymail/backend/modules/email/message/repository.go flymail/backend/modules/email/message/repository_test.go
git commit -m "feat(flymail): Message 仓储（upsert 幂等 + UID 游标分页）"
```

---

## Task 8：Message 服务（首同步单文件夹：分批 + UIDVALIDITY 兜底）

**Files:**
- Create: `flymail/backend/modules/email/message/service.go`
- Create: `flymail/backend/modules/email/message/service_test.go`

- [ ] **Step 1: 写失败测试**

`flymail/backend/modules/email/message/service_test.go`:
```go
package message_test

import (
	"path/filepath"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/modules/email/message"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	imapv2 "github.com/emersion/go-imap/v2"
)

// fakeFetcher 实现 message.IMAPFetcher，模拟一个 UIDNext=6、5 封邮件的文件夹。
type fakeFetcher struct {
	uidValidity uint32
	uidNext     uint32
	numMessages uint32
	emails      map[uint32]*types.ParsedEmail // uid -> email
	statusValidity uint32                     // 当 SelectFolder 返回 0 时由 FolderStatus 提供
	selectValidityZero bool
}

func (f *fakeFetcher) SelectFolder(path string) (*coreimap.SelectedFolder, error) {
	v := f.uidValidity
	if f.selectValidityZero {
		v = 0
	}
	return &coreimap.SelectedFolder{Path: path, NumMessages: f.numMessages, UIDValidity: v, UIDNext: f.uidNext}, nil
}

func (f *fakeFetcher) FolderStatus(path string, items ...coreimap.StatusItem) (*coreimap.FolderStatusResult, error) {
	v := f.statusValidity
	return &coreimap.FolderStatusResult{UIDValidity: &v}, nil
}

func (f *fakeFetcher) FetchByUIDRange(from, to imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	var out []*types.ParsedEmail
	for uid, e := range f.emails {
		if imapv2.UID(uid) >= from && (to == 0 || imapv2.UID(uid) <= to) {
			out = append(out, e)
		}
	}
	return out, nil
}

func newMsgService(t *testing.T) (*message.Service, *message.Repository) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := message.NewRepository(db)
	return message.NewService(repo), repo
}

func TestSyncFolderMessagesStoresMetadata(t *testing.T) {
	svc, repo := newMsgService(t)
	emails := map[uint32]*types.ParsedEmail{}
	for uid := uint32(1); uid <= 5; uid++ {
		emails[uid] = &types.ParsedEmail{
			UID: uid, Subject: "Mail", MessageID: "mid", Date: time.Now(),
			From: []types.Address{{Name: "张三", Email: "z@e.com"}},
			To:   []types.Address{{Name: "Me", Email: "me@e.com"}},
			IsRead: uid%2 == 0, Size: 100,
		}
	}
	f := &fakeFetcher{uidValidity: 42, uidNext: 6, numMessages: 5, emails: emails}
	state, rebuilt, err := svc.SyncFolderMessages(1, 1, "INBOX", 0, f)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if rebuilt {
		t.Errorf("first sync should not rebuild")
	}
	if state.UIDValidity != 42 || state.Total != 5 {
		t.Errorf("state wrong: %+v", state)
	}
	list, _ := repo.ListByFolder(1, 0, 50)
	if len(list) != 5 {
		t.Fatalf("want 5 stored, got %d", len(list))
	}
	if list[0].FromName != "张三" || list[0].FromAddr != "z@e.com" {
		t.Errorf("from not split: %+v", list[0])
	}
}

func TestSyncRebuildsOnUIDValidityChange(t *testing.T) {
	svc, repo := newMsgService(t)
	_ = repo.Upsert(&message.Message{AccountID: 1, FolderID: 1, UID: 999, Date: time.Now()})
	f := &fakeFetcher{uidValidity: 100, uidNext: 2, numMessages: 1, emails: map[uint32]*types.ParsedEmail{
		1: {UID: 1, Subject: "new", Date: time.Now()},
	}}
	// 本地 prevUIDValidity=42，服务器变成 100 → 应清空重建
	_, rebuilt, err := svc.SyncFolderMessages(1, 1, "INBOX", 42, f)
	if err != nil {
		t.Fatal(err)
	}
	if !rebuilt {
		t.Errorf("should rebuild on uidvalidity change")
	}
	list, _ := repo.ListByFolder(1, 0, 50)
	if len(list) != 1 || list[0].UID != 1 {
		t.Errorf("old uid 999 should be gone: %+v", list)
	}
}

func TestSyncUIDValidityFallbackToStatus(t *testing.T) {
	svc, _ := newMsgService(t)
	f := &fakeFetcher{uidNext: 2, numMessages: 1, selectValidityZero: true, statusValidity: 77,
		emails: map[uint32]*types.ParsedEmail{1: {UID: 1, Date: time.Now()}}}
	state, _, err := svc.SyncFolderMessages(1, 1, "INBOX", 0, f)
	if err != nil {
		t.Fatal(err)
	}
	if state.UIDValidity != 77 {
		t.Errorf("should fall back to STATUS uidvalidity, got %d", state.UIDValidity)
	}
}
```

- [ ] **Step 2: 运行验证失败**

Run: `cd flymail/backend && go test ./modules/email/message/ -run TestSync -v`
Expected: 编译失败（`message.NewService` / `IMAPFetcher` undefined）。

- [ ] **Step 3: 实现**

`flymail/backend/modules/email/message/service.go`:
```go
package message

import (
	"encoding/json"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	imapv2 "github.com/emersion/go-imap/v2"
)

const (
	defaultSyncDepth = 1000 // 首同步每文件夹最近 ~N 封
	fetchBatchSize   = 200  // 分批抓取，规避慢连接超时（旧 flymail 实证）
)

// IMAPFetcher 是邮件元数据同步所需的最小 IMAP 能力（便于测试 mock）。
// *coreimap.Session 满足此接口。
type IMAPFetcher interface {
	SelectFolder(path string) (*coreimap.SelectedFolder, error)
	FolderStatus(path string, items ...coreimap.StatusItem) (*coreimap.FolderStatusResult, error)
	FetchByUIDRange(from, to imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error)
}

// FolderState 是单文件夹同步后回写文件夹表所需的状态。
type FolderState struct {
	UIDValidity uint32
	UIDNext     uint32
	Total       int
	Unread      int
}

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

// SyncFolderMessages 同步单个文件夹最近 ~defaultSyncDepth 封邮件的元数据。
// prevUIDValidity 为本地已存的该文件夹 UIDVALIDITY（0=从未同步）。
// 返回：同步后状态、是否因 UIDVALIDITY 变化而重建、错误。
func (s *Service) SyncFolderMessages(accountID, folderID uint, folderPath string, prevUIDValidity uint32, c IMAPFetcher) (*FolderState, bool, error) {
	sel, err := c.SelectFolder(folderPath)
	if err != nil {
		return nil, false, err
	}

	// UIDVALIDITY 兜底：部分服务商 SELECT 不返回，用 STATUS 再取（旧 flymail 实证坑）。
	uidValidity := sel.UIDValidity
	if uidValidity == 0 {
		if st, serr := c.FolderStatus(folderPath, coreimap.StatusUIDValidity); serr == nil && st != nil && st.UIDValidity != nil {
			uidValidity = *st.UIDValidity
		}
	}

	rebuilt := false
	if prevUIDValidity != 0 && uidValidity != 0 && uidValidity != prevUIDValidity {
		if err := s.repo.DeleteByFolder(folderID); err != nil {
			return nil, false, err
		}
		rebuilt = true
	}

	if sel.NumMessages > 0 && sel.UIDNext > 0 {
		from := imapv2.UID(1)
		if sel.UIDNext > uint32(defaultSyncDepth) {
			from = imapv2.UID(sel.UIDNext - uint32(defaultSyncDepth))
		}
		end := imapv2.UID(sel.UIDNext - 1)
		if err := s.fetchRangeBatched(accountID, folderID, from, end, c); err != nil {
			return nil, rebuilt, err
		}
	}

	total, _ := s.repo.CountByFolder(folderID)
	unread, _ := s.repo.UnreadCountByFolder(folderID)
	return &FolderState{
		UIDValidity: uidValidity,
		UIDNext:     sel.UIDNext,
		Total:       int(total),
		Unread:      int(unread),
	}, rebuilt, nil
}

// fetchRangeBatched 把 [from,end] 切成 fetchBatchSize 的子区间逐批抓取并 upsert。
func (s *Service) fetchRangeBatched(accountID, folderID uint, from, end imapv2.UID, c IMAPFetcher) error {
	for start := from; start <= end; start += fetchBatchSize {
		batchEnd := start + fetchBatchSize - 1
		if batchEnd > end {
			batchEnd = end
		}
		emails, err := c.FetchByUIDRange(start, batchEnd, coreimap.FetchOptions{FetchBody: false, FallbackHeaders: true})
		if err != nil {
			return err
		}
		for _, e := range emails {
			if err := s.repo.Upsert(toMessage(accountID, folderID, e)); err != nil {
				return err
			}
		}
		if batchEnd == end {
			break
		}
	}
	return nil
}

func toMessage(accountID, folderID uint, e *types.ParsedEmail) *Message {
	m := &Message{
		AccountID: accountID,
		FolderID:  folderID,
		UID:       e.UID,
		MessageID: e.MessageID,
		Subject:   e.Subject,
		Date:      e.Date,
		Size:      e.Size,
		Seen:      e.IsRead,
		Flagged:   e.IsStarred,
	}
	if len(e.From) > 0 {
		m.FromName = e.From[0].Name
		m.FromAddr = e.From[0].Email
	}
	if b, err := json.Marshal(e.To); err == nil {
		m.ToJSON = string(b)
	}
	if b, err := json.Marshal(e.CC); err == nil {
		m.CcJSON = string(b)
	}
	for _, f := range e.Flags {
		switch f {
		case "\\Answered":
			m.Answered = true
		case "\\Deleted":
			m.Deleted = true
		}
	}
	return m
}
```

- [ ] **Step 4: 运行验证通过**

Run: `cd flymail/backend && go test ./modules/email/message/ -v`
Expected: PASS（三个 Sync 测试全过）。

- [ ] **Step 5: gofmt + 提交**

```bash
cd flymail/backend && gofmt -w modules/email/message/service.go modules/email/message/service_test.go
git add flymail/backend/modules/email/message/service.go flymail/backend/modules/email/message/service_test.go
git commit -m "feat(flymail): Message 服务（分批首同步 + UIDVALIDITY 兜底/重建）"
```

---

## Task 9：Message 路由 + DTO（列表）

**Files:**
- Create: `flymail/backend/modules/email/message/dto.go`
- Create: `flymail/backend/modules/email/message/handler.go`

- [ ] **Step 1: 实现 DTO（含收件人 JSON 反序列化）**

`flymail/backend/modules/email/message/dto.go`:
```go
package message

import (
	"encoding/json"

	"flymail-core/types"
)

// MessageListItem 是列表行的对外表示（无正文）。
type MessageListItem struct {
	ID            uint            `json:"id"`
	UID           uint32          `json:"uid"`
	Subject       string          `json:"subject"`
	FromName      string          `json:"from_name"`
	FromAddr      string          `json:"from_addr"`
	To            []types.Address `json:"to"`
	Date          string          `json:"date"` // RFC3339
	Size          int64           `json:"size"`
	Seen          bool            `json:"seen"`
	Flagged       bool            `json:"flagged"`
	HasAttachment bool            `json:"has_attachment"`
	Snippet       string          `json:"snippet"`
}

func toListItem(m *Message) MessageListItem {
	var to []types.Address
	if m.ToJSON != "" {
		_ = json.Unmarshal([]byte(m.ToJSON), &to)
	}
	return MessageListItem{
		ID: m.ID, UID: m.UID, Subject: m.Subject, FromName: m.FromName, FromAddr: m.FromAddr,
		To: to, Date: m.Date.Format("2006-01-02T15:04:05Z07:00"), Size: m.Size,
		Seen: m.Seen, Flagged: m.Flagged, HasAttachment: m.HasAttachment, Snippet: m.Snippet,
	}
}
```

- [ ] **Step 2: 实现 Handler**

`flymail/backend/modules/email/message/handler.go`:
```go
package message

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 挂载邮件列表路由：GET /folders/:fid/messages?before_uid=&limit=。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET("/folders/:fid/messages", h.list)
}

type handler struct{ svc *Service }

func (h *handler) list(c *gin.Context) {
	folderID, err := strconv.ParseUint(c.Param("fid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder id"})
		return
	}
	beforeUID, _ := strconv.ParseUint(c.DefaultQuery("before_uid", "0"), 10, 32)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	items, err := h.svc.List(uint(folderID), uint32(beforeUID), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": items})
}
```

- [ ] **Step 3: 给 Service 加 List 方法**

在 `flymail/backend/modules/email/message/service.go` 末尾追加：
```go
// List 返回文件夹内的邮件列表项（UID 游标分页）。
func (s *Service) List(folderID uint, beforeUID uint32, limit int) ([]MessageListItem, error) {
	rows, err := s.repo.ListByFolder(folderID, beforeUID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]MessageListItem, 0, len(rows))
	for i := range rows {
		out = append(out, toListItem(&rows[i]))
	}
	return out, nil
}
```

- [ ] **Step 4: 验证编译 + 测试**

Run: `cd flymail/backend && go build ./modules/email/message/ && go test ./modules/email/message/ -v`
Expected: PASS。

- [ ] **Step 5: gofmt + 提交**

```bash
cd flymail/backend && gofmt -w modules/email/message/dto.go modules/email/message/handler.go modules/email/message/service.go
git add flymail/backend/modules/email/message/dto.go flymail/backend/modules/email/message/handler.go flymail/backend/modules/email/message/service.go
git commit -m "feat(flymail): Message 路由 + DTO（GET /folders/:fid/messages）"
```

---

## Task 10：Sync 服务（后台编排 + 内存进度 + 账户串行锁）

**Files:**
- Create: `flymail/backend/modules/email/sync/service.go`
- Create: `flymail/backend/modules/email/sync/service_test.go`

- [ ] **Step 1: 写失败测试**

`flymail/backend/modules/email/sync/service_test.go`:
```go
package sync_test

import (
	"path/filepath"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	syncmod "flymail/modules/email/sync"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	imapv2 "github.com/emersion/go-imap/v2"
)

// fakeSession 满足 syncmod.Session（= folder.IMAPLister + message.IMAPFetcher + Close）。
type fakeSession struct{}

func (fakeSession) ListFolders() ([]types.FolderInfo, error) {
	return []types.FolderInfo{
		{Name: "Inbox", Path: "INBOX", Attributes: []string{"\\Inbox"}},
		{Name: "Sent", Path: "Sent", Attributes: []string{"\\Sent"}},
	}, nil
}
func (fakeSession) SelectFolder(path string) (*coreimap.SelectedFolder, error) {
	return &coreimap.SelectedFolder{Path: path, NumMessages: 2, UIDValidity: 1, UIDNext: 3}, nil
}
func (fakeSession) FolderStatus(path string, items ...coreimap.StatusItem) (*coreimap.FolderStatusResult, error) {
	v := uint32(1)
	return &coreimap.FolderStatusResult{UIDValidity: &v}, nil
}
func (fakeSession) FetchByUIDRange(from, to imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error) {
	return []*types.ParsedEmail{
		{UID: 1, Subject: "a", Date: time.Now()},
		{UID: 2, Subject: "b", Date: time.Now()},
	}, nil
}
func (fakeSession) Close() error { return nil }

// fakeAccounts 满足 syncmod.AccountConfigProvider。
type fakeAccounts struct{ touched bool }

func (f *fakeAccounts) IMAPConfig(id uint) (types.IMAPConfig, error) {
	return types.IMAPConfig{Host: "h"}, nil
}
func (f *fakeAccounts) TouchLastSync(id uint, t time.Time) error { f.touched = true; return nil }

func newSyncService(t *testing.T) (*syncmod.Service, *fakeAccounts, *folder.Service) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	fsvc := folder.NewService(folder.NewRepository(db))
	msvc := message.NewService(message.NewRepository(db))
	accts := &fakeAccounts{}
	svc := syncmod.NewService(accts, fsvc, msvc)
	svc.SetDial(func(cfg types.IMAPConfig) (syncmod.Session, error) { return fakeSession{}, nil })
	return svc, accts, fsvc
}

func TestTriggerRunsToCompletion(t *testing.T) {
	svc, accts, fsvc := newSyncService(t)
	if err := svc.Trigger(1); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	// 轮询直到完成（最多 2s）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := svc.StatusOf(1)
		if st.Phase == syncmod.PhaseDone || st.Phase == syncmod.PhaseError {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	st, ok := svc.StatusOf(1)
	if !ok || st.Phase != syncmod.PhaseDone {
		t.Fatalf("sync not done: %+v", st)
	}
	if !accts.touched {
		t.Errorf("TouchLastSync not called")
	}
	folders, _ := fsvc.List(1)
	if len(folders) != 2 {
		t.Errorf("folders not synced: %d", len(folders))
	}
}

func TestTriggerRejectsConcurrent(t *testing.T) {
	svc, _, _ := newSyncService(t)
	// 用阻塞 dial 模拟长时间运行
	block := make(chan struct{})
	svc.SetDial(func(cfg types.IMAPConfig) (syncmod.Session, error) {
		<-block
		return fakeSession{}, nil
	})
	if err := svc.Trigger(1); err != nil {
		t.Fatalf("first trigger: %v", err)
	}
	err := svc.Trigger(1)
	if err == nil {
		t.Errorf("second concurrent trigger should be rejected")
	}
	close(block)
}
```

- [ ] **Step 2: 运行验证失败**

Run: `cd flymail/backend && go test ./modules/email/sync/ -v`
Expected: 编译失败（`syncmod.NewService` undefined）。

- [ ] **Step 3: 实现**

`flymail/backend/modules/email/sync/service.go`:
```go
package sync

import (
	"errors"
	"sync"
	"time"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	"flymail/modules/email/folder"
	"flymail/modules/email/message"
)

// Session 是同步一个账户所需的 IMAP 能力集合（一个会话串行复用）。
// *coreimap.Session 满足此接口。
type Session interface {
	folder.IMAPLister
	message.IMAPFetcher
	Close() error
}

// AccountConfigProvider 由 account.Service 实现。
type AccountConfigProvider interface {
	IMAPConfig(id uint) (types.IMAPConfig, error)
	TouchLastSync(id uint, t time.Time) error
}

type Phase string

const (
	PhaseFolders  Phase = "folders"
	PhaseMessages Phase = "messages"
	PhaseDone     Phase = "done"
	PhaseError    Phase = "error"
)

// Status 是一次同步的内存进度快照。
type Status struct {
	AccountID uint      `json:"account_id"`
	Phase     Phase     `json:"phase"`
	Total     int       `json:"total"`
	Processed int       `json:"processed"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var ErrSyncRunning = errors.New("sync already running for this account")

type Service struct {
	accounts AccountConfigProvider
	folders  *folder.Service
	messages *message.Service

	dial func(types.IMAPConfig) (Session, error)

	mu       sync.Mutex
	statuses map[uint]*Status
	running  map[uint]bool
}

func NewService(accounts AccountConfigProvider, folders *folder.Service, messages *message.Service) *Service {
	return &Service{
		accounts: accounts, folders: folders, messages: messages,
		dial:     defaultDial,
		statuses: map[uint]*Status{},
		running:  map[uint]bool{},
	}
}

func defaultDial(cfg types.IMAPConfig) (Session, error) { return coreimap.Dial(cfg) }

// SetDial 覆盖拨号函数（测试用）。
func (s *Service) SetDial(d func(types.IMAPConfig) (Session, error)) { s.dial = d }

// Trigger 启动一次后台首同步；同账户已在跑则返回 ErrSyncRunning。
func (s *Service) Trigger(accountID uint) error {
	s.mu.Lock()
	if s.running[accountID] {
		s.mu.Unlock()
		return ErrSyncRunning
	}
	s.running[accountID] = true
	now := time.Now()
	s.statuses[accountID] = &Status{AccountID: accountID, Phase: PhaseFolders, StartedAt: now, UpdatedAt: now}
	s.mu.Unlock()

	go s.run(accountID)
	return nil
}

func (s *Service) StatusOf(accountID uint) (Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.statuses[accountID]
	if !ok {
		return Status{}, false
	}
	return *st, true
}

func (s *Service) setStatus(accountID uint, fn func(*Status)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.statuses[accountID]; st != nil {
		fn(st)
		st.UpdatedAt = time.Now()
	}
}

func (s *Service) run(accountID uint) {
	defer func() {
		s.mu.Lock()
		s.running[accountID] = false
		s.mu.Unlock()
	}()

	cfg, err := s.accounts.IMAPConfig(accountID)
	if err != nil {
		s.fail(accountID, err)
		return
	}
	sess, err := s.dial(cfg)
	if err != nil {
		s.fail(accountID, err)
		return
	}
	defer sess.Close()

	// 阶段一：文件夹
	if err := s.folders.SyncFolders(accountID, sess); err != nil {
		s.fail(accountID, err)
		return
	}

	// 阶段二：仅 INBOX 元数据
	s.setStatus(accountID, func(st *Status) { st.Phase = PhaseMessages })
	inbox, err := s.folders.FindInbox(accountID)
	if err != nil {
		s.fail(accountID, err)
		return
	}
	if inbox != nil {
		state, _, err := s.messages.SyncFolderMessages(accountID, inbox.ID, inbox.Path, inbox.UIDValidity, sess)
		if err != nil {
			s.fail(accountID, err)
			return
		}
		if err := s.folders.UpdateSyncState(inbox.ID, state.UIDValidity, state.UIDNext, state.Total, state.Unread, time.Now()); err != nil {
			s.fail(accountID, err)
			return
		}
		s.setStatus(accountID, func(st *Status) { st.Total = state.Total; st.Processed = state.Total })
	}

	_ = s.accounts.TouchLastSync(accountID, time.Now())
	s.setStatus(accountID, func(st *Status) { st.Phase = PhaseDone })
}

func (s *Service) fail(accountID uint, err error) {
	s.setStatus(accountID, func(st *Status) { st.Phase = PhaseError; st.Error = err.Error() })
}
```

- [ ] **Step 4: 运行验证通过**

Run: `cd flymail/backend && go test ./modules/email/sync/ -v`
Expected: PASS（两个测试均过）。

- [ ] **Step 5: gofmt + 提交**

```bash
cd flymail/backend && gofmt -w modules/email/sync/service.go modules/email/sync/service_test.go
git add flymail/backend/modules/email/sync/service.go flymail/backend/modules/email/sync/service_test.go
git commit -m "feat(flymail): Sync 服务（后台编排 + 内存进度 + 账户串行锁）"
```

---

## Task 11：Sync 路由

**Files:**
- Create: `flymail/backend/modules/email/sync/handler.go`

- [ ] **Step 1: 实现 Handler**

`flymail/backend/modules/email/sync/handler.go`:
```go
package sync

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 挂载同步路由：POST /accounts/:id/sync、GET /accounts/:id/sync/status。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.POST("/accounts/:id/sync", h.trigger)
	rg.GET("/accounts/:id/sync/status", h.status)
}

type handler struct{ svc *Service }

func (h *handler) trigger(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}
	if err := h.svc.Trigger(uint(id)); err != nil {
		if errors.Is(err, ErrSyncRunning) {
			c.JSON(http.StatusConflict, gin.H{"error": "sync already running"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "started"})
}

func (h *handler) status(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}
	st, ok := h.svc.StatusOf(uint(id))
	if !ok {
		c.JSON(http.StatusOK, gin.H{"phase": "none"})
		return
	}
	c.JSON(http.StatusOK, st)
}
```

- [ ] **Step 2: 验证编译**

Run: `cd flymail/backend && go build ./modules/email/sync/`
Expected: 无错误。

- [ ] **Step 3: gofmt + 提交**

```bash
cd flymail/backend && gofmt -w modules/email/sync/handler.go
git add flymail/backend/modules/email/sync/handler.go
git commit -m "feat(flymail): Sync 路由（POST /sync + GET /sync/status）"
```

---

## Task 12：装配（router + app）

**Files:**
- Modify: `flymail/backend/internal/server/router.go`
- Modify: `flymail/backend/internal/app/app.go`

- [ ] **Step 1: 扩展 Deps + 挂路由**

`flymail/backend/internal/server/router.go`：`Deps` 追加字段并在受保护组挂载（import 三个新包）：
```go
import (
	// ... 既有 ...
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	syncmod "flymail/modules/email/sync"
)

type Deps struct {
	Auth    *auth.Service
	Account *account.Service
	Folder  *folder.Service
	Message *message.Service
	Sync    *syncmod.Service
}
```
在现有 `protected` 组内（`account.RegisterRoutes(protected, deps.Account)` 之后）追加：
```go
	if deps.Auth != nil && deps.Account != nil {
		protected := api.Group("")
		protected.Use(auth.Middleware(deps.Auth))
		account.RegisterRoutes(protected, deps.Account)
		if deps.Folder != nil {
			folder.RegisterRoutes(protected, deps.Folder)
		}
		if deps.Message != nil {
			message.RegisterRoutes(protected, deps.Message)
		}
		if deps.Sync != nil {
			syncmod.RegisterRoutes(protected, deps.Sync)
		}
	}
```

- [ ] **Step 2: app 装配新 service**

`flymail/backend/internal/app/app.go` 在 `accountSvc` 之后、`server.New` 之前追加（import 三个新包）：
```go
	folderSvc := folder.NewService(folder.NewRepository(db))
	messageSvc := message.NewService(message.NewRepository(db))
	syncSvc := syncmod.NewService(accountSvc, folderSvc, messageSvc)
	handler := server.New(server.Deps{
		Auth:    authSvc,
		Account: accountSvc,
		Folder:  folderSvc,
		Message: messageSvc,
		Sync:    syncSvc,
	})
```
import 追加：
```go
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	syncmod "flymail/modules/email/sync"
```

> 注：`syncmod.NewService(accountSvc, ...)` 要求 `*account.Service` 满足 `syncmod.AccountConfigProvider`（已在 Task 1 实现 `IMAPConfig`/`TouchLastSync`）。编译即验证此契约。

- [ ] **Step 3: 全量构建 + 测试**

Run: `cd flymail/backend && go build ./... && go test ./...`
Expected: 全部 PASS，无编译错误。

- [ ] **Step 4: gofmt + 提交**

```bash
cd flymail/backend && gofmt -w internal/server/router.go internal/app/app.go
git add flymail/backend/internal/server/router.go flymail/backend/internal/app/app.go
git commit -m "feat(flymail): 装配 folder/message/sync 到 router 与 app"
```

---

## Task 13：后端端到端冒烟（真实/或脚本）

**Files:**
- Create: `flymail/backend/smoke_m3_test.go`（可选集成测试，默认 skip 除非设置真实账户环境变量）

- [ ] **Step 1: 写带环境开关的集成冒烟**

`flymail/backend/smoke_m3_test.go`:
```go
package main

import (
	"os"
	"testing"
	"time"

	"flymail/internal/crypto"
	"flymail/internal/database"
	"flymail/modules/email/account"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	syncmod "flymail/modules/email/sync"
)

// TestM3SmokeRealAccount 仅在设置了 FLYMAIL_SMOKE_* 环境变量时运行真实 IMAP 冒烟。
// 例：FLYMAIL_SMOKE_EMAIL/PW/IMAP_HOST/IMAP_PORT go test -run TestM3Smoke -v
func TestM3SmokeRealAccount(t *testing.T) {
	email := os.Getenv("FLYMAIL_SMOKE_EMAIL")
	if email == "" {
		t.Skip("set FLYMAIL_SMOKE_EMAIL etc. to run real IMAP smoke")
	}
	db, err := database.Open(t.TempDir() + "/smoke.db")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	enc, _ := crypto.New("smoke-key-smoke-key-smoke-key-32")
	acctSvc := account.NewService(account.NewRepository(db), enc)
	created, err := acctSvc.Create(account.CreateAccountRequest{
		Name: "smoke", Email: email, Password: os.Getenv("FLYMAIL_SMOKE_PW"),
		IMAPHost: os.Getenv("FLYMAIL_SMOKE_IMAP_HOST"), IMAPPort: atoiEnv("FLYMAIL_SMOKE_IMAP_PORT", 993), IMAPSecurity: "ssl",
		SMTPHost: os.Getenv("FLYMAIL_SMOKE_IMAP_HOST"), SMTPPort: 465, SMTPSecurity: "ssl",
	})
	if err != nil {
		t.Fatal(err)
	}
	fsvc := folder.NewService(folder.NewRepository(db))
	msvc := message.NewService(message.NewRepository(db))
	syncSvc := syncmod.NewService(acctSvc, fsvc, msvc)
	if err := syncSvc.Trigger(created.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		st, _ := syncSvc.StatusOf(created.ID)
		if st.Phase == syncmod.PhaseDone {
			break
		}
		if st.Phase == syncmod.PhaseError {
			t.Fatalf("sync error: %s", st.Error)
		}
		time.Sleep(500 * time.Millisecond)
	}
	folders, _ := fsvc.List(created.ID)
	t.Logf("synced %d folders", len(folders))
	if len(folders) == 0 {
		t.Fatal("no folders synced")
	}
	inbox, _ := fsvc.FindInbox(created.ID)
	if inbox == nil {
		t.Fatal("inbox not found")
	}
	items, _ := msvc.List(inbox.ID, 0, 20)
	t.Logf("inbox first page: %d messages", len(items))
	for _, m := range items {
		t.Logf("  uid=%d seen=%v from=%q subject=%q", m.UID, m.Seen, m.FromName, m.Subject)
	}
}

func atoiEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		_, _ = fmtSscan(v, &n)
		if n > 0 {
			return n
		}
	}
	return def
}
```

> 注：`fmtSscan` 用标准库——把 `atoiEnv` 改为用 `strconv.Atoi` 实现（避免 helper 占位）。最终实现：
```go
import "strconv"
func atoiEnv(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return def
}
```
（删除上面 `fmtSscan` 版本，仅保留此 `strconv` 版本。）

- [ ] **Step 2: 跑单测（无环境变量时应 SKIP）**

Run: `cd flymail/backend && go test -run TestM3Smoke -v`
Expected: SKIP（未设置真实账户）。

- [ ] **Step 3: （人工）用真实账户跑一次**

在你本机用一个真实邮箱（如 163 授权码）执行：
```bash
cd flymail/backend
FLYMAIL_SMOKE_EMAIL=you@163.com FLYMAIL_SMOKE_PW=授权码 \
FLYMAIL_SMOKE_IMAP_HOST=imap.163.com FLYMAIL_SMOKE_IMAP_PORT=993 \
go test -run TestM3Smoke -v
```
Expected: 打印同步到的文件夹数与 INBOX 首页若干邮件（uid/发件人/主题为正常中文，无乱码）。

- [ ] **Step 4: gofmt + 提交**

```bash
cd flymail/backend && gofmt -w smoke_m3_test.go
git add flymail/backend/smoke_m3_test.go
git commit -m "test(flymail): M3 真实账户端到端冒烟（环境变量开关）"
```

---

# 前端

> 前端遵循 `flymail/frontend/CLAUDE.md`：pnpm、完整 TS 类型、文本走 i18n、改 JSON 后验证格式、勿自行 `pnpm dev`。本计划只描述代码改动，**热重载由用户通过 `dev.ps1` 启动**。

## Task 14：i18n 基础设施 + 主题 CSS 变量

**Files:**
- Modify: `flymail/frontend/package.json`（加依赖）
- Create: `flymail/frontend/src/lib/i18n.ts`
- Create: `flymail/frontend/src/locales/zh.json`
- Create: `flymail/frontend/src/locales/en.json`
- Modify: `flymail/frontend/src/main.tsx`
- Modify: `flymail/frontend/src/index.css`

- [ ] **Step 1: 安装 i18n 依赖**

```bash
cd flymail/frontend && pnpm add i18next react-i18next
```

- [ ] **Step 2: 创建 locale 资源**

`flymail/frontend/src/locales/zh.json`:
```json
{
  "app": { "name": "FlyMail" },
  "sidebar": { "compose": "写邮件", "accounts": "账户", "settings": "设置" },
  "folder": {
    "inbox": "收件箱", "sent": "已发送", "drafts": "草稿",
    "trash": "已删除", "junk": "垃圾邮件", "archive": "归档", "custom": "文件夹"
  },
  "list": {
    "search": "搜索邮件", "unreadCount": "{{count}} 封未读",
    "empty": "暂无邮件", "filterAll": "全部", "filterUnread": "未读", "filterFlagged": "星标"
  },
  "sync": {
    "trigger": "同步", "syncing": "同步中…", "folders": "正在同步文件夹…",
    "messages": "正在同步邮件…", "done": "同步完成", "error": "同步失败：{{error}}"
  },
  "reader": { "welcome": "选择一封邮件以阅读", "notReady": "邮件阅读将在后续版本提供" }
}
```

`flymail/frontend/src/locales/en.json`:
```json
{
  "app": { "name": "FlyMail" },
  "sidebar": { "compose": "Compose", "accounts": "Accounts", "settings": "Settings" },
  "folder": {
    "inbox": "Inbox", "sent": "Sent", "drafts": "Drafts",
    "trash": "Trash", "junk": "Junk", "archive": "Archive", "custom": "Folder"
  },
  "list": {
    "search": "Search mail", "unreadCount": "{{count}} unread",
    "empty": "No messages", "filterAll": "All", "filterUnread": "Unread", "filterFlagged": "Flagged"
  },
  "sync": {
    "trigger": "Sync", "syncing": "Syncing…", "folders": "Syncing folders…",
    "messages": "Syncing messages…", "done": "Sync complete", "error": "Sync failed: {{error}}"
  },
  "reader": { "welcome": "Select a message to read", "notReady": "Message reading comes in a later version" }
}
```

- [ ] **Step 3: i18n 初始化**

`flymail/frontend/src/lib/i18n.ts`:
```ts
import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import zh from '@/locales/zh.json'
import en from '@/locales/en.json'

void i18n.use(initReactI18next).init({
  resources: {
    zh: { translation: zh },
    en: { translation: en },
  },
  lng: 'zh',
  fallbackLng: 'en',
  interpolation: { escapeValue: false },
})

export default i18n
```

- [ ] **Step 4: main 引入 i18n + 主题变量**

`flymail/frontend/src/main.tsx` 顶部 import 追加：`import '@/lib/i18n'`

`flymail/frontend/src/index.css` 追加主题变量（中文友好字体；布局/配色 token 借鉴 MailMaster 但字体本地化）：
```css
:root {
  /* 中文友好字体栈（不复刻参考的英文衬线） */
  --font-body: -apple-system, "Segoe UI", "Microsoft YaHei", "PingFang SC", "Hiragino Sans GB", sans-serif;
  --font-mono: "JetBrains Mono", "Cascadia Code", Consolas, monospace;

  /* 布局尺寸 */
  --sidebar-w: 248px;
  --list-w: 380px;

  /* 配色（暖色 Light，借鉴 MailMaster） */
  --bg: #fbfaf7;
  --bg-alt: #f5f3ee;
  --surface: #ffffff;
  --ink: #2a2823;
  --ink-2: #56524a;
  --ink-3: #8a857a;
  --rule: rgba(40, 36, 28, 0.08);
  --rule-strong: rgba(40, 36, 28, 0.14);
  --bg-hover: rgba(40, 36, 28, 0.035);
  --accent: #b5886b;
  --accent-wash: #f5ece0;
  --accent-ink: #7a5536;
}

body {
  font-family: var(--font-body);
  background: var(--bg);
  color: var(--ink);
}
```

- [ ] **Step 5: 验证类型 + 提交**

Run: `cd flymail/frontend && pnpm exec tsc -b --noEmit`
Expected: 无类型错误。

```bash
cd flymail/frontend
git add package.json pnpm-lock.yaml src/lib/i18n.ts src/locales/zh.json src/locales/en.json src/main.tsx src/index.css
git commit -m "feat(flymail-fe): i18n 基础设施 + 主题 CSS 变量（中文字体）"
```

---

## Task 15：API 类型 + TanStack Query hooks

**Files:**
- Create: `flymail/frontend/src/lib/types.ts`
- Create: `flymail/frontend/src/lib/queries.ts`

> 前置：确认 `src/lib/api.ts` 导出了一个带鉴权头的 axios 实例（M1 已建）。下面以 `import { api } from '@/lib/api'` 引用；若导出名不同，按实际调整。

- [ ] **Step 1: 定义 API 类型**

`flymail/frontend/src/lib/types.ts`:
```ts
export interface Account {
  id: number
  name: string
  email: string
}

export interface Folder {
  id: number
  account_id: number
  path: string
  display_name: string
  type: string
  selectable: boolean
  total_count: number
  unread_count: number
  sort_order: number
}

export interface Address {
  name: string
  email: string
}

export interface MessageListItem {
  id: number
  uid: number
  subject: string
  from_name: string
  from_addr: string
  to: Address[]
  date: string
  size: number
  seen: boolean
  flagged: boolean
  has_attachment: boolean
  snippet: string
}

export type SyncPhase = 'none' | 'folders' | 'messages' | 'done' | 'error'

export interface SyncStatus {
  account_id?: number
  phase: SyncPhase
  total?: number
  processed?: number
  error?: string
}
```

- [ ] **Step 2: 实现 Query hooks**

`flymail/frontend/src/lib/queries.ts`:
```ts
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type { Account, Folder, MessageListItem, SyncStatus } from '@/lib/types'

export function useAccounts() {
  return useQuery({
    queryKey: ['accounts'],
    queryFn: async (): Promise<Account[]> => {
      const { data } = await api.get<{ accounts: Account[] }>('/accounts')
      return data.accounts ?? []
    },
  })
}

export function useFolders(accountId: number | null) {
  return useQuery({
    queryKey: ['folders', accountId],
    enabled: accountId != null,
    queryFn: async (): Promise<Folder[]> => {
      const { data } = await api.get<{ folders: Folder[] }>(`/accounts/${accountId}/folders`)
      return data.folders ?? []
    },
  })
}

export function useMessages(folderId: number | null) {
  return useQuery({
    queryKey: ['messages', folderId],
    enabled: folderId != null,
    queryFn: async (): Promise<MessageListItem[]> => {
      const { data } = await api.get<{ messages: MessageListItem[] }>(`/folders/${folderId}/messages?limit=50`)
      return data.messages ?? []
    },
  })
}

export function useSyncStatus(accountId: number | null, enabled: boolean) {
  return useQuery({
    queryKey: ['sync-status', accountId],
    enabled: accountId != null && enabled,
    refetchInterval: enabled ? 1000 : false,
    queryFn: async (): Promise<SyncStatus> => {
      const { data } = await api.get<SyncStatus>(`/accounts/${accountId}/sync/status`)
      return data
    },
  })
}

export function useTriggerSync() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (accountId: number) => {
      await api.post(`/accounts/${accountId}/sync`)
    },
    onSuccess: (_data, accountId) => {
      void qc.invalidateQueries({ queryKey: ['sync-status', accountId] })
    },
  })
}
```

- [ ] **Step 3: 验证类型 + 提交**

Run: `cd flymail/frontend && pnpm exec tsc -b --noEmit`
Expected: 无类型错误（若 `api` 导出名不符，先修正 import）。

```bash
cd flymail/frontend
git add src/lib/types.ts src/lib/queries.ts
git commit -m "feat(flymail-fe): API 类型 + TanStack Query hooks（folders/messages/sync）"
```

---

## Task 16：三栏布局骨架 AppLayout

**Files:**
- Create: `flymail/frontend/src/components/mail/AppLayout.tsx`

- [ ] **Step 1: 实现 AppLayout**

`flymail/frontend/src/components/mail/AppLayout.tsx`:
```tsx
import type { ReactNode } from 'react'

interface AppLayoutProps {
  sidebar: ReactNode
  list: ReactNode
  reader: ReactNode
}

// 三栏布局：248px 侧栏 / 380px 列表 / flex-1 阅读区（参考 MailMaster 比例）。
export function AppLayout({ sidebar, list, reader }: AppLayoutProps) {
  return (
    <div className="flex h-screen w-screen overflow-hidden">
      <aside
        className="flex-shrink-0 overflow-y-auto"
        style={{ width: 'var(--sidebar-w)', background: 'var(--bg-alt)', borderRight: '1px solid var(--rule)' }}
      >
        {sidebar}
      </aside>
      <section
        className="flex flex-shrink-0 flex-col overflow-hidden"
        style={{ width: 'var(--list-w)', borderRight: '1px solid var(--rule)' }}
      >
        {list}
      </section>
      <main className="min-w-0 flex-1 overflow-y-auto">{reader}</main>
    </div>
  )
}
```

- [ ] **Step 2: 验证类型 + 提交**

Run: `cd flymail/frontend && pnpm exec tsc -b --noEmit`
Expected: 无类型错误。

```bash
cd flymail/frontend
git add src/components/mail/AppLayout.tsx
git commit -m "feat(flymail-fe): 三栏布局骨架 AppLayout"
```

---

## Task 17：AccountSidebar（账户 + 文件夹 + 同步按钮）

**Files:**
- Create: `flymail/frontend/src/components/mail/AccountSidebar.tsx`

- [ ] **Step 1: 实现 AccountSidebar**

`flymail/frontend/src/components/mail/AccountSidebar.tsx`:
```tsx
import { useTranslation } from 'react-i18next'
import { Inbox, Send, FileText, Trash2, ShieldAlert, Archive, Folder as FolderIcon, RefreshCw } from 'lucide-react'
import type { Account, Folder } from '@/lib/types'

const folderIcon: Record<string, typeof Inbox> = {
  inbox: Inbox, sent: Send, drafts: FileText, trash: Trash2, junk: ShieldAlert, archive: Archive,
}

interface Props {
  accounts: Account[]
  folders: Folder[]
  activeAccountId: number | null
  activeFolderId: number | null
  syncing: boolean
  onSelectAccount: (id: number) => void
  onSelectFolder: (id: number) => void
  onSync: (accountId: number) => void
}

export function AccountSidebar({
  accounts, folders, activeAccountId, activeFolderId, syncing,
  onSelectAccount, onSelectFolder, onSync,
}: Props) {
  const { t } = useTranslation()
  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between px-4 py-3" style={{ borderBottom: '1px solid var(--rule)' }}>
        <span className="text-base font-medium">{t('app.name')}</span>
      </div>
      <div className="flex-1 overflow-y-auto px-2 py-2">
        {accounts.map((acc) => (
          <div key={acc.id} className="mb-2">
            <button
              type="button"
              onClick={() => onSelectAccount(acc.id)}
              className="flex w-full items-center justify-between rounded-md px-2 py-1.5 text-sm"
              style={{ color: acc.id === activeAccountId ? 'var(--ink)' : 'var(--ink-2)', background: acc.id === activeAccountId ? 'var(--bg-hover)' : 'transparent' }}
            >
              <span className="truncate">{acc.name || acc.email}</span>
              <RefreshCw
                size={14}
                className={syncing && acc.id === activeAccountId ? 'animate-spin' : ''}
                onClick={(e) => { e.stopPropagation(); onSync(acc.id) }}
                aria-label={t('sync.trigger')}
              />
            </button>
            {acc.id === activeAccountId &&
              folders
                .filter((f) => f.selectable)
                .map((f) => {
                  const Icon = folderIcon[f.type] ?? FolderIcon
                  const label = f.type === 'custom' ? f.display_name : t(`folder.${f.type}`)
                  return (
                    <button
                      key={f.id}
                      type="button"
                      onClick={() => onSelectFolder(f.id)}
                      className="flex w-full items-center gap-2 rounded-md py-1.5 pl-7 pr-2 text-[13px]"
                      style={{ color: f.id === activeFolderId ? 'var(--ink)' : 'var(--ink-2)', background: f.id === activeFolderId ? 'var(--accent-wash)' : 'transparent' }}
                    >
                      <Icon size={14} style={{ color: 'var(--ink-3)' }} />
                      <span className="flex-1 truncate text-left">{label}</span>
                      {f.unread_count > 0 && <span className="text-[10.5px]" style={{ color: 'var(--ink-3)' }}>{f.unread_count}</span>}
                    </button>
                  )
                })}
          </div>
        ))}
      </div>
    </div>
  )
}
```

- [ ] **Step 2: 验证类型 + 提交**

Run: `cd flymail/frontend && pnpm exec tsc -b --noEmit`
Expected: 无类型错误（若 lucide-react 图标名不存在，按其导出调整）。

```bash
cd flymail/frontend
git add src/components/mail/AccountSidebar.tsx
git commit -m "feat(flymail-fe): AccountSidebar（账户树 + 文件夹 + 同步按钮）"
```

---

## Task 18：MailList（列表头 + 邮件项）

**Files:**
- Create: `flymail/frontend/src/components/mail/MailList.tsx`

- [ ] **Step 1: 实现 MailList**

`flymail/frontend/src/components/mail/MailList.tsx`:
```tsx
import { useTranslation } from 'react-i18next'
import { Paperclip } from 'lucide-react'
import type { Folder, MessageListItem } from '@/lib/types'

interface Props {
  folder: Folder | null
  messages: MessageListItem[]
  loading: boolean
  activeMessageId: number | null
  onSelectMessage: (id: number) => void
}

function initials(name: string, addr: string): string {
  const s = (name || addr || '?').trim()
  return s.slice(0, 1).toUpperCase()
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getMonth() + 1}/${d.getDate()}`
}

export function MailList({ folder, messages, loading, activeMessageId, onSelectMessage }: Props) {
  const { t } = useTranslation()
  const title = folder ? (folder.type === 'custom' ? folder.display_name : t(`folder.${folder.type}`)) : ''
  return (
    <div className="flex h-full flex-col">
      <div className="px-5 py-3" style={{ borderBottom: '1px solid var(--rule)' }}>
        <div className="text-lg font-medium">{title}</div>
        {folder && folder.unread_count > 0 && (
          <div className="text-[11px]" style={{ color: 'var(--ink-3)' }}>
            {t('list.unreadCount', { count: folder.unread_count })}
          </div>
        )}
      </div>
      <div className="flex-1 overflow-y-auto">
        {loading && <div className="px-5 py-8 text-center text-sm" style={{ color: 'var(--ink-3)' }}>…</div>}
        {!loading && messages.length === 0 && (
          <div className="px-5 py-8 text-center text-sm" style={{ color: 'var(--ink-3)' }}>{t('list.empty')}</div>
        )}
        {messages.map((m) => (
          <button
            key={m.id}
            type="button"
            onClick={() => onSelectMessage(m.id)}
            className="grid w-full grid-cols-[32px_1fr] gap-3 px-5 py-3.5 text-left"
            style={{
              borderBottom: '1px solid var(--rule)',
              background: m.id === activeMessageId ? 'var(--accent-wash)' : 'transparent',
            }}
          >
            <div
              className="flex h-8 w-8 items-center justify-center rounded-md text-[12.5px] font-semibold text-white"
              style={{ background: 'var(--accent)' }}
            >
              {initials(m.from_name, m.from_addr)}
            </div>
            <div className="min-w-0">
              <div className="flex items-center justify-between gap-2">
                <span className="truncate text-[13.5px]" style={{ color: m.seen ? 'var(--ink-2)' : 'var(--ink)', fontWeight: m.seen ? 400 : 600 }}>
                  {m.from_name || m.from_addr}
                </span>
                <span className="flex items-center gap-1 text-[10.5px]" style={{ color: 'var(--ink-3)' }}>
                  {m.has_attachment && <Paperclip size={11} />}
                  {formatDate(m.date)}
                </span>
              </div>
              <div className="truncate text-[13px]" style={{ color: m.seen ? 'var(--ink-2)' : 'var(--ink)', fontWeight: m.seen ? 400 : 600 }}>
                {m.subject || '(无主题)'}
              </div>
              {m.snippet && <div className="truncate text-[12px]" style={{ color: 'var(--ink-3)' }}>{m.snippet}</div>}
            </div>
          </button>
        ))}
      </div>
    </div>
  )
}
```

- [ ] **Step 2: 验证类型 + 提交**

Run: `cd flymail/frontend && pnpm exec tsc -b --noEmit`
Expected: 无类型错误。

```bash
cd flymail/frontend
git add src/components/mail/MailList.tsx
git commit -m "feat(flymail-fe): MailList（邮件项 + 未读加粗 + 附件标）"
```

---

## Task 19：Reader 占位 + Shell 组装 + URL 驱动

**Files:**
- Create: `flymail/frontend/src/components/mail/Reader.tsx`
- Modify: `flymail/frontend/src/pages/Shell.tsx`

- [ ] **Step 1: Reader 占位**

`flymail/frontend/src/components/mail/Reader.tsx`:
```tsx
import { useTranslation } from 'react-i18next'

export function Reader() {
  const { t } = useTranslation()
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 px-8 text-center">
      <p className="text-sm" style={{ color: 'var(--ink-2)' }}>{t('reader.welcome')}</p>
      <p className="text-xs" style={{ color: 'var(--ink-3)' }}>{t('reader.notReady')}</p>
    </div>
  )
}
```

- [ ] **Step 2: Shell 组装三栏 + URL 状态**

`flymail/frontend/src/pages/Shell.tsx`（整文件替换为以下，URL query 驱动 account/folder/message 选择）：
```tsx
import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router'
import { AppLayout } from '@/components/mail/AppLayout'
import { AccountSidebar } from '@/components/mail/AccountSidebar'
import { MailList } from '@/components/mail/MailList'
import { Reader } from '@/components/mail/Reader'
import { useAccounts, useFolders, useMessages, useSyncStatus, useTriggerSync } from '@/lib/queries'

export default function Shell() {
  const [params, setParams] = useSearchParams()
  const accountId = params.get('account') ? Number(params.get('account')) : null
  const folderId = params.get('folder') ? Number(params.get('folder')) : null
  const messageId = params.get('message') ? Number(params.get('message')) : null

  const { data: accounts = [] } = useAccounts()
  const { data: folders = [] } = useFolders(accountId)
  const { data: messages = [], isLoading: messagesLoading } = useMessages(folderId)

  const [syncEnabled, setSyncEnabled] = useState(false)
  const { data: syncStatus } = useSyncStatus(accountId, syncEnabled)
  const triggerSync = useTriggerSync()
  const syncing = syncStatus?.phase === 'folders' || syncStatus?.phase === 'messages'

  // 默认选中第一个账户
  useEffect(() => {
    if (accountId == null && accounts.length > 0) {
      setParams((p) => { p.set('account', String(accounts[0].id)); return p }, { replace: true })
    }
  }, [accountId, accounts, setParams])

  // 同步完成后停止轮询并刷新文件夹
  useEffect(() => {
    if (syncStatus?.phase === 'done' || syncStatus?.phase === 'error') setSyncEnabled(false)
  }, [syncStatus?.phase])

  const activeFolder = useMemo(() => folders.find((f) => f.id === folderId) ?? null, [folders, folderId])

  function selectAccount(id: number) {
    setParams((p) => { p.set('account', String(id)); p.delete('folder'); p.delete('message'); return p })
  }
  function selectFolder(id: number) {
    setParams((p) => { p.set('folder', String(id)); p.delete('message'); return p })
  }
  function selectMessage(id: number) {
    setParams((p) => { p.set('message', String(id)); return p })
  }
  function onSync(id: number) {
    setSyncEnabled(true)
    triggerSync.mutate(id)
  }

  return (
    <AppLayout
      sidebar={
        <AccountSidebar
          accounts={accounts}
          folders={folders}
          activeAccountId={accountId}
          activeFolderId={folderId}
          syncing={syncing}
          onSelectAccount={selectAccount}
          onSelectFolder={selectFolder}
          onSync={onSync}
        />
      }
      list={
        <MailList
          folder={activeFolder}
          messages={messages}
          loading={messagesLoading}
          activeMessageId={messageId}
          onSelectMessage={selectMessage}
        />
      }
      reader={<Reader />}
    />
  )
}
```

> 注：若现有 `Shell.tsx` 为命名导出或被 `router.tsx` 以特定名引用，保持其导出形式一致（默认导出 vs 命名导出），仅替换内部实现。`react-router` v7 的 `useSearchParams` 函数式更新签名以实际版本为准；若不支持函数式 setter，改为构造新的 `URLSearchParams` 再 `setParams(next)`。

- [ ] **Step 3: 验证类型 + 构建**

Run: `cd flymail/frontend && pnpm exec tsc -b --noEmit`
Expected: 无类型错误。

- [ ] **Step 4: gofmt 不适用 / 提交**

```bash
cd flymail/frontend
git add src/components/mail/Reader.tsx src/pages/Shell.tsx
git commit -m "feat(flymail-fe): Reader 占位 + Shell 三栏组装 + URL 驱动选择"
```

---

## Task 20：前端构建 + 端到端联调验证

- [ ] **Step 1: 构建前端（产物进 backend embed）**

Run: `cd flymail/frontend && pnpm build`
Expected: 生成 `flymail/backend/web/dist/index.html`。

- [ ] **Step 2: 构建后端单二进制**

Run: `cd flymail/backend && go build -o ../bin/flymail.exe .`（或用 `dev.ps1` 菜单项 7 完整构建）
Expected: 生成可执行文件。

- [ ] **Step 3: 端到端联调（人工）**

1. `dev.ps1` 菜单项 10 初始化管理员（若未建）。
2. 菜单项 4 全栈开发（后端 :8080 + 前端 :5390）。
3. 浏览器 `http://localhost:5390` 登录 → 进入三栏。
4. 在 M2 已添加的真实账户上点同步图标 → 观察 sidebar 文件夹出现、INBOX 选中后列表显示真实邮件（中文正常、未读加粗）。
5. 校验：未读/已读视觉区分、点击文件夹切换列表、URL 随选择变化（`?account=&folder=`）。

Expected: 文件夹与 INBOX 邮件正确展示，无乱码，无控制台报错。

- [ ] **Step 4: 提交（如有 lockfile / 配置变化）**

```bash
cd "D:/Develop/workspace/go_dev/MailDev"
git add -A
git commit -m "chore(flymail): M3 端到端联调验证通过"
```

---

## 自检（Self-Review）

**Spec 覆盖：**
- §3 数据模型 Folder/Message → Task 2/6（MessageBody 表 M3 不建，留 M4，符合范围）。✅
- §4 方案 A 首同步抓 ENVELOPE+FLAGS 不抓正文 → Task 8（`FetchBody:false`）。✅
- §4 UIDVALIDITY 变化重建 → Task 8（含 STATUS 兜底）。✅
- §5 API folders.list / messages.list / sync.trigger+status → Task 5/9/11。✅
- §6 三栏 + TanStack Query + URL 驱动 + i18n → Task 14–19。✅
- 范围决策（最近 1000 封 / 后台异步 / 仅 INBOX / 最小三栏）→ Task 8/10/19。✅

**踩坑覆盖：** UIDVALIDITY 兜底（T8）、分批 200（T8）、UTF-7 raw path（T2/T4 用 `info.Path`）、去重 `(folder_id,uid)`（T6/T7）、文件夹稳定排序（T2 SortOrder）。✅

**类型一致性：** `IMAPLister`(folder) / `IMAPFetcher`(message) / `Session`(sync 组合两者+Close) 一致；`FolderState` 字段在 T8 定义、T10 消费一致；`SyncFolderMessages(accountID, folderID, path, prevUIDValidity, fetcher)` 签名在 T8/T10 一致。✅

**已标注的执行期需确认点（非占位，均给了应对）：** `references` 列名是否与 SQLite 关键字冲突（T7 附调整方案）；`AccountResponse.LastSyncAt` 是否已存在（T1 附注）；`api` 导出名（T15 注）；`react-router` v7 `setParams` 函数式签名（T19 注）；lucide 图标名（T17 注）。

---

## 执行交接

计划完成。两种执行方式：
1. **Subagent 驱动（推荐）**：每个 Task 派新 subagent 实现 + 两段式审查，任务间快速迭代。
2. **当前会话内联执行**：用 executing-plans 批量执行 + 检查点。
