# FlyMail M5：写信 / 回复 / 转发 + 草稿（SMTP 发送）实现计划

> 用 superpowers:subagent-driven-development 逐任务执行。每任务 TDD（能测则测）+ gofmt/tsc + 提交。

**Goal:** 写信 / 回复 / 转发（轻量富文本）经 SMTP 发送；发送成功后 IMAP APPEND 到"已发送"；本地草稿保存/编辑/发送。

**Architecture:** core 加 `smtp.SendRaw`（发原始 RFC822）+ `imap.Append`（存已发送）。flymail 新增 `send` 模块（构建合规 RFC 5322：中文主题 RFC2047 编码、Date/Message-ID/MIME 头、回复线程头 In-Reply-To/References、base64 正文；SMTP 发送 + APPEND 已发送）与 `draft` 模块（本地草稿 CRUD + 发送）。前端 ComposeDialog（轻量富文本）+ Reader 回复/转发入口 + 侧栏写信/草稿入口。回复/转发的预填在前端基于邮件详情构造。

**Tech Stack:** core smtp/imap；Go `mime`/`net/mail` 构建消息；前端 react-simple-wysiwyg（轻量富文本）+ TanStack Query。

---

# 后端

## M5-B1：core 加 smtp.SendRaw + imap.Append
**Files:** `core/smtp/smtp.go`、`core/imap/append.go`（新建）；core/mail2im 回归
- `smtp.SendRaw(from string, recipients []string, raw []byte) error`：connect+auth（复用 connect()），MAIL FROM(from)、逐个 RCPT TO(recipients)、DATA 写 raw、关闭。（不自己拼消息，raw 由调用方构建好。）
- `imap.Append(folderPath string, flags []imapv2.Flag, msg []byte) error`：用 go-imap v2 的 Append。**实现者先确认 beta.8 的 Append 签名**（形如 `c.Append(mailbox, size int64, opts *imapv2.AppendOptions) *AppendCommand`，再 `cmd.Write(msg)` + `cmd.Close()`；opts.Flags 设 flags、可设 Time）。`s.Client==nil` 防护。
- 回归：`(cd core && go build ./... )`、`(cd mail2im/backend && go build ./...)`。
- TDD：smtp/imap 这两个需真连服务器，难单测；本任务以编译 + 后续 send 模块的集成验证为主。可加签名级的轻测（nil client 返回错误）。
提交 `feat(core): smtp.SendRaw + imap.Append`。

## M5-B2：account.Service.SMTPConfig
**Files:** `modules/email/account/credentials.go`
- 仿 `IMAPConfig`，加 `SMTPConfig(id uint)(types.SMTPConfig,error)`：取账户、解密密码（及代理），构建 `types.SMTPConfig{Host:SMTPHost,Port:SMTPPort,Username:LoginName(),Password:解密,Security:parseSecurity(SMTPSecurity),Proxy:...}`。
- TDD：SMTPConfig 解密正确（仿 IMAPConfig 测试）。
提交 `feat(flymail): account.SMTPConfig`。

## M5-B3：send 模块（构建消息 + 发送 + 存已发送）
**Files:** `modules/email/send/{builder,service,handler}.go` + 测试；router/app 装配
- `SendRequest{ AccountID uint; To,Cc,Bcc []string; Subject string; BodyHTML string; InReplyTo string; References string }`。
- **builder.go** `BuildRFC5322(from string, req SendRequest, messageID string, date time.Time) ([]byte, error)`：
  - 头：`From`(用 net/mail.Address{Name可空,Address:from}.String())、`To`/`Cc`（地址用 mail.Address 格式化，多个逗号连）、`Subject`（用 `mime.BEncoding.Encode("UTF-8", subject)` 编码中文）、`Date`(date.Format(RFC1123Z))、`Message-ID`(<messageID>)、`MIME-Version: 1.0`、`Content-Type: text/html; charset=UTF-8`、`Content-Transfer-Encoding: base64`，回复时加 `In-Reply-To` 和 `References`。
  - 体：`base64.StdEncoding` 编码 BodyHTML，按 76 字符换行（RFC 2045）。
  - **Bcc 不进头**（只用于 RCPT）。CRLF 行结尾。
  - 纯函数，可单测（断言含 From/To/编码后的 Subject/base64 体/In-Reply-To）。
