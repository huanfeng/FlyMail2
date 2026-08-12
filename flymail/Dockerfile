# syntax=docker/dockerfile:1
#
# FlyMail 生产镜像（多阶段构建）
#
# 构建上下文必须是 monorepo 根目录，不能是 flymail/ 或 flymail/backend/：
#   flymail/backend/go.mod 里有 `replace flymail-core => ../../core`，
#   编译时需要 core/ 的源码与相对目录结构。
#
# 构建方式（在仓库根目录）：
#   docker compose build
#   或 docker build -f flymail/Dockerfile -t flymail:local .

# ──────────────────────────────────────────────────────────────
# 阶段 1：前端构建
# ──────────────────────────────────────────────────────────────
FROM node:22-alpine AS frontend

# 目录结构必须与仓库保持一致：vite.config.ts 的 outDir 是相对路径
# `../backend/web/dist`，产物会落到 /src/flymail/backend/web/dist。
WORKDIR /src/flymail/frontend

RUN corepack enable

# 先只拷贝清单文件，让依赖层在源码变动时仍能命中缓存
COPY flymail/frontend/package.json flymail/frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY flymail/frontend/ ./
RUN pnpm run build

# ──────────────────────────────────────────────────────────────
# 阶段 2：后端编译
# ──────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS backend

# CGO_ENABLED=0：SQLite 驱动是纯 Go 实现（glebarez/sqlite + modernc.org/sqlite），
#                无需 CGO，可静态编译成不依赖 libc 的单文件。
# GOWORK=off：仓库根的 go.work 还纳入了 mail2im 各模块，本镜像未拷贝它们；
#             关闭 workspace 模式，改走 go.mod 里的 replace 指令解析 core。
ENV CGO_ENABLED=0 GOWORK=off

WORKDIR /src
COPY core/ ./core/
COPY flymail/backend/ ./flymail/backend/

WORKDIR /src/flymail/backend
RUN go mod download

# backend/web/embed.go 用 `//go:embed all:dist` 嵌入前端产物，
# 该目录不入库（被 .gitignore 排除），必须在 go build 之前就位，否则编译报错。
COPY --from=frontend /src/flymail/backend/web/dist ./web/dist

RUN go build -trimpath -ldflags="-s -w" -o /out/flymail .

# ──────────────────────────────────────────────────────────────
# 阶段 3：运行镜像
# ──────────────────────────────────────────────────────────────
FROM alpine:3.21

# ca-certificates：连接 IMAP/SMTP over TLS 需要根证书
# tzdata：日志与邮件时间按本地时区显示
RUN apk add --no-cache ca-certificates tzdata

COPY --from=backend /out/flymail /usr/local/bin/flymail
COPY flymail/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
# 剥离可能的 CRLF 再赋可执行位：仓库在 Windows 上开发，若换行符被转换，
# 容器启动会以 `bad interpreter: /bin/sh^M` 失败（.gitattributes 是第一道防线，这里是第二道）。
RUN sed -i 's/\r$//' /usr/local/bin/docker-entrypoint.sh \
    && chmod +x /usr/local/bin/docker-entrypoint.sh

# 容器内固定监听 0.0.0.0:8080，对外端口由 compose 的端口映射决定。
# 注意：应用默认只听 127.0.0.1，容器里必须显式设为 0.0.0.0，否则端口映射不通。
ENV FLYMAIL_DATA_DIR=/data \
    FLYMAIL_SERVER_HOST=0.0.0.0 \
    FLYMAIL_SERVER_PORT=8080 \
    FLYMAIL_LOG_CONSOLE=true \
    FLYMAIL_LOG_FORMAT=json

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/api/v1/healthz >/dev/null 2>&1 || exit 1

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["server"]
