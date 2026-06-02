# FlyMail M7 附件下载 + 预览 + 内联图 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 邮件附件可按需从 IMAP 抓取并下载/浏览器内预览（图片/PDF），正文内联图（cid:）正常渲染。

**Architecture:** 按需抓取——下载/预览时后端重新 `FetchRawMessage(uid)` 取整封原始 RFC822 → `parser.ExtractAttachments` 解析出指定附件字节流出，不落本地磁盘。`core/parser` 重构为共享 walker：`ParseBody`（展示用，仅元数据，签名不变以兼容 mail2im）与 `ExtractAttachments`（下载用，带内容字节）走同一套部件遍历，保证附件**顺序一致**（前端按 MessageDetail.attachments 的数组下标引用，后端按同下标取）。内联图：parser 把非文本的 InlineHeader 也纳入附件列表（is_inline=true + content_id），前端把正文 HTML 中的 `cid:` 改写为附件接口 URL。鉴权：附件接口同时接受 `Authorization` 头（用户主动下载走 axios blob）或 `?access_token=` query（img/iframe/预览新标签，沿用 KI-2 模式）。

**Tech Stack:** go-message/mail 部件遍历、go-imap FETCH BODY[]、gin 流式响应 + Content-Disposition(RFC5987)、前端 axios blob 下载 + iframe srcDoc cid 改写。

---

## 文件结构

**core:**
- 修改 `core/types/email.go` — `Attachment` 加 `Content []byte` 字段（`json:"-"`，不进 API/DB；仅 ExtractAttachments 填充）
- 修改 `core/parser/parser.go` — 抽出共享 `walkParts`；`ParseBody` 复用之（含内联图纳入元数据）；新增 `ExtractAttachments`
- 新建 `core/parser/attachments_test.go` — ExtractAttachments + 内联图 + 顺序一致性测试
- 修改 `core/imap/fetch.go` — 新增 `FetchRawMessage(uid) ([]byte, error)`

**flymail 后端:**
- 修改 `modules/email/sync/service.go` — `Session` 接口加 `FetchRawMessage`；新增 `AttachmentContent(messageID, idx)`
- 修改 `modules/email/sync/handler.go` — `GET /messages/:id/attachments/:idx`（dual auth + 流式 + Content-Disposition）
- 修改 `internal/server/router.go` — 附件路由挂在 `/api/v1` 下（不走 Bearer 中间件，自带 dual auth）；`Deps` 传入 auth 校验函数
- 修改 `modules/email/sync/service_test.go` — fakeSession 补 `FetchRawMessage`；AttachmentContent 测试

**flymail 前端:**
- 修改 `src/lib/types.ts` — Attachment 已有 filename/content_type/size/content_id/is_inline（确认）
- 修改 `src/lib/attachments.ts`（新建）— 构建附件 URL（带 token）、cid→idx 改写 HTML、判断可预览类型、blob 下载
- 修改 `src/components/mail/Reader.tsx` — 正文 cid 改写 + 附件列表（过滤 is_inline）下载/预览按钮
- 修改 `src/locales/zh.json` + `en.json` — 下载/预览文案

**文档:**
- 修改 `docs/superpowers/known-issues.md` — KI-2 补充附件接口 token-in-URL

---

## Phase A：core 解析层

### Task A1: types.Attachment 加 Content

**Files:** Modify `core/types/email.go`

- [ ] **Step 1: 加字段**（不破坏现有 JSON）

```go
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	ContentID   string `json:"content_id,omitempty"` // for inline attachments (CID)
	IsInline    bool   `json:"is_inline"`
	Content     []byte `json:"-"` // 附件内容字节，仅 ExtractAttachments 填充；不序列化、不入库
}
```

- [ ] **Step 2: 编译** `cd core && go build ./...`，预期通过（加字段向后兼容，mail2im 不受影响）。
- [ ] **Step 3: 提交** `feat(core): types.Attachment 加 Content 字段（json:"-"）`

### Task A2: parser 共享 walker + ExtractAttachments + 内联图

**Files:** Modify `core/parser/parser.go`；Test `core/parser/attachments_test.go`

设计：抽出 `walkParts(r, captureContent bool)` 返回 text/html 与有序附件切片；`ParseBody` 与 `ExtractAttachments` 都复用它，保证顺序一致。内联图（非文本 InlineHeader）纳入附件（IsInline=true）。`captureContent=false` 时附件只算 Size 不留 Content（展示路径省内存）。

- [ ] **Step 1: 写失败测试** `attachments_test.go`

