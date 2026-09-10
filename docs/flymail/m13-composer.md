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

### 待修 → ✅ 已于 2026-09-10 收尾清理全部修完

原先记录的 5 项（内存放大 / `data:` 正则误匹配 / 重复 cid / 唯一索引 409 / 测试 ctype 死代码）
已在分支 `feat/flymail-cleanup` 一并修掉，并经独立审查（实现者未自我批准）。
审查在修复本身里又抓出 3 个缺陷，一并修完：

- **H-1 预算闸门被架空**：闸门算术本身正确（判断确实全在 `DecodeString` 之前），
  但它只约束「解码后字节数」，没约束 `strings.Fields`+`Join` 去空白这一步的分配——
  后者的代价与 payload 的**空白密度**成正比，与闸门判的量完全脱钩。
  实测 8 MiB 输入 → 119 MiB 分配（~15x），外推单请求可达 ~600 MiB，
  等于第 1 项声称的「超限输入一个字节都不放大」当时并未成立。
  改用预分配 `base64Len` 大小、单趟拷贝的 `stripWS`；`base64Len` 与 `stripWS` 共用
  同一个 `isASCIISpace` 判定，从结构上保证容量预估不会和实际处理的字符走偏。
  **教训：闸门必须约束真实的分配点，而不是一个与之相关但不等价的量。**
- **H-1 顺藤摸出的另外两处同类脱钩**（修复过程中自查发现，非审查提出）：
  - `convertSrcset` / `splitSrcset`：同一个 `strings.Fields` 病，且发生在预算闸门判大小**之前**，
    实测 20.1×——比点名的 `src`（14.9×）更重，且白烧 42 MB 一张图都没产出。
    改成 `splitSrcset` 只记边界返回子串（零拷贝）+ 新增 `splitCandidate` 按下标切 URL 与描述符。
  - `cleanMsgIDList`（`builder.go`，**M13 原有**）：`strings.FieldsFunc` 把整串所有分段一次性物化，
    而函数最多只保留 `max` 个 id（References 255 / In-Reply-To 1）。5 MiB 输入分配 53 MB。
    要命的是这条路上**没有任何长度闸门**——`in_reply_to` / `references` 是自由字符串字段，
    只受 `maxRequestBody` 约束。改成按下标手工切分、`len(ids) >= max` 即停。
- **L-1 `<style/>` 自闭合写法漏转**：`style` 是 rawtext 元素，Go tokenizer 对 `<style/>`
  返回 `SelfClosingTagToken` 的同时**仍然**把后续内容当 rawtext 交出
  （`readStartTag` 里 `rawTag` 的设置早于自闭合判断），只判 `StartTagToken` 就漏了。
  后果是里面的背景图不转 `cid:`，data: 图原样发出被 Gmail/Outlook 屏蔽 → 收件方裂图。
- **L-2 空 base64 payload 产出 0 字节 inline part**：撰写器图片加载失败时会留下占位的空 data URI，
  外发邮件多挂一个没内容的 `image/png`，部分客户端显示为「损坏的附件」。

### 仍未修（已评估，判定不值得改）

- **L-3 `splitSrcset` 对「恰好以 `;base64` 结尾的普通 URL」会吞掉分隔逗号**：
  如 `srcset="a.png?x=;base64, data:image/png;base64,… 2x"`，第一个逗号被
  `endsWithBase64Marker` 误判为 URI 内部逗号，两候选合并 → 后一个候选漏转、收件方裂图。
  被吞的逗号最终由 `Join(", ")` 复原，**结构不损坏**，影响仅限漏转。极构造化。
  要修则把标记判断收紧为「`;base64` 之前必须能回溯到一个 `data:` scheme」。
- **重复属性**：`<img src="data:A" src="data:B">` 两个都转、各登记一张附件，客户端只认第一个，
  多一个无人引用的 inline part。与「重复 cid 整封拒收」的严格度略不一致，但畸形 HTML 不值得加逻辑。
- **外观类**：候选 `Join(", ")` 后的双空格；改动过的开始标签被小写化而结束标签保留原样
  （`<style>…</STYLE>`，HTML 大小写不敏感，无影响）。
- **票据比对非常量时间**（`internal/sse/ticket.go`）：走 map 查找。
  32 字节 `crypto/rand` 空间下无实际攻击面，放弃 map 换常量时间比对得不偿失。
- **正文改写的固有分配常数（6–9 × 正文长度）**：tokenizer 缓冲 + 属性拷贝 + 输出缓冲 + 解码结果，
  与空白密度已无关（那是 H-1 修掉的部分），但 40 MiB 正文极限下单请求峰值仍是数百 MB。
  要再压只能改成流式改写、不整份物化，属另一个量级的改动。
  （曾考虑把 `maxInlineHTML` 从 40 MiB 下调到 34 MiB，未采纳——余量是有意留的，
  这 6 MiB 不会让 6–9 倍的固有常数有质变。）
- **`in_reply_to` / `references` 没有独立长度上限**：只受 `maxRequestBody` 约束。
  分配已与 `max` 挂钩不再放大，但仍有一次 O(请求体) 的扫描。

### 审查明确通过的部分

tokenizer 改写（除 L-1 外）经 14 组边界探针实证正确、未引入新的 XSS/注入面：
属性值统一双引号 + `html.EscapeString`（会转义 `"`，故无法提前闭合属性）、实体往返语义等价且更严、
未改动 token 原样 `z.Raw()` 拷回、rawtext 容器（`<textarea>`）内不被误伤、
`splitSrcset` 对 `data:image/png;base64,` 自带逗号的切分正确（base64 字母表不含逗号，
只需保护紧跟 `;base64` 的那一个）。

**职责边界**：`dataurl.go` 不是净化器，净化归 `internal/htmlsan`。两者方向相反——
htmlsan 是收信侧白名单净化，dataurl 是发信侧定点改写；dataurl 对命中标签会重新序列化，
与 htmlsan 的规范化结果不完全一致。这不构成安全问题（dataurl 的重写只会让转义更严），
但不要指望「两边处理完字节相同」。
