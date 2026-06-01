# FlyMail 设置中心 + 账户管理增强 实现计划

> 用 superpowers:subagent-driven-development 逐任务执行。每任务 TDD（能测则测）+ gofmt/tsc + 提交。

**Goal:** 模态左右分栏设置中心，含四个分区：账户管理（整合 + 状态/启用停用/手动同步）、通用（语言+主题）、安全（改管理员密码）、邮件（同步偏好）。

**Architecture:** 后端新增 Setting 键值存储 + 改密 API + Account.enabled 字段 + 账户 stats；sync_depth 设置接入同步。前端新增 SettingsDialog（模态+左侧导航）+ 暗色主题变量 + 各分区，复用既有 AccountDialog/sync hooks。

---

# 后端

## B1：Setting 键值模型 + 仓储 + 服务 + API
**Files:** Create `modules/system/setting/{model,repository,service,handler}.go` + 测试；Modify `internal/database/database.go`、`internal/server/router.go`、`internal/app/app.go`

- `Setting{ Key string gorm:"primaryKey"; Value string; UpdatedAt time.Time }`，TableName "settings"。
- Repository：`Get(key)(string,bool,error)`、`Set(key,value)error`(upsert OnConflict key)、`All()(map[string]string,error)`。
- Service：`GetInt(key string, def int) int`、`GetString(key, def)`、`SetMany(map[string]string)error`、`All()map[string]string`。定义已知键常量：`KeySyncDepth = "sync_depth"`（默认 1000）。
- API（受保护）：`GET /settings` → `{settings: {key:value...}}`（只回已知键，带默认值）；`PUT /settings` body `{settings:{key:value}}` → 校验+保存（sync_depth 必须为正整数，范围 100..5000）。
- Migrate 追加 `&setting.Setting{}`；app 装配 settingSvc；router 挂 `setting.RegisterRoutes(protected, settingSvc)`。
- TDD：repo upsert/all；service GetInt 默认值/越界。
提交 `feat(flymail): Setting 键值存储 + /settings API`。

## B2：sync_depth 接入同步
**Files:** Modify `modules/email/message/service.go`、`modules/email/sync/service.go`、`internal/app/app.go`
- message：把 `defaultSyncDepth` 常量改为可配置——`SyncFolderMessages` 增加 `depth int` 参数（或 Service 持有 depth 字段）。最小改动：给 message.Service 加字段 `syncDepth int`，`NewService(repo, bodyRepo)` 默认 1000，新增 `SetSyncDepth(int)`；SyncFolderMessages 内用 `s.syncDepth`（<=0 回退 1000）。
- sync.Service.run 触发同步前，从 setting 读 sync_depth 设给 message.Service（或 sync 持有 settingSvc，run 时 `s.messages.SetSyncDepth(settingSvc.GetInt(KeySyncDepth,1000))`）。给 sync.NewService 增加 settingSvc 参数（或可选 setter）。app 装配传入。
- 注意更新 sync_test / message_test 对 NewService 的调用（若签名变）。优先用"加 setter 不改构造器签名"以减小波及：message 加 `SetSyncDepth`；sync 加 `SetSettings(getter)` 或在 run 内调用注入的回调。**实现者选波及最小的方式并说明。**
提交 `feat(flymail): 同步深度接入 sync_depth 设置`。

## B3：修改管理员密码 API
**Files:** Modify `modules/auth/{handler,service}.go`；先读 `modules/auth/middleware.go` 确认登录用户名如何放进 gin.Context。
- auth.Service 加 `ChangePassword(username, oldPassword, newPassword) error`：先 `Authenticate(username, old)`（失败返回 ErrInvalidCredentials）→ `SetAdminPassword(username, new)`。
- handler：`POST /auth/change-password`（**在受保护组**，需要登录）body `{old_password, new_password}`。用户名从鉴权中间件放入 context 的值取（读 middleware 确认 key；若没有，则单管理员场景可从 token claims 或直接查唯一 admin——优先用 context 里的 username）。new_password 非空校验。成功 200 `{status:"ok"}`，旧密码错 401。
- 路由：change-password 必须挂在 protected 组（auth.RegisterRoutes 目前在 public 组挂 login/refresh/logout；change-password 需要 auth.Middleware）。实现者在 router.go 的 protected 组单独挂 `auth.RegisterProtectedRoutes(protected, svc)` 或在 account 同级挂。给出清晰实现。
- TDD：ChangePassword 旧密码对/错。
提交 `feat(flymail): 修改管理员密码 API（验旧密码）`。