构造一封 multipart 邮件（含 1 个 text/plain、1 个 text/html、1 个内联 image/png 带 Content-Id、1 个普通 application/pdf 附件）。断言：
1. `ExtractAttachments` 返回 2 个元素（内联 png + pdf），顺序为 [png, pdf]，png.IsInline=true 且 ContentID 正确、Content 非空且等于原始字节；pdf.IsInline=false、Content 非空。
2. `ParseBody` 后 `email.Attachments` 同样 2 个、同序、同 IsInline/ContentID，但 Content 为 nil（不留内容），Size>0。
3. 顺序一致性：两者的 (Filename, ContentID, IsInline) 序列逐一相等。

```go
package parser

// 用 go-message 或手写 MIME 文本构造测试邮件；断言如上。
```

- [ ] **Step 2: 跑测试确认失败** `cd core && go test ./parser/ -run TestExtractAttachments -v`，预期 FAIL（未定义）。

- [ ] **Step 3: 重构实现**

```go
// AttachmentData 是带内容的附件（下载用）。
type AttachmentData struct {
	Filename    string
	ContentType string
	ContentID   string
	IsInline    bool
	Size        int64
	Content     []byte
}

// walkParts 遍历 MIME 部件：返回 text/html 正文与「有序」附件列表。
// captureContent=true 时读取附件字节到 Content，否则只统计 Size（节省内存）。
// 内联图（非文本的 InlineHeader）也作为附件返回（IsInline=true）。
func walkParts(mr *mail.Reader, captureContent bool) (text, html string, atts []AttachmentData) {
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			ct, _, _ := h.ContentType()
			if strings.HasPrefix(ct, "text/plain") || strings.HasPrefix(ct, "text/html") {
				b, readErr := io.ReadAll(p.Body)
				if readErr != nil {
					continue
				}
				if strings.HasPrefix(ct, "text/plain") && text == "" {
					text = string(b)
				} else if strings.HasPrefix(ct, "text/html") && html == "" {
					html = string(b)
				}
				continue
			}
			// 非文本内联部件（内联图等）→ 作为内联附件
			fn, _ := h.Filename()
			cid := strings.Trim(h.Get("Content-Id"), "<>")
			atts = append(atts, readAttachment(p.Body, fn, ct, cid, true, captureContent))
		case *mail.AttachmentHeader:
			fn, _ := h.Filename()
			ct, _, _ := h.ContentType()
			cid := strings.Trim(h.Get("Content-Id"), "<>")
			atts = append(atts, readAttachment(p.Body, fn, ct, cid, false, captureContent))
		}
	}
	return text, html, atts
}

func readAttachment(body io.Reader, filename, ct, cid string, inline, capture bool) AttachmentData {
	a := AttachmentData{Filename: filename, ContentType: ct, ContentID: cid, IsInline: inline}
	if capture {
		b, _ := io.ReadAll(body)
		a.Content = b
		a.Size = int64(len(b))
	} else {
		a.Size, _ = io.Copy(io.Discard, body)
	}
	return a
}

// ParseBody 复用 walkParts（仅元数据，不留 Content），签名保持不变。
func ParseBody(r io.Reader, email *types.ParsedEmail, fallbackHeaders bool) error {
	mr, err := mail.CreateReader(r)
	if err != nil {
		return err
	}
	if fallbackHeaders {
		fillFromHeaders(mr, email)
	}
	text, html, atts := walkParts(mr, false)
	if email.TextBody == "" {
		email.TextBody = text
	}
	if email.HTMLBody == "" {
		email.HTMLBody = html
	}
	for _, a := range atts {
		email.Attachments = append(email.Attachments, types.Attachment{
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Size:        a.Size,
			ContentID:   a.ContentID,
			IsInline:    a.IsInline,
		})
	}
	return nil
}

// ExtractAttachments 解析原始 RFC822，返回带内容的有序附件（含内联图）。下载/预览用。
func ExtractAttachments(r io.Reader) ([]AttachmentData, error) {
	mr, err := mail.CreateReader(r)
	if err != nil {
		return nil, err
	}
	_, _, atts := walkParts(mr, true)
	return atts, nil
}
```

注意：保持 `walkParts` 顺序与原 ParseBody 一致（先文本入 body，其它入 atts）。原 ParseBody 的文本判断有 `&& email.TextBody==""` 守卫；这里 walkParts 内部用局部 text/html 的空判断，ParseBody 再以 `email.TextBody==""` 决定是否覆盖，等价。

- [ ] **Step 4: 跑测试确认通过** `cd core && go test ./parser/ -v`，预期全绿（含原有 parser 测试不回归）。
- [ ] **Step 5: 提交** `feat(core): parser 共享 walker + ExtractAttachments + 内联图纳入附件`

