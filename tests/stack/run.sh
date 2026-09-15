#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/../.."
docker compose version >/dev/null
docker info >/dev/null

mkdir -p tests/postman/reports tests/stack/reports
rm -f tests/postman/reports/newman.xml tests/stack/reports/authorization.txt

# A unique project and no published ports keep this test away from developer data.
project="umkm-stage1-${RANDOM}-$$"
compose=(docker compose --project-name "$project" --env-file .env.example
  -f infra/docker-compose.yml -f infra/docker-compose.test.yml)
services=(auth-service user-service partnerships-service document-service)

cleanup() {
  status=$?
  trap - EXIT
  if [ "$status" -ne 0 ]; then
    "${compose[@]}" logs --no-color --tail 80 || true
  fi
  "${compose[@]}" --profile check down --volumes --remove-orphans || true
  exit "$status"
}
trap cleanup EXIT

"${compose[@]}" config --quiet
"${compose[@]}" up -d --build --wait --wait-timeout 180 "${services[@]}"
"${compose[@]}" run --rm --no-deps db-fixtures
if "${compose[@]}" run --rm --no-deps \
  -e DATABASE_URL=postgres://umkm_test:stage1_database_only@postgres:5432/postgres?sslmode=disable db-fixtures; then
  echo "ERROR: Fixtures accepted a database outside the test scope." >&2
  exit 1
fi
"${compose[@]}" run --rm --no-deps stack-check prepare

"${compose[@]}" run --build --rm --no-deps newman-check

"${compose[@]}" run --rm --no-deps authorization-check 2>&1 | tee tests/stack/reports/authorization.txt

echo "Re-running migrations and Garage bootstrap, then recreating the containers..."
"${compose[@]}" run --rm --no-deps db-migrate
"${compose[@]}" run --rm --no-deps garage-bootstrap
"${compose[@]}" run --rm --no-deps stack-check verify

# Recreate containers while retaining this test project's database and object volumes.
"${compose[@]}" down
"${compose[@]}" up -d --wait --wait-timeout 180 "${services[@]}"
"${compose[@]}" run --rm --no-deps stack-check verify
echo "Stage 1 stack checks passed."
echo "Stage 2 authorization checks passed."
