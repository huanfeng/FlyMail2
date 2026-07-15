#!/usr/bin/env bash
# FlyMail 后端 E2E（Linux/CI，需 Docker）。Windows 本机请用 ./e2e.ps1。
set -euo pipefail
cd "$(dirname "$0")"

KEEP_UP="${KEEP_UP:-0}"
COMPOSE="docker compose -f docker-compose.e2e.yml"

echo "[e2e] starting GreenMail..."
$COMPOSE up -d

echo "[e2e] waiting for GreenMail REST API (:8080)..."
for i in $(seq 1 30); do
  if curl -fsS "http://localhost:8080/api/service/readiness" >/dev/null 2>&1; then
    echo "[e2e] GreenMail ready"; break
  fi
  sleep 1
  if [ "$i" -eq 30 ]; then echo "[e2e] GreenMail not ready, abort"; $COMPOSE logs; exit 1; fi
done

echo "[e2e] running tests..."
set +e
( cd flymail/backend && E2E_GREENMAIL=1 go test ./internal/e2e/... -p 1 -count=1 -timeout 300s -v )
RC=$?
set -e

if [ "$KEEP_UP" = "1" ]; then
  echo "[e2e] KEEP_UP=1, leaving GreenMail running"
else
  $COMPOSE down
fi
exit $RC
