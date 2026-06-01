# FlyMail M4：正文按需 + 阅读 + 已读/星标回写 实现计划

> **For agentic workers:** 用 superpowers:subagent-driven-development 逐任务执行。每任务 TDD（能测则测）+ gofmt + 提交。

**Goal:** 点击邮件 → 按需抓取正文（首次 dial+FetchBody 落库，之后读本地）→ Reader 沙箱 iframe 渲染（默认不加载远程图）→ 打开自动标已读 + 星标切换，标记本地先改、异步回写 IMAP。附件仅存元数据 + 回填 has_attachment（不下载，下载留 M7）。**删除/移动本期不做**（core 仅永久 expunge、无 Trash MOVE，留后续）。

**Architecture:** message 模块只管持久化（模型/仓储/列表 + 存正文/附件/标记的本地方法）。**sync 模块作为实时 IMAP 操作编排层**（已有 account 配置 + folder + dial），新增：正文按需抓取、标记回写（本地先改 + 异步 IMAP 回写 + 重试）、及对应 HTTP 端点。避免 message↔sync 循环依赖（sync→message 单向）。

**Tech Stack:** Go / gin / gorm；core imap（FetchByUIDs FetchBody / MarkRead / MarkStarred）；前端 React + 沙箱 iframe + DOMPurify(可选) + TanStack Query。

---

## core 能力（已确认，直接用）
- `(*coreimap.Session).FetchByUIDs(uids []imapv2.UID, opts coreimap.FetchOptions)([]*types.ParsedEmail,error)`，`FetchOptions{FetchBody:true, FallbackHeaders:true}` → ParsedEmail 填 `TextBody/HTMLBody/Attachments`。
- `Attachment{Filename, ContentType, Size int64, ContentID, IsInline bool}`（core/types）。
- `MarkRead/MarkUnread/MarkStarred/MarkUnstarred(uids ...imapv2.UID) error`、`Delete(...)`（**永久 expunge，本期不用**）。均操作**当前选中文件夹**，故回写须先 `SelectFolder(path)`。

---

# 后端

## Task 1：MessageBody + Attachment 模型 + 迁移
**Files:** Create `modules/email/message/body_model.go`；Modify `internal/database/database.go`

```go
// body_model.go
package message
import "time"

// MessageBody 单独分表，避免列表查询拉大正文。
type MessageBody struct {
	ID        uint   `gorm:"primaryKey" json:"-"`
	MessageID uint   `gorm:"uniqueIndex;not null" json:"-"`
	TextBody  string `json:"text_body"`
	HTMLBody  string `json:"html_body"`
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}
func (MessageBody) TableName() string { return "message_bodies" }

type Attachment struct {
	ID          uint   `gorm:"primaryKey" json:"-"`
	MessageID   uint   `gorm:"index;not null" json:"-"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	ContentID   string `json:"content_id,omitempty"`
	IsInline    bool   `json:"is_inline"`
}
func (Attachment) TableName() string { return "attachments" }
```
Migrate 追加 `&message.MessageBody{}`, `&message.Attachment{}`。
验证 `go build ./...`。提交 `feat(flymail): MessageBody + Attachment 模型 + 迁移`。

## Task 2：仓储扩展（body/attachment + message 标记/取详情）
**Files:** Create `modules/email/message/body_repository.go`；Modify `modules/email/message/repository.go`

`body_repository.go`：
```go
package message
import "gorm.io/gorm/clause"

type BodyRepository struct{ db *gorm.DB }
func NewBodyRepository(db *gorm.DB) *BodyRepository { return &BodyRepository{db: db} }

func (r *BodyRepository) Upsert(b *MessageBody) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "message_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"text_body", "html_body", "updated_at"}),
	}).Create(b).Error
}
func (r *BodyRepository) GetByMessageID(messageID uint) (*MessageBody, error) { /* First; ErrRecordNotFound→(nil,nil) */ }

