# FlyMail 桌面端（Wails）

复用 server 形态的同一 gin 引擎作为 Wails 的 `AssetServer.Handler`：SPA 与 `/api/v1`
全部走同源 `wails://` 协议，无需 TCP 监听、无 CORS、零业务重写。前端产物经
`web/embed.go` 嵌入 Go 二进制。

## 数据目录

沿用 CLI 默认：`config.Load` 空选项 → `./data`（相对 exe 工作目录），或由
`FLYMAIL_DATA_DIR` 环境变量覆盖。与命令行 `flymail server` 共用同一套配置/库。

## 构建

桌面端不走 Wails 自带前端流水线（前端已由 vite 输出到 `backend/web/dist` 并被 Go 嵌入），
因此先构建前端，再用 `-s` 跳过 Wails 的前端步骤：

```bash
# 1) 构建前端（输出到 backend/web/dist）
cd flymail/frontend
pnpm install
pnpm build

# 2) 构建桌面 exe（跳过前端）
cd ../backend/desktop
wails build -s
```

产物：`flymail/backend/desktop/build/bin/FlyMail.exe`（已 gitignore，不入库）。

## 已知事项

- **SSE 实时推送**：经 WebView2 自定义协议，社区有缓冲问题的先例；即便降级，
  react-query 轮询（通知 30s / 监控 5s）+ 后台同步仍兜底，不影响核心可用。真机需验证。
- 开发热重载（`wails dev`）未配置：当前交付目标为可运行二进制。