### Task A3: core/imap FetchRawMessage

**Files:** Modify `core/imap/fetch.go`

- [ ] **Step 1: 实现**

```go
// FetchRawMessage 取单封邮件的完整原始 RFC822 字节（BODY[]）。供附件按需提取用。
func (s *Session) FetchRawMessage(uid imapv2.UID) ([]byte, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}
	var uidSet imapv2.UIDSet
	uidSet.AddNum(uid)

	section := &imapv2.FetchItemBodySection{} // 空 section = 整封 BODY[]
	fetchOpts := &imapv2.FetchOptions{
		UID:         true,
		BodySection: []*imapv2.FetchItemBodySection{section},
	}
	cmd := s.Client.Fetch(uidSet, fetchOpts)
	var raw []byte
	for {
		msg := cmd.Next()
		if msg == nil {
			break
		}
		buf, err := msg.Collect()
		if err != nil {
			continue
		}
		if b := buf.FindBodySection(section); b != nil {
			raw = b
		}
	}
	if err := cmd.Close(); err != nil {
		return raw, fmt.Errorf("fetch raw close: %w", err)
	}
	if raw == nil {
		return nil, fmt.Errorf("message uid %d not found", uid)
	}
	return raw, nil
}
```

- [ ] **Step 2: 编译** `cd core && go build ./...`，预期通过。
- [ ] **Step 3: 提交** `feat(core): imap.FetchRawMessage（取整封原始 RFC822）`

---

## Phase B：flymail 后端附件接口

### Task B1: sync.Service.AttachmentContent + Session 接口

**Files:** Modify `modules/email/sync/service.go`、`modules/email/sync/service_test.go`

- [ ] **Step 1: Session 接口加方法**（service.go）

在 `Session` 接口追加：`FetchRawMessage(uid imapv2.UID) ([]byte, error)`。

- [ ] **Step 2: service_test.go fakeSession 补方法**（避免编译失败）

```go
func (f *fakeSession) FetchRawMessage(uid imapv2.UID) ([]byte, error) { return f.rawMessage, nil }
```
给 fakeSession 加字段 `rawMessage []byte`（测试按需设置一封含附件的原始邮件）。

- [ ] **Step 3: 写失败测试** `TestAttachmentContent`（service_test.go）

预置 account=1 + INBOX 文件夹 + 一封邮件（message id=1，uid=1，folder=inbox）。fakeSession.rawMessage 设为一封含 1 个 pdf 附件的原始 RFC822（可用 core/parser 测试里同款构造，或内联一段 MIME 文本）。调 `svc.AttachmentContent(1, 0)` → 返回 filename/contentType/data 正确（data 非空）。越界 idx 返回错误。

- [ ] **Step 4: 实现**（service.go）

```go
import "bytes"
import coreparser "flymail-core/parser"

// AttachmentResult 附件下载结果。
type AttachmentResult struct {
	Filename    string
	ContentType string
	Data        []byte
}

// ErrAttachmentNotFound 附件下标越界。
var ErrAttachmentNotFound = errors.New("attachment not found")

// AttachmentContent 按需从 IMAP 取整封邮件，解析出第 idx 个附件（含内联图，顺序同 MessageDetail.attachments）。
func (s *Service) AttachmentContent(messageID uint, idx int) (*AttachmentResult, error) {
	m, err := s.messages.GetByID(messageID)
	if err != nil {
		return nil, err
	}
	f, err := s.folders.GetByID(m.FolderID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.accounts.IMAPConfig(m.AccountID)
	if err != nil {
		return nil, err
	}
	sess, err := s.dial(cfg)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	if _, err := sess.SelectFolder(f.Path); err != nil {
		return nil, err
	}
	raw, err := sess.FetchRawMessage(imapv2.UID(m.UID))
	if err != nil {
		return nil, err
	}
	atts, err := coreparser.ExtractAttachments(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if idx < 0 || idx >= len(atts) {
		return nil, ErrAttachmentNotFound
	}
	a := atts[idx]
	ct := a.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	return &AttachmentResult{Filename: a.Filename, ContentType: ct, Data: a.Content}, nil
}
```

- [ ] **Step 5: 跑测试** `cd flymail/backend && go test -p 1 ./modules/email/sync/ -run TestAttachment -v`，预期 PASS；整包 `go test -p 1 ./modules/email/sync/` 不回归。
- [ ] **Step 6: 提交** `feat(flymail): sync.AttachmentContent（按需取附件字节）`

### Task B2: 附件 HTTP 端点（dual auth + 流式）

