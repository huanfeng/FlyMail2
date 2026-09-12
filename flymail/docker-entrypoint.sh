#!/bin/sh
#
# FlyMail 容器入口：首次启动时初始化数据库与管理员账户，然后交棒给主程序。
#
set -e

DATA_DIR="${FLYMAIL_DATA_DIR:-/data}"
DB_FILE="${DATA_DIR}/flymail.db"

mkdir -p "${DATA_DIR}" 2>/dev/null || true

# 数据目录可写是一切的前提，而它出错时的表现极具误导性：
# bind mount 的宿主目录若不存在，Docker 会以 **root** 建出来，而容器按 PUID:PGID 运行，
# 于是写不进去——SQLite 把这个权限错误报成 `unable to open database file: out of memory (14)`，
# 任何人第一次照着 README 走都会卡在这句毫无指向性的话上。
# 这里提前说清楚，别让下一个人再去查 SQLite 的错误码。
# 用真写一下来探测，而不是 `test -w`：后者只看权限位，
# 挂载成只读（:ro）时权限位可能完全正常而写入照样失败。
if ! touch "${DATA_DIR}/.write-probe" 2>/dev/null; then
    echo "错误：数据目录 ${DATA_DIR} 不可写（当前身份 uid=$(id -u) gid=$(id -g)）。" >&2
    echo "" >&2
    echo "多半是宿主机上的挂载目录属主不对——它由 Docker 以 root 建出来，而容器以 PUID:PGID 运行。" >&2
    echo "在仓库目录下执行：" >&2
    echo "    mkdir -p data && sudo chown -R \$(id -u):\$(id -g) data" >&2
    echo "或者让 .env 里的 PUID / PGID 与该目录的属主一致（id -u / id -g）。" >&2
    echo "若该目录是只读挂载（:ro），去掉这个标志。" >&2
    exit 1
fi
rm -f "${DATA_DIR}/.write-probe"

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
