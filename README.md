# MailDev

自部署邮件工具集 monorepo。各子项目状态如下：

| 子项目 | 说明 | 状态 |
|--------|------|------|
| [`flymail/`](flymail/) | 自部署邮件客户端（Go + React，支持 Wails 桌面端打包）—— 见其 [README](flymail/README.md) | **活跃开发中（当前重点）** |
| [`core/`](core/) | 共享 Go 库：imap / smtp / parser / logger / auth / crypto / provider 等 | 活跃（随 flymail 演进） |
| [`mail2im/`](mail2im/) | 邮件转 IM 通知工具（Telegram / 企业微信 / 飞书 / 钉钉等） | 维护态（功能完成，暂停新功能） |

## 工作区

`go.work` 纳入 `core`、`flymail/backend`、`mail2im/backend`、`mail2im/oauth-proxy`，
workspace 内模块以本地源码互相解析依赖。

## 文档

- FlyMail 文档：[`docs/flymail/`](docs/flymail/)
  - [部署与测试环境](docs/flymail/deployment.md)（Docker 部署、日常测试流程、故障排查）
  - [功能对比分析](docs/flymail/mailflow-gap-analysis.md)（对标开源 MailFlow 的现状快照）
  - [路线图](docs/flymail/roadmap.md)（M9–M16，持续更新）
  - [M9 检索重构](docs/flymail/m9-search.md)（FTS5 索引、搜索语法、服务端兜底的设计与取舍）
  - [M10 会话线程](docs/flymail/m10-threads.md)（线程归属算法、会话列表查询与索引、会话级操作范围）
  - [M11 规则引擎 + 黑名单](docs/flymail/m11-rules.md)（执行时序、幂等与动作合并、条件语义、接口）
  - [M12 阅读隐私与安全加固](docs/flymail/m12-privacy.md)（服务端净化边界、iframe 沙箱方案、登录限流与代理信任、附件端点）
  - [M13 撰写器升级](docs/flymail/m13-composer.md)（Tiptap 内核、内联图 MIME 结构、别名与信封发件人分离、签名）
  - [M14 OAuth2 接入](docs/flymail/m14-oauth.md)（授权码 + PKCE / 设备码、令牌刷新与并发合并、回调形态取舍）
- `docs/superpowers/`：早期设计文档与实现计划（M1–M8），已停止更新，仅作历史参考
- 各子项目自带 `README.md` / `docs/`；历史 AI 分析笔记归档在各自 `docs/archive/`
- FlyMail 开发环境（后台进程托管、日志、启停）：`flymail/dev.ps1`

## FlyMail 快速上手

### 本地开发

```powershell
cd flymail
./dev.ps1 start   # 启动前后端（后台进程，日志在 flymail/logs/）
./dev.ps1 status  # 查看状态
./dev.ps1 stop    # 停止
```

后端默认仅监听 127.0.0.1，数据与日志目录均已 gitignore。

### Docker 部署

```bash
cp .env.example .env    # 填入密钥与管理员密码
docker compose up -d --build
```

构建上下文必须是仓库根目录（backend 的 go.mod 依赖 `../../core`）。
详见[部署文档](docs/flymail/deployment.md)。