**Files:** Modify `modules/email/sync/handler.go`、`internal/server/router.go`

- [ ] **Step 1: router.go — Deps 加 auth 校验函数 + 注册路由**

`Deps` 增加字段 `VerifyToken func(token string) error`（app 装配时注入 `authSvc.VerifyAccessToken` 包装）。在 `/api/v1` 组（protected 之前，与 `/events` 同级）注册：

```go
if deps.Sync != nil && deps.VerifyToken != nil {
	api.GET("/messages/:id/attachments/:idx", syncmod.AttachmentHandler(deps.Sync, deps.VerifyToken))
}
```

app.go 装配处传入：`VerifyToken: func(t string) error { _, err := authSvc.VerifyAccessToken(t); return err }`。

- [ ] **Step 2: handler.go — AttachmentHandler**

```go
// AttachmentHandler 流式返回附件。鉴权：Authorization: Bearer 头 或 ?access_token= query
// （img/iframe/预览新标签无法设头，故支持 query，见 KI-2）。
func AttachmentHandler(svc *Service, verify func(token string) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Query("access_token")
		if token == "" {
			h := c.GetHeader("Authorization")
			token = strings.TrimPrefix(h, "Bearer ")
		}
		if verify(token) != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		mid, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
			return
		}
		idx, err := strconv.Atoi(c.Param("idx"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid index"})
			return
		}
		res, err := svc.AttachmentContent(uint(mid), idx)
		if err != nil {
			if errors.Is(err, ErrAttachmentNotFound) || errors.Is(err, message.ErrMessageNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "attachment not found"})
				return
			}
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		// 默认 inline（便于图片/PDF 预览）；filename 用 RFC5987 编码兼容中文名。
		disp := "inline"
		if c.Query("dl") == "1" {
			disp = "attachment"
		}
		fn := res.Filename
		if fn == "" {
			fn = "attachment"
		}
		c.Header("Content-Disposition", disp+"; filename*=UTF-8''"+url.PathEscape(fn))
		c.Data(http.StatusOK, res.ContentType, res.Data)
	}
}
```

需要 import：`net/url`、`strings`、`flymail/modules/email/message`（已有）。

- [ ] **Step 3: 编译 + 全后端测试** `cd flymail/backend && go build ./... && go test -p 1 ./...`，预期通过、不回归。
- [ ] **Step 4: 提交** `feat(flymail): 附件下载/预览端点 GET /messages/:id/attachments/:idx（dual auth）`

---

## Phase C：前端附件下载/预览/内联图

### Task C1: lib/attachments.ts 工具

**Files:** Create `src/lib/attachments.ts`；确认 `src/lib/types.ts` 的 Attachment 含 `content_id?: string; is_inline: boolean`

- [ ] **Step 1: 实现**

```ts
import { auth } from '@/lib/auth'
import api from '@/lib/api'
import type { Attachment } from '@/lib/types'

const PREVIEWABLE = /^(image\/|application\/pdf)/

export function isPreviewable(a: Attachment): boolean {
  return PREVIEWABLE.test(a.content_type || '')
}

// 带 token 的附件 URL（用于 img/iframe/预览新标签；token 走 query，见 KI-2）。
export function attachmentUrl(messageId: number, idx: number, opts?: { download?: boolean }): string {
  const t = auth.access ?? ''
  const dl = opts?.download ? '&dl=1' : ''
  return `/api/v1/messages/${messageId}/attachments/${idx}?access_token=${encodeURIComponent(t)}${dl}`
}

// 用 axios 以 Bearer 头取 blob 并触发浏览器下载（不暴露 token 到 URL）。
export async function downloadAttachment(messageId: number, idx: number, filename: string): Promise<void> {
  const res = await api.get(`/messages/${messageId}/attachments/${idx}?dl=1`, { responseType: 'blob' })
  const url = URL.createObjectURL(res.data as Blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename || 'attachment'
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

// 把正文 HTML 中的 cid: 引用改写为附件接口 URL，使内联图渲染。
export function rewriteCidLinks(html: string, messageId: number, attachments: Attachment[]): string {
  return html.replace(/(["'(])cid:([^"')\s]+)/gi, (m, pre, cid) => {
    const idx = attachments.findIndex((a) => a.content_id && a.content_id.toLowerCase() === String(cid).toLowerCase())
    if (idx < 0) return m
    return pre + attachmentUrl(messageId, idx)
  })
}
```

- [ ] **Step 2: 类型检查** `pnpm -C flymail/frontend exec tsc --noEmit`，预期通过。
- [ ] **Step 3: 提交** `feat(flymail-web): 附件工具（URL/下载/cid 改写/可预览判断）`

