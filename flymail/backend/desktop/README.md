# FlyMail 桌面端（Wails）

复用 server 形态的同一 gin 引擎作为 Wails 的 `AssetServer.Handler`：SPA 与 `/api/v1`
全部走同源 `wails://` 协议，无需 TCP 监听、无 CORS、零业务重写。前端产物经
`web/embed.go` 嵌入 Go 二进制。

## 本地应用体验

- **开箱即用**：首次运行（数据库中无管理员）自动创建默认账户 `admin/admin`，
  登录后请在「设置 → 账户」中修改密码。登录态经 WebView2 localStorage 持久化，
  refresh token 有效期内（默认 7 天）无需重复登录。
- **单实例**：重复启动只会唤起已有窗口（`SingleInstanceLock`），避免 SQLite 双开。
- **系统托盘**：点击窗口「关闭」最小化到托盘，后台继续收信；托盘左键/双击唤起
  窗口，右键菜单可「显示主窗口 / 退出」。真正退出请走托盘菜单。
- **原生通知**：新邮件（`mail_new`）弹 Windows toast；同步失败等其余事件留在
  站内通知中心，避免打扰。
- **窗口状态记忆**：尺寸/位置/最大化保存在 `<数据目录>/window_state.json`，
  异常（如显示器变更导致窗口跑到屏幕外）删除该文件即可复位。

## 数据目录

优先级从高到低：

1. `FLYMAIL_DATA_DIR` 环境变量；
2. **便携模式**：exe 同目录放一个 `portable.txt`（内容任意）→ 数据落
   `<exe 目录>/data`，跟着 exe 走；
3. 默认：OS 用户数据目录，Windows 为 `%APPDATA%\FlyMail`。

与命令行 `flymail server` 共用同一套配置/库结构（server 形态默认仍是 `./data`）。

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

## 实现要点

- 托盘用 `energye/systray` 的 `RunWithExternalLoop`：托盘消息窗口必须创建在
  主 OS 线程上（`main()` 内、`wails.Run` 之前注册），事件由 Wails 自己的消息泵
  分发到 systray 的 wndProc。⚠ 不能在 goroutine 里调 `systray.Run`——窗口线程
  与 GetMessage 泵线程分离会导致图标显示但事件全丢（踩过）。托盘回调里调
  Wails runtime 前先 `go` 一跳脱离 wndProc 调用栈。
- toast 用 `go-toast/v2`：`initToast` 在注册表登记 AppID/GUID（HKCU，无需管理员），
  失败仅降级为无原生通知。
- 新邮件事件经 `app.SetEmitHook` 注入的观察者到达桌面层，与站内通知/外发渠道
  （notify 模块）解耦。

## 已知事项

- **SSE 实时推送**：经 WebView2 自定义协议，社区有缓冲问题的先例；即便降级，
  react-query 轮询（通知 30s / 监控 5s）+ 后台同步仍兜底，不影响核心可用。真机需验证。
- 开发热重载（`wails dev`）未配置：当前交付目标为可运行二进制。
- 旧版本桌面端数据在 exe 工作目录 `./data`；升级后若需保留数据，把旧 `data`
  目录拷到 `%APPDATA%\FlyMail`（或放 `portable.txt` 走便携模式）。
