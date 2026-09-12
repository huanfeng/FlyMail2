# FlyMail Docker 部署与测试环境

> 创建：2026-08-11
> 测试服务器：`dufeng@develop.server`，部署目录 `/home/dufeng/docker/flymail`
> 访问地址：`http://develop.server:8086`

日常功能测试统一在这个环境进行，本地 `dev.ps1` 仅用于开发时的快速迭代。

---

## 1. 组成

| 文件 | 位置 | 作用 |
|---|---|---|
| `flymail/Dockerfile` | 仓库内 | 三阶段构建：pnpm 构建前端 → Go 静态编译 → alpine 运行镜像 |
| `flymail/docker-entrypoint.sh` | 仓库内 | 首次启动时执行 `db init` 创建管理员，之后直接启动服务 |
| `docker-compose.yml` | 仓库根 | 服务编排、端口映射、数据卷、环境变量 |
| `.dockerignore` | 仓库根 | 排除 `ref/`、`mail2im/`、`node_modules` 等，避免上下文膨胀 |
| `.env.example` | 仓库根 | 配置模板 |
| `.env` | **仅服务器，不入库** | 实际密钥与端口配置 |

### 构建上下文的硬约束

构建上下文**必须是仓库根目录**。`flymail/backend/go.mod` 中有 `replace flymail-core => ../../core`，
编译时需要 `core/` 的源码与相对目录结构。因此 `docker-compose.yml` 放在仓库根，
`context: .` + `dockerfile: flymail/Dockerfile`。

### 镜像的三个阶段

1. **frontend**（`node:22-alpine`）：`pnpm install --frozen-lockfile` → `pnpm run build`。
   注意工作目录必须是 `/src/flymail/frontend`，因为 `vite.config.ts` 的 `outDir` 是相对路径
   `../backend/web/dist`。
2. **backend**（`golang:1.26-alpine`）：`CGO_ENABLED=0`（SQLite 驱动是纯 Go 的
   `glebarez/sqlite` + `modernc.org/sqlite`）、`GOWORK=off`（根 `go.work` 含未拷贝的 mail2im 模块）。
   前端产物必须在 `go build` **之前** COPY 到 `web/dist`，因为 `web/embed.go` 用
   `//go:embed all:dist` 嵌入，该目录不入库，缺失会直接编译失败。
3. **运行**（`alpine:3.21`）：只含静态二进制 + `ca-certificates`（IMAP/SMTP TLS 需要）+ `tzdata`。

---

## 2. 首次部署

```bash
ssh dufeng@develop.server
cd /home/dufeng/docker/flymail

git clone git@github.com:huanfeng/FlyMail2.git .

cp .env.example .env
# 填写三项必填值：
#   FLYMAIL_AUTH_JWT_SECRET=$(openssl rand -hex 32)
#   FLYMAIL_CRYPTO_ENCRYPTION_KEY=$(openssl rand -hex 32)
#   FLYMAIL_ADMIN_PASS=<管理员密码>
chmod 600 .env

mkdir -p data
docker compose up -d --build
```

首次启动会自动执行 `flymail db init` 创建管理员账户。浏览器打开
`http://develop.server:8086` 用 `.env` 里的账号密码登录。

---

## 3. 日常测试流程

```bash
# 本地：提交并推送
git push

# 服务器：拉取 + 重建 + 重启
ssh dufeng@develop.server
cd /home/dufeng/docker/flymail
git pull && docker compose up -d --build
docker compose logs -f
```

数据在 `./data` 卷里，重建镜像不会丢数据。

---

## 4. 常用运维命令

```bash
cd /home/dufeng/docker/flymail

docker compose ps                    # 状态（含健康检查结果）
docker compose logs -f --tail=100    # 实时日志
docker compose restart               # 重启
docker compose down                  # 停止并移除容器（数据保留）

# 应用自身的文件日志（比容器日志更完整，含轮转）
tail -f data/logs/flymail.log

# 重置管理员密码
docker compose exec flymail flymail db reset-admin-password --admin-pass 新密码

# 健康检查
curl -s http://127.0.0.1:8086/api/v1/healthz
```

### 备份与恢复

数据分两部分：SQLite 单文件（`data/flymail.db`）与附件目录（`data/attachments`）。
附件写入后不再修改，直接拷贝即可；数据库则**不能直接 `cp`**——SQLite 的一致性保证只在
事务边界上成立，拷贝一个正在被写入的库会得到撕裂的快照。

因此数据库备份走内建命令，它用 `VACUUM INTO` 在一个读事务里导出一份整理过的副本，
**服务运行中执行也安全**，不阻塞其他读者：

```bash
# 热备份（无需停服）。默认落在 data/backups/flymail-<时间戳>.db
docker compose exec flymail flymail db backup

# 指定路径
docker compose exec flymail flymail db backup --output /data/backups/before-upgrade.db

# 附件另行拷贝（普通文件，无一致性问题）
tar czf attachments-$(date +%Y%m%d).tar.gz data/attachments/
```