### Task C2: Reader 接入内联图 + 附件下载/预览

**Files:** Modify `src/components/mail/Reader.tsx`

先读 Reader.tsx 现状（远程图拦截 `blockRemoteImages`、附件列表渲染、sandbox iframe srcDoc 构造）。改动：

- [ ] **Step 1: 内联图改写**：在构造 iframe srcDoc 的 HTML 处理链里，先 `rewriteCidLinks(html, messageId, detail.attachments)` 再做远程图拦截。注意 cid 改写后的 URL 是同源 `/api/...`，远程图拦截正则不应误伤它（远程图拦截只针对 http(s) 外链；cid 改写出的是相对路径 `/api/...`，确认不被拦截）。

- [ ] **Step 2: 附件列表**：过滤 `!a.is_inline` 后渲染；每项加两个按钮：
  - 下载：`onClick={() => downloadAttachment(messageId, idx, a.filename)}`（注意 idx 必须是在**完整 attachments 数组**中的下标，不是过滤后的下标——遍历时用原始 index）。
  - 预览（仅 `isPreviewable(a)`）：`<a href={attachmentUrl(messageId, idx)} target="_blank" rel="noopener noreferrer">预览</a>`。
  - 移除原“暂不支持下载”占位文案。

实现要点：遍历 `detail.attachments.map((a, idx) => ...)` 保留原始 idx，再 `if (a.is_inline) return null`。

- [ ] **Step 3: 类型检查 + 无头冒烟**（若可）：`tsc --noEmit` 通过。
- [ ] **Step 4: 提交** `feat(flymail-web): Reader 内联图渲染 + 附件下载/预览`

### Task C3: i18n

**Files:** Modify `src/locales/zh.json`、`src/locales/en.json`

- [ ] **Step 1:** `reader` 段增/改 key：`"download": "下载"`、`"preview": "预览"`，移除/保留 `attachmentNoDownload`（不再使用可删）。en 对应 `Download`/`Preview`。改完用 JSON 解析校验无重复 key、无引号内嵌。
- [ ] **Step 2: 校验** `node -e "JSON.parse(require('fs').readFileSync('src/locales/zh.json','utf8'));JSON.parse(require('fs').readFileSync('src/locales/en.json','utf8'))"`。
- [ ] **Step 3: 提交** `feat(flymail-web): 附件下载/预览 i18n`

---

## Phase D：文档

### Task D1: KI-2 补充

**Files:** Modify `docs/superpowers/known-issues.md`

- [ ] **Step 1:** 在 KI-2 补一句：附件接口 `GET /messages/:id/attachments/:idx` 的 img/iframe/预览访问同样经 `?access_token=` query（与 SSE 同模式，主动下载走 Bearer 头不暴露 token）；后续 stream-ticket 方案一并覆盖附件接口。
- [ ] **Step 2: 提交** `docs(flymail): KI-2 补充附件接口 token-in-URL`

---

## 最终审查 + 收尾

- [ ] 全量 `cd core && go test ./...`、`cd flymail/backend && go test -p 1 ./...` 全绿（`-p 1` 规避本机构建 OOM）。
- [ ] 前端 `pnpm build` 通过。
- [ ] 派最终 code-reviewer 审查整支分支（重点：附件下标对齐的脆弱性、内联图/远程图正则不互相误伤、dual auth 正确性、大附件内存占用一次性 ReadAll 的取舍、Content-Disposition 中文名编码、token-in-URL）。
- [ ] superpowers:finishing-a-development-branch（合并回 main 并删分支）。
- [ ] 真机验证清单交用户：打开带附件邮件 → 附件列表显示 → 下载（中文名正确）→ 图片/PDF 点预览新标签查看 → 带签名图/内联图的邮件正文图片正常显示。

## 已知限制 / 取舍

- **按需重抓整封**：每次下载/预览某附件都重新 FETCH 整封邮件并 `io.ReadAll` 到内存提取，大邮件多附件场景有重复下载与内存峰值；MVP 取舍，后续可用 BODYSTRUCTURE + 分部 FETCH 优化。
- **下标对齐**：前端按 MessageDetail.attachments 下标引用，后端 ExtractAttachments 同序取——依赖 ParseBody/ExtractAttachments 共享 walker 的确定性顺序（A2 测试锁定）。
- **token-in-URL**：附件预览/内联图沿用 KI-2，后续 stream-ticket 一并改。
- **附件发送**仍未做（M5 推后项），本里程碑只做接收侧下载/预览。
