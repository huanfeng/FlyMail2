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

数据是 SQLite 单文件加附件目录，备份即拷贝：

```bash
# 冷备份（推荐：停服后拷贝，保证一致性）
docker compose stop
tar czf flymail-$(date +%Y%m%d).tar.gz data/
docker compose start

# 恢复
docker compose down
rm -rf data && tar xzf flymail-YYYYMMDD.tar.gz
docker compose up -d
```

> 热备份需要 `sqlite3 .backup`，当前运行镜像里没装 `sqlite3`（只有静态二进制）。
> 若要支持不停服备份，需在 Dockerfile 运行阶段加 `apk add sqlite`。

---

## 5. 配置项

全部通过环境变量注入，命名规则为 `FLYMAIL_` + 配置路径大写、`.` 换成 `_`
（如 `auth.jwt_secret` → `FLYMAIL_AUTH_JWT_SECRET`）。

| 变量 | 默认 | 说明 |
|---|---|---|
| `FLYMAIL_AUTH_JWT_SECRET` | **必填** | JWT 签名密钥，变更会使所有会话失效 |
| `FLYMAIL_CRYPTO_ENCRYPTION_KEY` | **必填** | 邮箱凭证 AES 加密密钥，**存入账户后不可再变更** |
| `FLYMAIL_ADMIN_USER` / `_PASS` | `admin` / 必填 | 仅首次启动（数据库不存在时）生效 |
| `FLYMAIL_BIND` / `FLYMAIL_PORT` | `0.0.0.0` / `8086` | 宿主机监听地址与端口（8080 已被占用） |
| `PUID` / `PGID` | `1000` | 容器运行身份，保证 `./data` 文件属主正常 |
| `FLYMAIL_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `FLYMAIL_LOG_FORMAT` | `json` | `json` 便于检索，`console` 便于人读 |
| `TZ` | `Asia/Shanghai` | 时区 |

> **`FLYMAIL_SERVER_HOST` 必须是 `0.0.0.0`**（已在 Dockerfile 中固定）。
> 应用默认只监听 `127.0.0.1`，在容器里那样设置会导致端口映射不通。

---

## 6. 故障排查

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

## 7. 待办

- [ ] 反向代理 + HTTPS（目前是明文 HTTP，仅限内网测试）
- [ ] 镜像推送到 registry，免去服务器上构建
- [ ] 与 GreenMail 编排到一起，支持在服务器上跑 E2E（现有 `docker-compose.e2e.yml`）
      —— 注意它把 GreenMail 的 REST API 映射到宿主 `8080`，而服务器上该端口已被占用，
      迁移时需要改端口映射
