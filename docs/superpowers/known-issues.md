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
