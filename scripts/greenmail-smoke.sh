#!/usr/bin/env bash
#
# 在**真实部署形态**下走一遍收发：容器里的 FlyMail ← IMAP → 容器里的 GreenMail。
#
# 与 `e2e.sh` 的分工：那个跑的是宿主机的 `go test`（要求装了 Go，测的是代码）；
# 这个只用 HTTP 接口，测的是**部署出来的那个镜像**——镜像里少装一个 CA 证书、
# entrypoint 的换行符被转换、数据卷权限不对，都只有这条路能发现。
#
# 前置：
#   docker compose -f docker-compose.yml -f docker-compose.greenmail.yml up -d
#
# 用法：
#   ./scripts/greenmail-smoke.sh
#   FLYMAIL_URL=http://127.0.0.1:8086 ADMIN_PASS=xxx ./scripts/greenmail-smoke.sh
set -euo pipefail
cd "$(dirname "$0")/.."

FLYMAIL_URL="${FLYMAIL_URL:-http://127.0.0.1:8086}"
GREENMAIL_API="${GREENMAIL_API:-http://127.0.0.1:3080}"
# 宿主侧的 SMTP 入口（投递测试邮件用）。容器之间走服务名，这里走映射出来的端口。
GREENMAIL_SMTP="${GREENMAIL_SMTP:-smtp://127.0.0.1:3025}"
ADMIN_USER="${ADMIN_USER:-admin}"
# `|| true` 在命令替换**内部**：`set -o pipefail` 让 grep 无匹配时整条管道返回 1，
# 而 `VAR=$(...)` 这条简单命令的退出码就是命令替换的退出码，`set -e` 会直接把脚本带走
# ——零输出退出，下面那句友好提示永远打印不出来。这恰好是新机器上第一次跑的场景。
ADMIN_PASS="${ADMIN_PASS:-$(grep -E '^FLYMAIL_ADMIN_PASS=' .env 2>/dev/null | cut -d= -f2- || true)}"

# 容器之间用服务名互访；这两个值是写进 FlyMail 账户里的，不是给宿主用的
IMAP_HOST="${IMAP_HOST:-greenmail}"
IMAP_PORT="${IMAP_PORT:-3143}"
SMTP_HOST="${SMTP_HOST:-greenmail}"
SMTP_PORT="${SMTP_PORT:-3025}"

# 必须是带 TLD 的地址：后端对 email 字段做了格式校验，smoke@localhost 会被 400 挡掉。
# GreenMail 开了 setup.test.all + auth.disabled，任意域名都能收。
TEST_ADDR="${TEST_ADDR:-smoke@example.com}"
PEER_ADDR="${PEER_ADDR:-box@example.com}"
# 主题保持纯 ASCII：下面要把它拼进查询串，手搓 URL 编码只会引入自己的 bug
SUBJECT="smoke-in-$(date +%H%M%S)"

say() { printf '\n[smoke] %s\n' "$*"; }
fail() { printf '\n[smoke] 失败：%s\n' "$*" >&2; exit 1; }

# 带响应体的请求：curl -f 在 4xx 时只给一个退出码，而接口把原因写在 body 里
# （比如「email 字段格式校验不通过」）。吞掉它等于自己给自己蒙眼。
req() {
  local method="$1" url="$2" body="${3:-}"
  local out status
  if [ -n "$body" ]; then
    out=$(curl -sS -X "$method" "$url" "${AUTH[@]}" -H 'Content-Type: application/json' \
      -d "$body" -w '\n%{http_code}')
  else
    out=$(curl -sS -X "$method" "$url" "${AUTH[@]}" -w '\n%{http_code}')
  fi
  status="${out##*$'\n'}"
  RESP="${out%$'\n'*}"
  case "$status" in
    2*) return 0 ;;
    *) fail "$method $url → HTTP $status\n$RESP" ;;
  esac
}

[ -n "$ADMIN_PASS" ] || fail "拿不到管理员密码，设 ADMIN_PASS 或在 .env 里填 FLYMAIL_ADMIN_PASS"

# ── 0. 两个服务都活着 ────────────────────────────────────────────────────────
say "检查服务"
curl -fsS "$FLYMAIL_URL/api/v1/healthz" >/dev/null || fail "FlyMail 没起来（$FLYMAIL_URL）"
curl -fsS "$GREENMAIL_API/api/service/readiness" >/dev/null || fail "GreenMail 没起来（$GREENMAIL_API）"