func (r *BodyRepository) ReplaceAttachments(messageID uint, atts []Attachment) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("message_id = ?", messageID).Delete(&Attachment{}).Error; err != nil { return err }
		if len(atts) == 0 { return nil }
		return tx.Create(&atts).Error
	})
}
func (r *BodyRepository) ListAttachments(messageID uint) ([]Attachment, error) { /* Find where message_id */ }
```
（注：`BodyRepository` 也用同一 `*gorm.DB`；import errors/gorm。GetByMessageID 未找到返回 (nil,nil)。）

`repository.go` 给 `Repository` 加：
```go
func (r *Repository) GetByID(id uint) (*Message, error)            // ErrMessageNotFound
func (r *Repository) SetSeen(id uint, seen bool) error             // Update seen
func (r *Repository) SetFlagged(id uint, flagged bool) error
func (r *Repository) MarkBodySynced(id uint, snippet string, hasAttachment bool) error // 置 body_synced=true + snippet + has_attachment
```
新增 `var ErrMessageNotFound = errors.New("message not found")`。
TDD：`body_repository_test.go` 测 Upsert 幂等、ReplaceAttachments 覆盖、GetByID、SetSeen/SetFlagged、MarkBodySynced。提交 `feat(flymail): Message 正文/附件仓储 + 标记方法`。

## Task 3：message.Service 持久化方法（存正文 + 组装详情）
**Files:** Modify `modules/email/message/service.go`（构造器加 bodyRepo）；可能新增 dto 字段

把 `NewService(repo *Repository)` 改为 `NewService(repo *Repository, bodyRepo *BodyRepository)`，并更新所有调用点（app.go、sync_test、message tests——本任务一并改）。Service 加：
```go
// StoreParsedBody 把抓到的正文/附件落库并回填元信息（snippet 取 TextBody 去空白前 150 字，无 text 用 HTML 提取）。
func (s *Service) StoreParsedBody(messageID uint, e *types.ParsedEmail) error
// Detail 从本地组装详情（含 body + attachments）。
func (s *Service) Detail(messageID uint) (*MessageDetail, error)
// GetByID 暴露 message 行（sync 编排用）。
func (s *Service) GetByID(id uint) (*Message, error)
// 标记本地（回写由 sync 负责）。
func (s *Service) SetSeenLocal(id uint, seen bool) error
func (s *Service) SetFlaggedLocal(id uint, flagged bool) error
```
`MessageDetail` DTO（dto.go）：含 list 字段 + `To/Cc []types.Address`、`text_body`、`html_body`、`attachments []Attachment`、`body_synced bool`。`snippet` 生成函数 `makeSnippet(text, html string) string`（参考旧 flymail generatePreview：去换行/多空格、截 150）。
TDD：service_test 测 StoreParsedBody（落 body+附件+snippet+has_attachment）、Detail 组装。提交 `feat(flymail): Message 服务存正文 + 组装详情 + 本地标记`。

## Task 4：sync 编排——正文按需 + 标记回写
**Files:** Modify `modules/email/sync/service.go`；Create `modules/email/sync/writeback.go`

扩展 `Session` 接口（加 FetchByUIDs + 四个 Mark 方法）：
```go
type Session interface {
	folder.IMAPLister
	message.IMAPFetcher
	FetchByUIDs(uids []imapv2.UID, opts coreimap.FetchOptions) ([]*types.ParsedEmail, error)
	MarkRead(uids ...imapv2.UID) error
	MarkUnread(uids ...imapv2.UID) error
	MarkStarred(uids ...imapv2.UID) error
	MarkUnstarred(uids ...imapv2.UID) error
	Close() error
}
```
sync.Service 需要 folder.GetByID（已存在 folder.Service.GetByID）。新增方法：
```go
// MessageDetail：本地无正文则 dial+select+FetchByUIDs(FetchBody)+落库，再返回本地详情。
func (s *Service) MessageDetail(messageID uint) (*message.MessageDetail, error) {
	m, err := s.messages.GetByID(messageID)
	if err != nil { return nil, err }
	if !m.BodySynced {
		f, err := s.folders.GetByID(m.FolderID)
		if err != nil { return nil, err }
		cfg, err := s.accounts.IMAPConfig(m.AccountID)
		if err != nil { return nil, err }
		sess, err := s.dial(cfg)
		if err != nil { return nil, err }
		defer sess.Close()
		if _, err := sess.SelectFolder(f.Path); err != nil { return nil, err }
		emails, err := sess.FetchByUIDs([]imapv2.UID{imapv2.UID(m.UID)}, coreimap.FetchOptions{FetchBody: true, FallbackHeaders: true})
		if err != nil { return nil, err }
		if len(emails) > 0 {
			if err := s.messages.StoreParsedBody(messageID, emails[0]); err != nil { return nil, err }
		}
	}
	return s.messages.Detail(messageID)
}