- **service.go** `Service{ accounts AccountProvider; folders FolderLookup; dial smtp; appendDial imap }`：
  - `Send(req SendRequest) error`：取 account（SMTPConfig + email 作 from）→ 生成 Message-ID（`<uuid@域名>`，域名取 from 的 @ 后）+ now → BuildRFC5322 → `smtp.NewClient(cfg).SendRaw(from, allRecipients, raw)`。发送成功后**尽力 APPEND 到已发送**：找该账户 type=sent 的 folder（folders.FindByType(accountID,"sent")，没有就跳过），用 IMAP（account.IMAPConfig + coreimap.Dial）`Append(sentPath, [\Seen], raw)`；APPEND 失败只 log、不影响 Send 返回成功。
  - 为可测：dial 函数可注入（smtp 发送 + imap append 用接口/函数字段，测试注入 fake 验证 SendRaw 被调用、APPEND 被调用）。
- **handler.go** `POST /send`（受保护）body=SendRequest → svc.Send → 200 `{status:"ok"}`；校验 To 非空。
- folder 需要 `FindByType(accountID, type)(*Folder,error)`（仿 FindInbox，泛化）——加到 folder repo/service。
- 装配 router/app。
- TDD：builder 纯函数测试（中文主题编码、In-Reply-To、base64 体）；service 用 fake smtp/imap 测 Send 调用 SendRaw + APPEND（找到 sent 时）。
提交 `feat(flymail): send 模块（RFC5322 构建 + SMTP 发送 + APPEND 已发送）`。

## M5-B4：draft 模块（本地草稿 CRUD + 发送）
**Files:** `modules/email/draft/{model,repository,service,handler}.go` + 测试；migration、router/app 装配
- `Draft{ ID uint; AccountID uint; ToStr,CcStr,BccStr string; Subject string; BodyHTML string; InReplyTo string; References string; CreatedAt,UpdatedAt }`（收件人存逗号分隔字符串，简单）。TableName "drafts"。
- repo CRUD：Create/Update/GetByID/ListByAccount/Delete。
- service：Create/Update/Get/List/Delete；`Send(draftID, sendSvc *send.Service) error`：取草稿→构造 send.SendRequest→sendSvc.Send→成功后 Delete 草稿。（draft 依赖 send，单向。）
- API（受保护）：`GET /accounts/:id/drafts`、`POST /drafts`、`PUT /drafts/:id`、`DELETE /drafts/:id`、`POST /drafts/:id/send`。
- migration 加 `&draft.Draft{}`；装配。
- TDD：repo CRUD；service Send 调 sendSvc 后删除草稿（fake send）。
提交 `feat(flymail): draft 本地草稿模块（CRUD + 发送）`。

---

# 前端

## M5-F1：编辑器依赖 + 类型/hooks
**Files:** `package.json`(加 react-simple-wysiwyg)；`src/lib/types.ts`、`src/lib/queries.ts`
- `pnpm add react-simple-wysiwyg`（轻量富文本，contenteditable 封装）。
- types：`SendRequest{account_id,to,cc?,bcc?,subject,body_html,in_reply_to?,references?}`（注意 snake_case 与后端 json 对齐——后端 SendRequest 用 json tag account_id/to/cc/bcc/subject/body_html/in_reply_to/references）；`Draft{id,account_id,to,cc,bcc,subject,body_html,in_reply_to,references,...}`（后端用逗号字符串字段 to_str 等？**前后端字段对齐**：draft DTO 用数组还是字符串需一致，实现者让后端 draft DTO 暴露 to/cc/bcc 为字符串或数组并与前端统一，推荐后端 DTO 转成数组）。
- hooks：`useSend()`(POST /send)、`useDrafts(accountId)`、`useCreateDraft()`、`useUpdateDraft()`、`useDeleteDraft()`、`useSendDraft()`。成功后 invalidate drafts、（发送成功）invalidate folders/messages。
- 验证 tsc。提交 `feat(flymail-fe): 写信类型 + 发送/草稿 hooks + 富文本依赖`。

