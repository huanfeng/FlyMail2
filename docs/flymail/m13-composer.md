# M13 — 撰写器升级

**状态**：进行中
**优先级**：P2

## 问题

`react-simple-wysiwyg` 是一个 contentEditable 薄包装：无 schema、无粘贴清洗、无法扩展节点。
正式回复场景需要的能力它都给不了——从 Outlook/Gmail 粘过来的富文本会带进整坨 `<o:p>`、
`mso-*` 样式和裸 `<font>`；插入表格、改字号只能靠 `document.execCommand`（已废弃且各浏览器行为不一）；
引用原文没法折叠，因为它没有"节点"这个概念，只有一串 HTML 字符串。

## 范围

1. 编辑器内核换 **Tiptap**（ProseMirror），schema 受控、粘贴经 schema 过滤
2. 富文本工具栏：字号、颜色、高亮、列表、链接、表格
3. 引用折叠：回复时原文包在可折叠节点里，默认收起，展开后可编辑
4. 内联图片：粘贴/拖入的图片随邮件以 `cid:` 内联发出
5. 签名：按账户配置，新建 / 回复分别可选是否插入，切换发件人时同步替换
6. 发件人别名：一个账户下可配多个发信地址

---

## 数据模型

### `account_aliases`（新表）

```go
type Alias struct {
    ID          uint   `gorm:"primaryKey"`
    AccountID   uint   `gorm:"index;not null"`
    Email       string `gorm:"not null"`      // 唯一索引 (account_id, email)
    DisplayName string
    IsDefault   bool                          // 同账户至多一个，置位时其余自动清零
    CreatedAt, UpdatedAt time.Time
}
```

### `account_signatures`（新表，与账户 1:1）

```go
type Signature struct {
    AccountID  uint   `gorm:"primaryKey"`     // 主键即外键，天然 1:1
    BodyHTML   string
    UseOnNew   bool
    UseOnReply bool
    UpdatedAt  time.Time
}
```

签名 HTML 写入时过一遍 `htmlsan.Sanitize(html, true)`。签名虽由本人编辑，
但它会被注入到编辑器 DOM 里，且随每封信发出去；净化一次的成本远低于日后排查。

### `drafts`（加字段）

`FromAlias string` — 空表示用账户主地址。老草稿读出为空，行为不变。

---

## API 契约

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/accounts/:id/aliases` | 列出别名 |
| POST | `/accounts/:id/aliases` | 新增 `{email, display_name, is_default}` |
| PUT | `/accounts/:id/aliases/:aliasId` | 修改 |
| DELETE | `/accounts/:id/aliases/:aliasId` | 删除 |
| GET | `/accounts/:id/signature` | 取签名，未配置返回空对象 |
| PUT | `/accounts/:id/signature` | 保存 `{body_html, use_on_new, use_on_reply}` |

### 发送

`POST /send` 的 `payload` JSON 增加两个字段：

```jsonc
{
  "from_alias": "sales@example.com",   // 可选；必须是该账户已配置的别名，否则 400
  "inline_cids": ["ii_abc123", "ii_def456"]  // 与 form.File["inline"] 按序一一对应
}
```

- 普通附件仍走 `form.File["attachments"]`
- 内联资源走 `form.File["inline"]`，其 Content-ID 取自 `inline_cids` 同下标项
- HTML 中以 `<img src="cid:ii_abc123">` 引用

**cid 校验**：只接受 `[A-Za-z0-9._-]{1,128}`。`Content-ID: <...>` 是邮件头，
cid 里混进 CRLF 就是一次头注入——这个校验不是格式洁癖，是安全边界。

---

## MIME 结构

现在只有两种形态，M13 后是四种，按"有没有内联图 × 有没有普通附件"取：

```
无内联、无附件      text/html                       （保持历史字节兼容）

有内联、无附件      multipart/related; type="text/html"
                    ├── text/html
                    └── image/png; Content-ID: <cid>; Content-Disposition: inline

无内联、有附件      multipart/mixed
                    ├── text/html
                    └── application/pdf; Content-Disposition: attachment

有内联、有附件      multipart/mixed
                    ├── multipart/related; type="text/html"
                    │   ├── text/html
                    │   └── image/png; Content-ID: <cid>
                    └── application/pdf; Content-Disposition: attachment