// SetRead / SetFlagged：本地先改，异步回写 IMAP（失败重试）。
func (s *Service) SetRead(messageID uint, read bool) error {
	if err := s.messages.SetSeenLocal(messageID, read); err != nil { return err }
	m, err := s.messages.GetByID(messageID)
	if err != nil { return err }
	s.enqueueWriteback(m.AccountID, m.FolderID, m.UID, flagOp{seen: &read})
	return nil
}
func (s *Service) SetFlagged(messageID uint, flagged bool) error { /* 同理，flagOp{flagged:&flagged} */ }
```
`writeback.go`：一个内存队列 + 单 goroutine worker（启动于 NewService）。任务 `{accountID, folderID, uid, op}`，处理：folder.GetByID→path、accounts.IMAPConfig→dial→SelectFolder→MarkRead/Unread/Starred/Unstarred；失败按指数退避重试（上限 N 次）后丢弃并 log。可注入 dial（复用 s.dial）。并发安全用 channel。
> 说明：worker 用独立连接，不与同步抢 per-account 锁（IMAP 允许多连接）。`flagOp{seen *bool; flagged *bool}` 区分操作。
TDD：用 fakeSession 记录调用，测 SetRead 本地置位 + worker 最终调用 MarkRead（用同步等待或可注入的"立即执行"模式测试）；MessageDetail 在 body_synced=false 时触发 FetchByUIDs 并落库。**更新 sync_test 的 fakeSession 补全新接口方法。**
提交 `feat(flymail): sync 正文按需抓取 + 已读/星标异步回写`。

## Task 5：HTTP 端点（详情 + 标记）
**Files:** Modify `modules/email/sync/handler.go`

加路由（仍在受保护组）：
```go
rg.GET("/messages/:id", h.detail)
rg.POST("/messages/:id/read", h.markRead)     // body {"read": true|false}
rg.POST("/messages/:id/flag", h.markFlag)     // body {"flagged": true|false}
```
- detail：parse id → `svc.MessageDetail(id)` → 200 detail；未找到 404；IMAP 失败 502 `{"error":...}`。
- markRead/markFlag：解析 body bool → `svc.SetRead/SetFlagged` → 200 `{"status":"ok"}`。
提交 `feat(flymail): 邮件详情 + 已读/星标 路由`。

## Task 6：装配
**Files:** Modify `internal/app/app.go`（messageSvc 构造加 bodyRepo；sync 不变签名）；确认 router 已挂 sync。
`messageSvc := message.NewService(message.NewRepository(db), message.NewBodyRepository(db))`。
全量 `go build ./... && go test ./...` 通过。提交 `feat(flymail): 装配 M4（正文仓储注入）`。

---

# 前端

## Task B1：类型 + Query hooks
**Files:** Modify `src/lib/types.ts`、`src/lib/queries.ts`
types 加：
```ts
export interface Attachment { filename: string; content_type: string; size: number; content_id?: string; is_inline: boolean }
export interface MessageDetail extends MessageListItem {
  to: Address[]; cc?: Address[]; text_body: string; html_body: string; attachments: Attachment[]; body_synced: boolean
}
```
queries 加：
```ts
export function useMessageDetail(messageId: number | null) {
  return useQuery({ queryKey:['message', messageId], enabled: messageId!=null,
    queryFn: async (): Promise<MessageDetail> => (await api.get<MessageDetail>(`/messages/${messageId}`)).data })
}
export function useMarkRead() { /* mutation POST /messages/:id/read {read}; onSuccess 失效 ['messages'],['folders'],['message',id] */ }
export function useToggleFlag() { /* mutation POST /messages/:id/flag {flagged}; onSuccess 同上 */ }
```
验证 tsc。提交 `feat(flymail-fe): 邮件详情类型 + hooks`。

## Task B2：Reader 详情 + 沙箱 iframe
**Files:** Rewrite `src/components/mail/Reader.tsx`
- props：`messageId: number | null`。messageId==null → 欢迎占位（保留现状）。
- 用 `useMessageDetail(messageId)`，loading 显示加载态。
- 头部：主题（大字）、发件人 from_name<from_addr>、收件人 to、日期；右侧星标按钮（点击 `useToggleFlag`）。
- 正文：优先 html_body → **sandboxed iframe**：`<iframe sandbox srcDoc={html}>`，sandbox 不含 allow-scripts（禁 JS）。**默认移除/不加载远程图片**：对 html 做简单处理——把 `src=` 的远程图替换为占位 + 顶部"显示远程内容"按钮，点击后用原始 html 重渲染。无 html 用 text_body（`<pre>` 或 whitespace-pre-wrap）。
  > 远程图处理：可用正则把 `<img ... src="http...">` 的 src 暂存到 data 属性；点击"显示图片"后还原。或最简：默认 srcDoc 注入 `<meta>` CSP/不处理但提供开关——MVP 用正则替换 http(s) 图片 src 为空 + 计数，开关切换。实现者择简实现并说明。
- 附件：若 attachments 非空，列出文件名 + 大小（lucide Paperclip），**不可下载**（M7），可加禁用态或"暂不支持下载"提示。
- 文案走 i18n（reader.* 扩展：from/to/showImages/attachments/loading 等）。
验证 tsc + build。提交 `feat(flymail-fe): Reader 邮件详情（沙箱 iframe + 远程图开关 + 附件元数据）`。

## Task B3：Shell 接线——选中加载详情 + 自动标已读 + 列表星标
**Files:** Modify `src/pages/Shell.tsx`、`src/components/mail/MailList.tsx`
- Shell：把 `messageId` 传给 `<Reader messageId={messageId} />`。新增 effect：messageId 变化且该邮件未读 → `useMarkRead().mutate({id, read:true})`（自动标已读；本地+回写）。注意避免重复触发（只在打开未读时）。
- MailList：邮件项加星标小图标（lucide Star，已星标实心），点击 `e.stopPropagation()` + `useToggleFlag().mutate({id, flagged:!flagged})`。
- 缓存：标记成功失效 messages/folders/message。
验证 tsc + build。提交 `feat(flymail-fe): 选中邮件加载详情 + 自动已读 + 列表星标`。

## Task B4：构建 + 真机验证（主控）
- pnpm build + go build。
- admin/admin + 无头浏览器（或用户真机）：打开已同步账户 → 点 INBOX 邮件 → 详情正文渲染（HTML 走 iframe、远程图默认不加载）→ 列表该邮件变已读 → 星标切换生效 → 重开邮件秒出（本地缓存）。检查无控制台错误、回写 IMAP 不报错。

---

## 自检
- 范围：正文按需(T3/T4)、详情 API(T5)、已读/星标本地+异步回写(T4)、附件元数据(T1/T3)、沙箱 iframe+远程图开关(B2)、自动已读(B3)。✅
- 删除/移动**明确不做**（core 仅 expunge），单列后续。
- 循环依赖：sync→message 单向；详情/标记端点在 sync handler，列表在 message handler。
- 回写：本地先改、异步 worker 回写 + 重试；独立连接不与同步抢锁。
- 类型一致：sync.Session 扩展后 fakeSession 同步补全；message.NewService 加 bodyRepo 后所有调用点更新。
