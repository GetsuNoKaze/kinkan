#!/usr/bin/env sh
# Node e2e: builds mikan-node, starts real mihomo clients and runs the tests.
# KEEP=1 leaves the stack running for debugging.
set -eu
cd "$(dirname "$0")"
export MSYS_NO_PATHCONV=1
if [ -n "${E2E_IMAGE:-}" ]; then
  export COMPOSE_FILE=compose.yaml:compose.published.yaml
fi

docker volume create mikan-gomod >/dev/null
docker volume create mikan-gocache >/dev/null
docker compose up -d driver
if [ -n "${E2E_IMAGE:-}" ]; then
  # The driver creates the shared socket volume first; the released node runs as 65532.
  docker compose exec -T driver chown 65532:65532 /run/mikan
fi
docker compose exec -T driver go run ./test/e2e/gen -out /work
docker compose up -d --build node target client-a client-b
sleep 3

status=0
docker compose exec -T driver go test -tags e2e -count=1 -timeout=5m -run "${E2E_RUN:-.}" -v ./test/e2e/ || status=$?
if [ "$status" = 0 ] && [ "${E2E_RESTART:-0}" = 1 ]; then
  docker compose restart node
  sleep 3
  docker compose exec -T -e MIKAN_E2E_RESTORED=1 driver go test -tags e2e -count=1 -timeout=5m -run TrustTunnel -v ./test/e2e/ || status=$?
fi
if [ "$status" != 0 ]; then
  docker compose logs --tail 80 node
fi
if [ "${KEEP:-0}" != 1 ]; then
  docker compose down -v --remove-orphans
fi
exit "$status"
