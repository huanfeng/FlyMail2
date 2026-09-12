# FlyMail

自部署的多账户邮件客户端。一个 Go 二进制 + 一个 SQLite 文件，没有 PostgreSQL、
没有 Redis、没有消息队列——`docker compose up -d` 就能跑起来，备份就是拷一个目录。

可以当服务跑（浏览器访问），也可以装成 Windows 桌面应用。

---

## 功能

**收信与整理**

- 多账户聚合收件箱，IMAP IDLE 实时推送，后台增量同步
- 会话线程：一条 10 封往返的讨论在列表里占 1 行，展开是手风琴，引用内容自动折叠
- 全文检索：SQLite FTS5 索引，支持 `from:` `subject:` `has:attachment` `is:unread`
  `before:` `in:` 等限定符，中文按二元切分入索引（一万多封邮件实测 10ms 内返回）
- 规则引擎：按发件人/主题/正文/附件等条件自动移动、标记、打标签、通知，
  命中可停止后续规则；黑名单命中直接进垃圾箱且不触发通知
- IM 通知：新邮件推到 Telegram / 企业微信 / 飞书 / 钉钉等渠道

**键盘操作**

整套键位取自主流邮件客户端，肌肉记忆可以直接迁移过来。按 `?` 随时看速查表。

| 键 | 作用 | | 键 | 作用 |
|---|---|---|---|---|
| `J` / `K` | 下一条 / 上一条 | | `E` | 归档 |
| `U` | 回到列表 | | `#` / `Del` | 删除 |
| `C` / `N` | 写新邮件 | | `S` | 星标 |
| `R` / `A` / `F` | 回复 / 全部回复 / 转发 | | `Shift`+`U` | 标为未读 |
| `Ctrl`+`Enter` | 发送 | | `X` | 选中当前行 |
| `/` 或 `Ctrl`+`K` | 搜索 | | `Shift`+`J`/`K` | 扩展选择 |
| `G` 然后 `I`/`S`/`T`/`D` | 跳收件箱 / 星标 / 已发送 / 草稿 | | `Shift`+点击 | 选中一段 |

删除、归档、移动都带 5 秒撤销：不会弹确认框打断操作，点错了按提示条上的「撤销」即可。
处理完一封会自动打开下一封，适合一口气把收件箱清空。

**阅读**

- 默认不发起任何邮件内容触发的外部请求：服务端净化 HTML，远程图片替换为占位符，
  用户点"显示图片"才加载，可按发件人记住选择
- 正文 iframe 完整沙箱隔离（不开 `allow-same-origin`，高度经 `postMessage` 上报）

**撰写**

- Tiptap 富文本编辑器，工具栏含字号、颜色、高亮、列表、链接、表格
- 粘贴或拖入的图片自动转成 `cid:` 内联附件（`multipart/related`）
- 按账户配置签名，新建与回复分别可选是否插入
- 发件人别名：一个账户下可配多个发信地址（信封发件人保持主地址以过 SPF）

**账户接入**

- 用户名密码（IMAP/SMTP，支持 SSL / STARTTLS）
- OAuth2 授权登录（Gmail / Outlook），令牌加密存储、过期前自动刷新，
  刷新失败时账户进入明确的"需重新授权"状态
  —— ⚠ 流程与测试已就绪，但**需要你自己注册 Google Cloud 项目 / Azure 应用**
  并填入客户端凭据，详见 [OAuth2 接入文档](../docs/flymail/m14-oauth.md)

---

## 三种运行方式

### 一、Docker（推荐用于自部署）

```bash
git clone git@github.com:huanfeng/FlyMail2.git
cd FlyMail2

mkdir -p data && chown $(id -u):$(id -g) data   # 首次必做，理由见下
cp .env.example .env
# 必填三项：
#   FLYMAIL_AUTH_JWT_SECRET=$(openssl rand -hex 32)
#   FLYMAIL_CRYPTO_ENCRYPTION_KEY=$(openssl rand -hex 32)
#   FLYMAIL_ADMIN_PASS=<你的管理员密码>

docker compose up -d --build
```