## B4：Account.enabled + 启用/停用 + 同步跳过
**Files:** Modify `modules/email/account/{model,repository,service,handler,dto}.go`、`modules/email/sync/service.go`
- Account 加 `Enabled bool gorm:"not null;default:true"`（注意 gorm default:true + bool 零值坑——参考 folder Selectable：去 default、Create 时显式置 true。Account 创建走 service，确保新账户 Enabled=true）。AutoMigrate 自动加列（已有账户列默认... SQLite 加列 NOT NULL 需默认值；用 `default:true` 让旧行得 true，或迁移后手动 update。实现者验证已有账户迁移后 enabled 为 true）。
- AccountResponse 加 `enabled`。toResponse 带上。
- 端点：`POST /accounts/:id/enabled` body `{enabled:bool}` → service.SetEnabled(id,bool)。
- sync.Trigger：若账户 disabled，返回错误或忽略（manual sync 可允许；但 disabled 主要给未来自动同步用）。本期：Trigger 前查账户 enabled，disabled 则返回 ErrAccountDisabled（前端不显示同步按钮或提示）。account.Service 加 `IsEnabled(id)(bool,error)` 或 sync 通过 accounts provider 取。简化：AccountConfigProvider 加 `IsEnabled(id)(bool,error)`，sync.Trigger 检查。
- TDD：SetEnabled；新账户默认 enabled=true。
提交 `feat(flymail): 账户启用/停用（enabled 字段 + 端点 + 同步跳过）`。

## B5：账户 stats
**Files:** Modify `modules/email/account/{handler}.go` + 依赖 message 计数
- `GET /accounts/:id/stats` → `{message_count, folder_count}`。message_count = 该账户所有 messages 数；folder_count = folders 数。
- 实现：account handler 需要 message/folder 的计数能力。为避免 account→message 依赖，给 account.Service 注入计数函数，或新建一个轻量 stats 端点放在 sync handler（sync 已依赖 message/folder）。**推荐放 sync handler**：`GET /accounts/:id/stats`，用 message repo CountByAccount + folder repo CountByAccount。需在 message/folder repo 加 `CountByAccount(accountID)`。
- TDD：CountByAccount。
提交 `feat(flymail): 账户 stats（邮件/文件夹数）`。

---

# 前端

## F1：暗色主题变量 + 主题切换基础
**Files:** Modify `src/index.css`；Create `src/lib/theme.ts`
- index.css 的 `.dark` 块补 FlyMail 变量暗色版（--bg/--bg-alt/--surface/--ink/--ink-2/--ink-3/--rule/--bg-hover/--accent-color/--accent-wash），参考 MailMaster 暖色暗色（如 --bg:#1f1d18, --surface:#26231d, --ink:#ece7dc, --accent-color:#d2a482 等）。
- `theme.ts`：`getTheme():'light'|'dark'`（读 localStorage 'flymail_theme'，默认 light）、`applyTheme(t)`（toggle document.documentElement.classList 'dark' + 存 localStorage）。main.tsx 启动时 applyTheme(getTheme())。
- 验证 tsc + build。提交 `feat(flymail-fe): 暗色主题变量 + theme 工具`。

