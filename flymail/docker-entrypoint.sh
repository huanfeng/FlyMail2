#!/bin/sh
#
# FlyMail 容器入口：首次启动时初始化数据库与管理员账户，然后交棒给主程序。
#
set -e

DATA_DIR="${FLYMAIL_DATA_DIR:-/data}"
DB_FILE="${DATA_DIR}/flymail.db"

mkdir -p "${DATA_DIR}"

# 仅在「启动服务」且「数据库尚不存在」时初始化管理员。
# 不能每次启动都跑 db init —— 它内部是 upsert，会把用户后来改过的密码重置回环境变量的值。
if [ "$1" = "server" ] && [ ! -f "${DB_FILE}" ]; then
    ADMIN_USER="${FLYMAIL_ADMIN_USER:-admin}"
    ADMIN_PASS="${FLYMAIL_ADMIN_PASS:-}"

    if [ -z "${ADMIN_PASS}" ]; then
        echo "错误：首次启动需要 FLYMAIL_ADMIN_PASS 来创建管理员账户，请在 .env 中设置后重试。" >&2
        exit 1
    fi

    echo "首次启动：初始化数据库，创建管理员 \"${ADMIN_USER}\""
    flymail db init --admin-user "${ADMIN_USER}" --admin-pass "${ADMIN_PASS}"
fi

exec flymail "$@"