# ── 1. 登录 ────────────────────────────────────────────────────────────────
say "登录"
TOKEN=$(curl -fsS -X POST "$FLYMAIL_URL/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" \
  | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
[ -n "$TOKEN" ] || fail "登录没拿到 token"
AUTH=(-H "Authorization: Bearer $TOKEN")

# ── 2. 建账户（已存在就复用）────────────────────────────────────────────────
say "准备邮箱账户 $TEST_ADDR"
req GET "$FLYMAIL_URL/api/v1/accounts"
# `|| true` 不能省：grep 没匹配时返回 1，配上 `set -o pipefail` 会让整条管道非 0，
# 而这里「账户还不存在」是完全正常的分支，不是错误。
ACCT_ID=$(printf '%s' "$RESP" | tr '{' '\n' | grep -F "\"email\":\"$TEST_ADDR\"" \
  | sed -n 's/.*"id":\([0-9]*\).*/\1/p' | head -1 || true)

if [ -z "$ACCT_ID" ]; then
  req POST "$FLYMAIL_URL/api/v1/accounts" "{
      \"name\": \"冒烟测试\",
      \"email\": \"$TEST_ADDR\",
      \"password\": \"smoke\",
      \"imap_host\": \"$IMAP_HOST\", \"imap_port\": $IMAP_PORT, \"imap_security\": \"none\",
      \"smtp_host\": \"$SMTP_HOST\", \"smtp_port\": $SMTP_PORT, \"smtp_security\": \"none\"
    }"
  ACCT_ID=$(printf '%s' "$RESP" | sed -n 's/.*"id":\([0-9]*\).*/\1/p' | head -1)
  [ -n "$ACCT_ID" ] || fail "建账户成功但没解析出 id：$RESP"
  say "新建账户 id=$ACCT_ID"
else
  say "复用已有账户 id=$ACCT_ID"
fi

# ── 3. 往 GreenMail 投一封信 ────────────────────────────────────────────────
#
# 走真正的 SMTP 而不是 REST：GreenMail 2.x 的 REST 没有投递端点（只有
# readiness / user 那几个），而且走 SMTP 本来就更接近真实链路。
say "投递测试邮件"
EML=$(mktemp)
trap 'rm -f "$EML"' EXIT
printf 'From: sender@example.com\r\nTo: %s\r\nSubject: %s\r\n\r\n这封信由部署冒烟脚本投递。\r\n' \
  "$TEST_ADDR" "$SUBJECT" > "$EML"
curl -sS --url "$GREENMAIL_SMTP" --mail-from sender@example.com --mail-rcpt "$TEST_ADDR" \
  --upload-file "$EML" >/dev/null || fail "SMTP 投递失败（$GREENMAIL_SMTP）"

# ── 4. 触发同步并等它出现在列表里 ───────────────────────────────────────────
say "触发同步"
curl -sS -X POST "$FLYMAIL_URL/api/v1/accounts/$ACCT_ID/sync" "${AUTH[@]}" >/dev/null || true

# 搜一次并检查状态码。轮询里把错误一律 `|| true` 掉的话，一个确定性的 400
# 会表现成「一直没搜到」——白等满 60 秒，还把人指向错误的方向。
search_hit() {
  local q="$1" out status
  out=$(curl -sS "${AUTH[@]}" "$FLYMAIL_URL/api/v1/search/messages?q=$q&limit=5" -w '\n%{http_code}')
  status="${out##*$'\n'}"
  local body="${out%$'\n'*}"
  [ "${status:0:1}" = 2 ] || fail "搜索接口 HTTP $status\n$body"
  printf '%s' "$body" | grep -qF "$q"
}

say "等待邮件入库（最多 60 秒）"
for i in $(seq 1 60); do
  if search_hit "$SUBJECT"; then
    say "✓ 收到了：$SUBJECT（第 ${i} 秒）"
    FOUND=1
    break
  fi
  sleep 1
done
[ "${FOUND:-0}" = 1 ] || fail "60 秒内没在列表里看到这封邮件"

# ── 5. 反向：用 FlyMail 发一封 ──────────────────────────────────────────────
#
# 发给账户自己：这样一次就把「SMTP 发得出去」和「IMAP 收得回来」两条链路
# 一起验了，也不必依赖 GreenMail REST 有没有「查某个邮箱」的端点。
say "从 FlyMail 发信（收件人是账户自己）"
OUT_SUBJECT="smoke-out-$(date +%H%M%S)"
req POST "$FLYMAIL_URL/api/v1/send" "{
    \"account_id\": $ACCT_ID,
    \"to\": [\"$TEST_ADDR\"],
    \"subject\": \"$OUT_SUBJECT\",
    \"body_html\": \"<p>由部署冒烟脚本发出。</p>\"
  }"

say "等待这封信经 SMTP → GreenMail → IMAP 绕回来（最多 60 秒）"
for i in $(seq 1 30); do
  curl -sS -X POST "$FLYMAIL_URL/api/v1/accounts/$ACCT_ID/sync" "${AUTH[@]}" >/dev/null 2>&1 || true
  if search_hit "$OUT_SUBJECT"; then
    say "✓ 绕回来了（第 $((i * 2)) 秒）"
    SENT=1
    break
  fi
  sleep 2
done
[ "${SENT:-0}" = 1 ] || fail "60 秒内没看到自己发出的那封信绕回来"

say "全部通过：部署形态下收发链路正常"