## F2：设置相关类型 + hooks
**Files:** Modify `src/lib/types.ts`、`src/lib/queries.ts`
- types：`AppSettings { sync_depth: number }`、`AccountStats { message_count: number; folder_count: number }`；Account 加 `enabled: boolean`。
- hooks：`useSettings()`(GET /settings → settings)、`useUpdateSettings()`(PUT)、`useChangePassword()`(POST /auth/change-password)、`useSetAccountEnabled()`(POST /accounts/:id/enabled，invalidate accounts)、`useAccountStats(id)`(GET /accounts/:id/stats)。
- 验证 tsc。提交 `feat(flymail-fe): 设置/改密/启停/stats 类型与 hooks`。

## F3：SettingsDialog 外壳（模态 + 左侧导航）
**Files:** Create `src/components/settings/SettingsDialog.tsx`
- radix Dialog；左侧导航(账户/通用/安全/邮件)，右侧渲染对应 section（用本地 state 切换）。props `{open, onOpenChange}`。文案 i18n（settings.*）。各 section 子组件先占位，F4–F7 填充。
- i18n：zh/en 加 settings 段（title/accounts/general/security/mail + 各 section 文案，随 F4–F7 补充）。
- 验证 tsc。提交 `feat(flymail-fe): SettingsDialog 外壳`。

## F4：账户 section（列表+状态+启停+同步+增删改）
**Files:** Create `src/components/settings/AccountsSection.tsx`
- 账户列表：每行显示 名称/邮箱、状态(status)、最后同步(last_sync_at 格式化)、邮件数(useAccountStats)、enabled 开关、编辑/删除、"立即同步"按钮(复用 useTriggerSync + useSyncStatus 显示进度)。
- 顶部"添加账户"→ 复用 `AccountDialog`。编辑→AccountDialog(account)。删除→确认。
- enabled 开关 → useSetAccountEnabled。
- 验证 tsc。提交 `feat(flymail-fe): 设置-账户管理 section`。

## F5：通用 section（语言 + 主题）
**Files:** Create `src/components/settings/GeneralSection.tsx`
- 语言：select(中文/English) → i18n.changeLanguage + 存 localStorage 'flymail_lang'（main.tsx 启动读取设 lng）。
- 主题：select/toggle(亮/暗) → applyTheme。
- 验证 tsc。提交 `feat(flymail-fe): 设置-通用（语言+主题）`。

## F6：安全 section（改管理员密码）
**Files:** Create `src/components/settings/SecuritySection.tsx`
- 表单：旧密码/新密码/确认新密码 → useChangePassword；成功提示、旧密码错提示；新密码与确认不一致校验。
- 验证 tsc。提交 `feat(flymail-fe): 设置-安全（改密码）`。

## F7：邮件 section（同步偏好）
**Files:** Create `src/components/settings/MailSection.tsx`
- sync_depth 输入(数字，100..5000) → useSettings/useUpdateSettings。
- "默认加载远程图片"开关 → localStorage 'flymail_load_remote_images'（Reader 读取它决定默认 showImages）。
- 验证 tsc。提交 `feat(flymail-fe): 设置-邮件偏好`。

## F8：设置入口接线 + 构建验证
**Files:** Modify `src/components/mail/AccountSidebar.tsx`（底部加齿轮"设置"入口）、`src/pages/Shell.tsx`（管理 SettingsDialog open 状态 + 渲染）；Reader 读 load_remote_images 默认值。
- 侧栏底部加设置齿轮按钮 → 打开 SettingsDialog。账户的增删改可保留侧栏快捷入口或移交设置（保留侧栏"+"快捷添加即可）。
- 构建 + 无头浏览器/真机验证：打开设置 → 四个分区可用；切语言/主题即时生效；改密码（用 admin/admin 测，改完用新密码登录）；账户启停；同步深度保存后重新同步生效。
- 提交。

---

## 自检
- 后端新增：Setting/API、改密 API、Account.enabled、stats、sync_depth 接入。前端：暗色主题、设置中心四分区、入口。
- 注意：gorm bool default 坑（enabled，参考 folder.Selectable 处理）；旧账户迁移后 enabled=true；change-password 必须在受保护组；sync_depth 范围校验。
- i18n：settings.* zh/en 对齐，JSON 合法无重复 key。
- 已有账户管理（AccountDialog/CRUD/预设）复用，不重写。