恢复必须停服——运行中的进程仍持有旧文件句柄，它后续的写入会覆盖掉刚恢复的内容：

```bash
docker compose stop

# 校验并恢复。现有库会先改名保留为 flymail.db.bak-<时间戳>，不会被直接删除
docker compose run --rm --entrypoint flymail flymail \
    db restore /data/backups/flymail-20260911-183500.db --force

docker compose start
```

`restore` 在覆盖前会做两道校验：`PRAGMA integrity_check` 确认文件完好，再检查
`admin_users` / `accounts` / `messages` 三张表是否存在，确认这确实是一个 FlyMail 库——
任一不过就原样退出，不动现有数据。恢复后还会清理目标库的 `-wal` / `-shm` / `-journal`
残留：那些旁文件属于被替换掉的旧库，留在原地会被 SQLite 当成新库的未提交事务重放。

定期备份可以挂 cron，顺手清掉超过 30 天的旧档：

```cron
0 4 * * * cd /opt/flymail && docker compose exec -T flymail flymail db backup \
          && find data/backups -name 'flymail-*.db' -mtime +30 -delete
```

> 若要连同附件做完整的冷备份，停服后 `tar czf flymail-$(date +%Y%m%d).tar.gz data/`
> 仍然是最省事的办法，恢复时整个 `data/` 覆盖回去即可。

---


### SMTP 发信：认证相关的两个已知行为

- **服务器不要求认证时不再发 AUTH**（内网 postfix 中继、本地 MTA）。
  此前无条件发 AUTH，而明文链路上 net/smtp 的 PlainAuth 拒绝交凭证，
  这类服务器因此一封信都发不出去。现在按 RFC 4954 先看 EHLO 的回应。
  老式的 `AUTH=LOGIN` 形式广告也会被认出来（net/smtp 按空格切 key，精确查 "AUTH" 查不到）。
- **安全模式选 `none` 时的凭证策略**：`none` 在本实现里是**机会式 STARTTLS**（能升就升），
  所以它的语义是「自动」而不是「我要明文」。据此分两种情形：
  - 服务器**没有**广告 STARTTLS（真正的明文中继）→ 按配置发送凭证，日志记一条警告
  - 服务器广告了 STARTTLS 但升级失败 → 按**降级**处理，拒绝发送凭证。
    那正是主动 MITM 打断 TLS 握手就能把凭证降级出来的情形，日志里会记下 TLS 失败的原因。

> **已知残留风险（低）**：Go 的 `net/smtp` 存 EHLO 扩展 key 时保留原样，
> 查询时却先转大写，所以服务器若回小写的 `250-starttls` / `250-auth login`
> （RFC 5321 允许大小写不敏感）会被漏判。现实中的 SMTP 服务器几乎一律用大写，
> 且 `connect()` 与认证走的是同一次判断、同样漏判，**内部是自洽的**——
> 不存在「连接层升级了、认证层却以为没提供」这种更糟的错配。
> 加密处境已收敛到 `connState`（`core/smtp/smtp.go`）一处，
> 将来真要加大小写兜底也只需改那一个地方。

## 5. 反向代理与 HTTPS

对外暴露时把 `FLYMAIL_BIND` 改成 `127.0.0.1`，只让反向代理能连到容器端口，
再由代理终止 TLS。

### 必须先做的一件事：声明可信代理

```bash
# .env
FLYMAIL_SERVER_TRUSTED_PROXIES=127.0.0.1        # 代理与容器同机
# 或 172.16.0.0/12                               # 代理也在 Docker 网络里
```

不填的后果不是"少了个功能"：FlyMail 默认不信任任何代理（gin 的默认是信任所有，
已显式收紧），于是**所有请求的客户端 IP 都是代理自身**。M12 的登录限流按 IP 计数，
一个人连错 11 次密码就会把整站锁住；结构化日志里的来源 IP 也全是同一个，追查失去意义。

反过来，不走反代时**必须保持为空**——信任所有代理等于让任何客户端自己伪造
`X-Forwarded-For` 绕过限流。

### Caddy（推荐）

自动申请并续期证书，流式响应默认不缓冲，基本零配置：

```caddyfile
mail.example.com {
    reverse_proxy 127.0.0.1:8086
}
```

### nginx

```nginx
server {
    listen 443 ssl http2;
    server_name mail.example.com;

    ssl_certificate     /etc/letsencrypt/live/mail.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/mail.example.com/privkey.pem;

    # 附件上传。默认 1m 会让稍大的附件以 413 失败
    client_max_body_size 25m;

    location / {
        proxy_pass http://127.0.0.1:8086;
        proxy_http_version 1.1;

        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE 实时推送是一条长连接，默认 60s 读超时会让它每分钟断一次重连
        proxy_read_timeout 3600s;
    }
}

server {
    listen 80;
    server_name mail.example.com;
    return 301 https://$host$request_uri;
}
```

