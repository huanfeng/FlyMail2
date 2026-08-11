# MailDev

自部署邮件工具集 monorepo。各子项目状态如下：

| 子项目 | 说明 | 状态 |
|--------|------|------|
| [`flymail/`](flymail/) | 自部署邮件客户端（Go + React，支持 Wails 桌面端打包） | **活跃开发中（当前重点）** |
| [`core/`](core/) | 共享 Go 库：imap / smtp / parser / logger / auth / crypto / provider 等 | 活跃（随 flymail 演进） |
| [`mail2im/`](mail2im/) | 邮件转 IM 通知工具（Telegram / 企业微信 / 飞书 / 钉钉等） | 维护态（功能完成，暂停新功能） |

## 工作区

`go.work` 纳入 `core`、`flymail/backend`、`mail2im/backend`、`mail2im/oauth-proxy`，
workspace 内模块以本地源码互相解析依赖。

## 文档

- 设计文档与实现计划：[`docs/superpowers/`](docs/superpowers/)（specs = 设计，plans = 任务分解）
- 各子项目自带 `README.md` / `docs/`；历史 AI 分析笔记归档在各自 `docs/archive/`
- FlyMail 开发环境（后台进程托管、日志、启停）：`flymail/dev.ps1`

## FlyMail 快速上手

```powershell
cd flymail
./dev.ps1 start   # 启动前后端（后台进程，日志在 flymail/logs/）
./dev.ps1 status  # 查看状态
./dev.ps1 stop    # 停止
```

后端默认仅监听 127.0.0.1，数据与日志目录均已 gitignore。