```

两条不能违反的规则：内联资源必须和引用它的 HTML 在**同一个** related 容器内
（放到外层 mixed 里，Outlook 会把它当成独立附件、图裂）；related 的**第一个** part
必须是 HTML（省略 `start` 参数时，接收方按首 part 认定根文档）。

## 别名与信封发件人

`From:` 头用别名地址，**SMTP 信封发件人（MAIL FROM）仍用账户主地址**。

多数 SMTP 服务器只允许信封发件人等于认证账户，用别名会被直接拒收；而 SPF 校验的
恰恰是信封发件人的域。主地址做信封、别名做 From，是同域别名（`sales@x.com` 之于
`admin@x.com`）能正常投递的唯一组合。跨域别名的 DMARC 对齐仍会失败——那是协议约束，
不是实现能绕开的，UI 上不作承诺。

不额外写 `Sender:` 头：RFC 5322 建议 From ≠ 实际发送者时补 Sender，但 Gmail 会据此
显示"由 … 代发"，主流客户端（Thunderbird identity）也都不写。

---

## 前端要点

- **粘贴**：Tiptap schema 天然丢弃未注册节点，但要显式允许 `span[style]` 的字号/颜色，
  否则 Outlook 粘过来的格式会被吃掉。`mso-*` 私有属性一律不保留。
- **内联图**：粘贴/拖入 → 生成 `cid` 与 blob URL，编辑器内用 blob 预览，
  发送前把 `<img src="blob:…">` 重写为 `cid:…` 并把文件挂到 `inline` 字段。
- **草稿里的内联图**：存为 `data:` URI（草稿自包含，不需要额外的服务端暂存区），
  发送时再转 cid。草稿总大小按 5 MiB 截断并提示。
- **引用折叠**：自定义节点 `quoteBlock`，NodeView 渲染成"显示引用内容"折叠头，
  展开后内部仍是可编辑的正常内容。
- **签名**：包在 `signature` 节点内，切换发件人时整节点替换，避免误伤用户正文。

---

## 验收标准

- [ ] 从 Outlook / Gmail 粘贴的富文本保留格式
- [ ] 内联图片在收件方（Gmail / Outlook）正确显示
- [ ] 签名按账户正确插入，切换发件账户时同步切换
- [ ] 别名发信时 `From` 头正确，且通过 SPF 校验的服务器能正常投递
- [ ] 现有草稿的读写兼容不破坏

---

## 实现说明（后端）

### 内联图的 Content-Type 必须由服务端定

表单上传的 Content-Type 完全由客户端决定，而它经常就是 `application/octet-stream`
（Go 的 `CreateFormFile` 默认值，某些浏览器对未知扩展名也一样）。带着这个类型发出去，
收件方不会把它当图片渲染——一张裂图。所以内联资源的类型由服务端重新判定：
**先内容嗅探，嗅探不出图片再看扩展名**。嗅探对 png/jpeg/gif/webp 准确且骗不过去，
svg 这类文本格式嗅探不出来，才回退扩展名。

这个 bug 是 E2E 抓到的：手工点测时浏览器通常给出正确 MIME，只有程序化的
multipart 客户端才会暴露。

### `data:` URI 的兜底转换

草稿里的内联图存成 `data:` URI，而 Outlook 与 Gmail 都屏蔽 `data:` 图片
（这正是它们防追踪的手段之一）。所以 `send.Service.Send` 在构建 MIME 前会把正文里
残留的 `data:image/*` 一律转成 `cid:` 内联附件。放在 Send 而不是 draft 里，
"草稿直发"与"前端漏网"两条路径一次覆盖。

解不开的 base64 原样保留而不是报错——宁可那一张图裂，也不要整封信发不出去。

### 附件总量的两道闸

`multipart` 路径在 handler 里按累计字节数拦（25 MiB）。但**草稿走的是 JSON**，
根本不经过那道校验，`data:` URI 展开后体积还会再涨。所以 `Send` 里另有一道总量兜底。

### 别名的默认项与创建时间

同账户至多一个默认别名，靠"置位时在同一事务内清零其余项"保证。
修改别名时先读出原记录再改字段——`gorm.Save` 对带主键的记录做的是全字段 UPDATE，
拿一个新结构体去存会把 `CreatedAt` 刷成零值（已有断言守着）。

删账户时在同一事务里连带清理别名与签名，否则重建同 id 的账户会捡到上一个的发信身份。

## 已知限制

- 别名列表按账户逐个拉取（`/accounts/:id/aliases`），账户多时是 N 次请求。
  账户数通常个位数，暂不合并进账户列表响应。
- 跨域别名（`me@a.com` 之于 `me@b.com`）的 DMARC 对齐仍会失败，这是协议约束。
- 内联图与附件都全量读进内存，25 MiB 上限之下可接受，不做流式。

---

## 审查发现与修复

两轮审查（后端安全/正确性 + 前端）都用实际运行验证，而不是只读代码。

### 已修

**别名只接受朴素 addr-spec**（严重）。原先用 `mail.ParseAddress` 校验却把**原始输入**落库，
而它接受的是整个 name-addr——用户填 `Bob <bob@e.com>`（别名表单旁边就有显示名栏，
这是极自然的误输入）会被判合法。实测那个串存下去后发信写出的是：

```
From: "N" <"bob <bob"@e.com>>     ← 尖括号不配对
Message-ID: <1234@e.com>>          ← 多出一个 >
```

严格的 MTA（Postfix `smtpd_reject_unlisted_sender`、Exchange）直接 501 拒信，
宽松的投进去但线程断掉。**最坏的地方是错误被推迟**：保存那一刻用户看到的是"保存成功"，
只有真的发一封才炸。现在存的是解析出来的 addr-spec，并加了朴素性校验
（纯 ASCII、单个 @、无引号/括号/逗号、拒域名字面量 `a@[127.0.0.1]`、长度 ≤254）。

**`In-Reply-To` / `References` 补字符集校验**（中等）。这两个头原先只去首尾尖括号就原样写入，
实测能注出任意头（`Bcc: victim@evil.com` 被 `net/mail` 确认解析成真正的 Bcc）。
目前远程打不通——入站侧 `core/parser.MessageIDs()` 与 `textproto` 的续行展开挡住了 CRLF——
但这道防线**依赖的是别的模块恰好做了归一化**，任何一次 core 侧重构都会静默捅穿它。
同一个函数里为 cid 建了安全边界，紧邻的两个头却不设防，本身就是不一致。
现在整个值含 CR/LF 直接整条丢弃（那必然是注入尝试），合法 msg-id 校验后重新拼装。

### 待修（不阻塞本次提交，已记录）

- **草稿直发路径的内存放大**（中等）：`InlineDataURIImages` 先把所有 `data:` 图解码进内存，
  之后才判 25 MiB。单张有 10 MiB 上限但**张数无上限**，且路由层没有任何请求体大小限制。
  实测 40 张 9 MiB 图的单个请求吃掉 2.4 GiB 堆、跑满 209 秒才回 500。
  multipart 路径有前置校验挡着，恰恰是 JSON/草稿这条绕过去了。
  修法：进解码前先判 `len(BodyHTML)`（O(1)）+ 解码中累计短路 + 路由层 `MaxBytesReader`。
  顺带这个错误应该是 400 不是 500。
- **`data:` 图正则按 `src=` 子串匹配**（中等）：没有前置边界，实测 `data-src="data:image/..."`
  会被改成 `data-src="cid:..."`（属性名没改，图在收件方是裂的，但附件照挂），
  正文里贴一段讲 data URI 的代码也会被静默篡改并多出无引用的 inline part。
  另有漏匹配：`srcset`、CSS `background:url(...)`、大写 `DATA:IMAGE`
  （`strings.Contains` 预检是大小写敏感的，`(?i)` 白加了）。
  修法：改用 `x/net/html` tokenizer 遍历属性，项目里 `internal/htmlsan` 已有同款实现可复用。
- **重复 cid 被静默接受**（轻微）：`inline_cids: ["ii_dup","ii_dup"]` 会产出两个同 `Content-ID`
  的 part，第二张永远显示不出来。既然已因"数量对不上宁可整封拒收"做了严格配对，重复也该一并拒掉。
- **唯一索引撞车返回 500 而非 409**（轻微）：服务层查重是第一道防线且并发下实测没撞上过
  （SQLite 写锁把并发串行化了），但 DB 层 `ErrDuplicatedKey` 落到了 `default` 分支。
- **`postMultipart` 测试辅助的 `ctype` 字段是死代码**（轻微）：`CreateFormFile` 永远写
  `application/octet-stream`，所以 `TestSendInlineSniffsContentType` 是**碰巧**走到嗅探分支的，
  且没有任何用例覆盖"客户端声明了非 octet-stream 类型"那条分支。
