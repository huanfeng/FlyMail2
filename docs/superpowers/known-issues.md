# FlyMail 已知问题 / 待修登记

记录已发现但暂不修复的问题，便于后续集中处理。新问题往下追加。

---

## ✅ KI-1（已解决）：邮件正文 iframe 内的链接会在 iframe 内直接打开（安全 + 体验）

- **发现于**：M4（2026-06-01），Reader 正文沙箱 iframe。
- **现象**：邮件 HTML 正文用 `<iframe sandbox="" srcDoc=...>` 渲染。点击正文里的 `<a href>` 链接时，由于默认 `target=_self`，会在 **iframe 内部直接导航**，把邮件正文替换成目标网页（停留在沙箱里），而不是在浏览器新标签/外部打开。
- **风险**：① 用户体验差（邮件被替换、无法返回）；② 安全——不应让不可信邮件里的链接在应用内直接加载。
- **建议修法（后续）**：
  1. 渲染前对 HTML 做处理：给所有 `<a href="http...">` 注入 `target="_blank" rel="noopener noreferrer"`（同远程图拦截那套正则处理一起做）。
  2. iframe sandbox 增加 `allow-popups allow-popups-to-escape-sandbox`（仅为让 `target=_blank` 能在真正的新标签/外部浏览器打开；**不要**加 `allow-scripts`/`allow-top-navigation`）。
  3. 桌面 Wails 形态下，进一步拦截为用系统默认浏览器打开外链。
- **涉及文件**：`flymail/frontend/src/components/mail/Reader.tsx`（`blockRemoteImages` 附近，可加 `rewriteLinks`）。
- **解决于**：M12（2026-09-07）阅读隐私与安全加固，抽出 `frontend/src/lib/mail-frame.ts` 时一并做掉，当时未回来销账。
  实际实现比原建议更严：
  1. 注入脚本（带 CSP nonce）在 iframe 内拦截 `click` **与 `auxclick`**（中键不触发 click，漏了就会经
     `<base target=_blank>` + `allow-popups-to-escape-sandbox` 开出脱离沙箱的窗口，绕过整条白名单）；
  2. 只把 `http:` / `https:` / `mailto:` 经 postMessage 上报父窗口，其余协议（`file:` / `javascript:` / 自定义 scheme）
     拦下即止、不上报；文档内 `#` 锚点保留默认滚动；
  3. 父窗口 `MessageBody.tsx` 收到后交给 `lib/platform.ts` 的 `openExternal`——Wails 桌面端走
     `runtime.BrowserOpenURL` 交系统浏览器，浏览器端退化为 `window.open(..., 'noopener,noreferrer')`；
  4. `<base target="_blank">` 保留为注入脚本失效时的兜底路径。


---

## ✅ KI-2（已解决）：SSE 端点的 access_token 经 URL query 传递

- **发现于**：M6（2026-06-02），实时收信 SSE 端点 `GET /api/v1/events`。
- **现象**：浏览器原生 `EventSource` 无法设置自定义请求头（不能带 `Authorization: Bearer`），故 access_token 通过 `?access_token=...` 走 URL query 传递并由后端 `sse.NewHandler` 校验。
- **风险**：token 可能落入服务器/代理访问日志、浏览器历史。自托管 localhost 场景下风险有限，但非最佳实践。
- **建议修法（后续）**：改为一次性 stream ticket——新增受保护端点 `POST /events/ticket` 返回短 TTL 一次性票据，前端用 `?ticket=...` 连接 SSE，后端校验并立即作废票据。或迁移到基于 `fetch` 的 SSE 客户端（可设头）。
- **同源问题（M7）**：附件接口 `GET /api/v1/messages/:id/attachments/:idx` 用于 img/iframe 内联图与浏览器内预览（新标签）时，同样无法设请求头，故也支持 `?access_token=` query 鉴权（用户主动下载走 axios Bearer 头取 blob，不暴露 token）。后续 stream-ticket 方案应一并覆盖附件接口。
- **涉及文件**：`flymail/backend/internal/sse/handler.go`、`flymail/frontend/src/lib/sse.ts`、`flymail/backend/modules/email/sync/handler.go`（AttachmentHandler）、`flymail/frontend/src/lib/attachments.ts`。
- **解决于**：2026-09-10。两条 URL 鉴权路径按各自的形态分别收敛，**URL 上不再可能出现 access token**：
  1. **SSE**：新增受保护端点 `POST /api/v1/events/ticket`（挂在 Bearer 中间件后的独立子组），
     返回 60 秒有效的一次性票据；前端用 `?ticket=` 连接，后端在**握手时**核销（长连接照常存续）。
     票据存储在进程内（`internal/sse/ticket.go`，单锁保护，签发时顺带 GC 过期项），
     查找与删除在同一把锁内完成，保证并发下「一次性」成立。`?access_token=` 兼容路径已移除。
  2. **附件**：`?access_token=` 参数改名为 `?ticket=`，且**只接受**详情接口签发的 attachment_token
     （限定单封、1 小时；M12 已引入）。access token 从此只能走 `Authorization: Bearer`
     （用户主动下载的 axios blob 路径）。
     这里刻意**不用**一次性票据：一封信里的十几张 `cid:` 内联图由浏览器并发请求，
     用完即废的票只有第一张能加载出来；「限定单封 + 短时效 + 可重用」才是这条路径上正确的收敛方向。
  3. 前端 `sse.ts` 里自己实现的那份 token 刷新逻辑一并删除——取票走 `lib/api` 实例，
     401 由它的响应拦截器统一刷新重试，重连时重新取票，刷新自然一并发生。
- **残留说明**：邮件正文文档里仍然带着 attachment_token（这条路径上凭据躲不开那份文档）。
  开启远程内容后，邮件自带的 `<style>` 仍可用属性前缀选择器把它逐字符外泄（服务端净化保留 `<style>` 块，
  `allowRemote=true` 时 CSS `url()` 与 CSP `img-src https:` 都放行）。代价被限定在
  「拿到用户本就正在看的这一封邮件的附件、且一小时后失效」，不再是整个账号。