## M5-F2：ComposeDialog（轻量富文本）
**Files:** `src/components/mail/ComposeDialog.tsx`
- props：`{ open; onOpenChange; accountId; initial?: Partial<ComposeState>; draftId?: number }`（initial 用于回复/转发/编辑草稿预填）。
- 字段：收件人 To（必填）、Cc、Bcc（逗号分隔输入）、主题、正文（react-simple-wysiwyg 富文本，输出 HTML）。
- 操作：发送（useSend，To 非空校验，成功关闭 + 提示）、存草稿（useCreateDraft/useUpdateDraft）、取消。
- 账户：默认用当前 accountId（from 即该账户邮箱）；多账户可加发件账户下拉（可选，先用 accountId）。
- i18n compose.*。验证 tsc。提交 `feat(flymail-fe): ComposeDialog 写信对话框（轻量富文本）`。

## M5-F3：回复/转发入口
**Files:** `src/components/mail/Reader.tsx`、`src/pages/Shell.tsx`
- Reader 头部加"回复""转发"按钮（lucide Reply/Forward）。点击回调到 Shell 打开 ComposeDialog，传 initial：
  - 回复：to=[原发件人 from_addr]，subject="Re: "+原主题（已 Re: 不重复加），body=引用原文（`<blockquote>` 包原 html/text + "在 X 写道：" 头），in_reply_to=原 message_id，references=原 references+message_id。
  - 转发：subject="Fwd: "+原主题，body=转发原文（原发件人/收件人/日期/主题 + 原文），to 留空。
  - 预填构造放一个工具函数（reader 或 lib）。需要原邮件的 message_id/references——detail 里有 message_id 吗？**确认 MessageDetail 是否含 message_id/references**；message 表有 MessageID，但 MessageListItem/Detail DTO 可能没暴露——若没有，后端 MessageDetail DTO 补 `message_id`、`in_reply_to`、`references` 字段（来自 Message 行）。实现者据此补 DTO。
- Shell 管理 ComposeDialog 状态 + initial。验证 tsc。提交 `feat(flymail-fe): 回复/转发入口 + 预填`。

## M5-F4：写信入口 + 草稿箱
**Files:** `src/components/mail/AccountSidebar.tsx`、`src/pages/Shell.tsx`、`src/components/mail/DraftsList.tsx`(可选)
- 侧栏顶部加"写邮件"按钮（lucide Pencil/Edit，t('sidebar.compose')）→ 打开空 ComposeDialog（当前账户）。
- 草稿：侧栏账户下加"草稿箱(本地)"入口 → 点击在中栏或弹窗列出本地草稿（useDrafts），点草稿 → ComposeDialog 编辑（draftId + initial）。最小实现：草稿用一个列表（可复用 MailList 风格或简单列表）。
- 验证 tsc + build。提交 `feat(flymail-fe): 写信入口 + 本地草稿箱`。

## M5-F5：构建 + 验证（主控）
- pnpm build + go build。
- admin/admin + 真实账户（用户真机）：写信发给自己 → 收件箱能收到（同步后）；已发送文件夹出现该信（APPEND 生效）；回复/转发预填正确、能发出；存草稿→重开→编辑→发送→草稿消失。
- 无头烟雾（无真实 SMTP 时）：ComposeDialog 打开/校验/草稿存取的 UI 流程。

---

## 自检
- core：SendRaw/Append + 回归。flymail：SMTPConfig、send（builder 纯函数可测 + service fake 测）、draft CRUD。前端：富文本、Compose、回复转发预填、写信/草稿入口。
- 关键正确性：中文主题 RFC2047 编码；回复 In-Reply-To/References 线程；Bcc 不进头只进 RCPT；base64 正文 76 列换行 CRLF；APPEND 失败不影响发送成功。
- DTO 对齐：MessageDetail 补 message_id/in_reply_to/references（供回复）；draft DTO 收件人字段前后端一致。
- 附件本期不做（M7）。IMAP Drafts 同步不做（仅本地草稿）。
