# FlyMail 已知问题 / 待修登记

记录已发现但暂不修复的问题，便于后续集中处理。新问题往下追加。

---

## KI-1：邮件正文 iframe 内的链接会在 iframe 内直接打开（安全 + 体验）

- **发现于**：M4（2026-06-01），Reader 正文沙箱 iframe。
- **现象**：邮件 HTML 正文用 `<iframe sandbox="" srcDoc=...>` 渲染。点击正文里的 `<a href>` 链接时，由于默认 `target=_self`，会在 **iframe 内部直接导航**，把邮件正文替换成目标网页（停留在沙箱里），而不是在浏览器新标签/外部打开。
- **风险**：① 用户体验差（邮件被替换、无法返回）；② 安全——不应让不可信邮件里的链接在应用内直接加载。
- **建议修法（后续）**：
  1. 渲染前对 HTML 做处理：给所有 `<a href="http...">` 注入 `target="_blank" rel="noopener noreferrer"`（同远程图拦截那套正则处理一起做）。
  2. iframe sandbox 增加 `allow-popups allow-popups-to-escape-sandbox`（仅为让 `target=_blank` 能在真正的新标签/外部浏览器打开；**不要**加 `allow-scripts`/`allow-top-navigation`）。
  3. 桌面 Wails 形态下，进一步拦截为用系统默认浏览器打开外链。
- **涉及文件**：`flymail/frontend/src/components/mail/Reader.tsx`（`blockRemoteImages` 附近，可加 `rewriteLinks`）。

---

## KI-2：SSE 端点的 access_token 经 URL query 传递

- **发现于**：M6（2026-06-02），实时收信 SSE 端点 `GET /api/v1/events`。
- **现象**：浏览器原生 `EventSource` 无法设置自定义请求头（不能带 `Authorization: Bearer`），故 access_token 通过 `?access_token=...` 走 URL query 传递并由后端 `sse.NewHandler` 校验。
- **风险**：token 可能落入服务器/代理访问日志、浏览器历史。自托管 localhost 场景下风险有限，但非最佳实践。
- **建议修法（后续）**：改为一次性 stream ticket——新增受保护端点 `POST /events/ticket` 返回短 TTL 一次性票据，前端用 `?ticket=...` 连接 SSE，后端校验并立即作废票据。或迁移到基于 `fetch` 的 SSE 客户端（可设头）。
- **同源问题（M7）**：附件接口 `GET /api/v1/messages/:id/attachments/:idx` 用于 img/iframe 内联图与浏览器内预览（新标签）时，同样无法设请求头，故也支持 `?access_token=` query 鉴权（用户主动下载走 axios Bearer 头取 blob，不暴露 token）。后续 stream-ticket 方案应一并覆盖附件接口。
- **涉及文件**：`flymail/backend/internal/sse/handler.go`、`flymail/frontend/src/lib/sse.ts`、`flymail/backend/modules/email/sync/handler.go`（AttachmentHandler）、`flymail/frontend/src/lib/attachments.ts`。