> **关于 SSE 的缓冲**：nginx 默认 `proxy_buffering on` 会把流式响应攒着不发，
> 实时推送表现为"过几十秒一次性涌出一批"。这里不需要额外配置——
> `internal/sse/handler.go` 已经在响应头里发了 `X-Accel-Buffering: no`，
> nginx 会据此对该响应关闭缓冲。**若换用其他代理**（HAProxy、Traefik、
> 云厂商的 7 层负载均衡），先确认它是否认这个头，不认的话要在代理侧显式关掉缓冲。

---

## 6. 配置项

全部通过环境变量注入，命名规则为 `FLYMAIL_` + 配置路径大写、`.` 换成 `_`
（如 `auth.jwt_secret` → `FLYMAIL_AUTH_JWT_SECRET`）。

| 变量 | 默认 | 说明 |
|---|---|---|
| `FLYMAIL_AUTH_JWT_SECRET` | **必填** | JWT 签名密钥，变更会使所有会话失效 |
| `FLYMAIL_CRYPTO_ENCRYPTION_KEY` | **必填** | 邮箱凭证 AES 加密密钥，**存入账户后不可再变更** |
| `FLYMAIL_ADMIN_USER` / `_PASS` | `admin` / 必填 | 仅首次启动（数据库不存在时）生效 |
| `FLYMAIL_BIND` / `FLYMAIL_PORT` | `0.0.0.0` / `8086` | 宿主机监听地址与端口（8080 已被占用） |
| `FLYMAIL_SERVER_TRUSTED_PROXIES` | 空 | 可信反向代理的 IP/CIDR，逗号分隔。走反代时必填，详见第 5 节 |
| `PUID` / `PGID` | `1000` | 容器运行身份，保证 `./data` 文件属主正常 |
| `FLYMAIL_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `FLYMAIL_LOG_FORMAT` | `json` | `json` 便于检索，`console` 便于人读 |
| `TZ` | `Asia/Shanghai` | 时区 |

> **`FLYMAIL_SERVER_HOST` 必须是 `0.0.0.0`**（已在 Dockerfile 中固定）。
> 应用默认只监听 `127.0.0.1`，在容器里那样设置会导致端口映射不通。

---

## 7. 故障排查

| 现象 | 原因与处理 |
|---|---|
| 容器起来但端口不通 | 检查 `FLYMAIL_SERVER_HOST` 是否为 `0.0.0.0`；默认值 `127.0.0.1` 在容器内只能自己访问 |
| `bad interpreter: /bin/sh^M` | entrypoint 被转成 CRLF。仓库根 `.gitattributes` 已强制 `*.sh eol=lf`，Dockerfile 里另有 `sed` 兜底；若仍出现，检查文件是否绕过 git 传输 |
| `pattern all:dist: no matching files` | 前端构建产物未就位。检查 Dockerfile 中 `COPY --from=frontend` 是否在 `go build` 之前 |
| 登录后立刻失效 / token 报错 | `FLYMAIL_AUTH_JWT_SECRET` 未生效。该 key 必须在 `config.go` 中 `SetDefault` 过，否则 viper 的 `AutomaticEnv` 不读取它 |
| 已保存的邮箱账户无法连接、报解密失败 | `FLYMAIL_CRYPTO_ENCRYPTION_KEY` 被改过。只能删除账户重新添加 |
| `./data` 下文件读不了 | `PUID`/`PGID` 与宿主机用户不一致，用 `id -u` / `id -g` 核对 |
| 构建极慢 | 服务器 Docker 用的是 `vfs` 存储驱动（无写时复制），首次构建耗时较长属正常 |

---

## 8. 待办

- [x] 反向代理 + HTTPS —— 配置样例见第 5 节；测试服务器仍是明文 HTTP，公网暴露前按该节配置
- [x] 镜像推送到 registry —— `.github/workflows/release.yml` 打 tag 后推 GHCR，
      服务器可改用 `docker compose pull` 免去本地构建（**流水线本身尚未在真实 tag 上跑过**）
- [x] 与 GreenMail 编排到一起，支持在服务器上跑 E2E
      —— `docker-compose.greenmail.yml`（叠加编排）+ `scripts/greenmail-smoke.sh`。
      与 `e2e.sh` 分工：那个跑宿主机的 `go test`（测代码，要求装 Go），
      这个只用 HTTP 接口测**部署出来的镜像本身**（镜像缺 CA 证书、entrypoint 换行符
      被转换、数据卷权限不对，都只有这条路能发现）。
      —— 端口只绑 `127.0.0.1`（GreenMail 关闭了鉴权，绝不能让它对局域网可见），
      默认 SMTP 3025 / IMAP 3143 / REST 3080，都可用环境变量覆盖
- [x] 镜像体积核对（M15 目标 < 50MB）：2026-09-12 本机实测 **41.9MB**。
      `release.yml` 也会把实测值打进日志，
      也可在服务器上 `docker images flymail:local` 直接看