打开 `http://<主机>:8086`，用 `.env` 里的账号密码登录。

> **为什么要先手工建 `data` 目录**：bind mount 的宿主目录若不存在，Docker 会以 **root**
> 把它建出来，而容器按 `.env` 里的 `PUID:PGID`（默认 1000:1000）运行，于是写不进去。
> 更麻烦的是 SQLite 把这个权限错误报成 `unable to open database file: out of memory (14)`
> ——看上去与权限毫无关系。容器启动时会提前探测并给出提示，但预先建好目录可以完全避开它。
> 若目录已经被 Docker 以 root 建出来了，用 `sudo chown -R $(id -u):$(id -g) data` 修正。

> **构建上下文必须是仓库根目录**：`flymail/backend/go.mod` 里有
> `replace flymail-core => ../../core`，编译需要 `core/` 的源码。

要对外暴露，请先读[部署文档的反向代理一节](../docs/flymail/deployment.md#5-反向代理与-https)——
其中**声明可信代理**那一步不是可选项，漏了会让登录限流按代理 IP 计数，一个人触发就锁住整站。

### 二、Windows 桌面端

从 [Releases](https://github.com/huanfeng/FlyMail2/releases) 下载
`FlyMail-amd64-installer.exe` 安装。

- 首次运行自动创建 `admin/admin`，登录后请立即改密码
- 关闭窗口最小化到托盘，后台继续收信；真正退出走托盘右键菜单
- 新邮件弹 Windows 原生通知
- 数据默认在 `%APPDATA%\FlyMail`；想让数据跟着 exe 走，在 exe 同目录放一个
  `portable.txt`（内容任意）即可切换到便携模式

细节见 [桌面端说明](backend/desktop/README.md)。

### 三、从源码运行

需要 Go 1.26+、Node 22+、pnpm。

```powershell
cd flymail
./dev.ps1 start    # 前后端都起在后台，日志在 flymail/logs/
./dev.ps1 status
./dev.ps1 stop
```

后端默认只监听 `127.0.0.1`。

---

## 日常维护

```bash
# 热备份数据库（服务运行中也可执行，不阻塞）
docker compose exec flymail flymail db backup

# 附件另行拷贝
tar czf attachments-$(date +%Y%m%d).tar.gz data/attachments/

# 恢复（必须先停服）
docker compose stop
docker compose run --rm --entrypoint flymail flymail \
    db restore /data/backups/flymail-20260911-183500.db --force
docker compose start

# 忘记管理员密码
docker compose exec flymail flymail db reset-admin-password --admin-pass 新密码
```

数据库**不要直接 `cp`**：SQLite 的一致性保证只在事务边界成立，拷贝一个正在被写入的库
会得到撕裂的快照。`db backup` 用 `VACUUM INTO` 导出一致副本，因此不需要停服。

完整运维与故障排查见[部署文档](../docs/flymail/deployment.md)。

---

## 技术栈

| 层 | 选型 |
|---|---|
| 后端 | Go + Gin + GORM，SQLite（纯 Go 驱动，无需 CGO） |
| 前端 | React + TypeScript + Vite + TanStack Query + Tailwind |
| 桌面端 | Wails v2（复用同一个 Gin 引擎作为 AssetServer，无 TCP 监听、无 CORS） |
| 检索 | SQLite FTS5，应用层 bigram 切分 |
| 实时 | SSE（IMAP IDLE → 后端事件总线 → 浏览器） |

前端产物经 `//go:embed` 嵌入二进制，部署时只有一个可执行文件。

---

## 文档

- [部署与运维](../docs/flymail/deployment.md) —— Docker、反向代理、备份恢复、故障排查
- [路线图](../docs/flymail/roadmap.md) —— 已完成与计划中的里程碑
- [设计文档](backend/DESIGN.md) —— 后端分层与模块划分
- 各里程碑的实现细节与踩坑记录：[`docs/flymail/`](../docs/flymail/)
