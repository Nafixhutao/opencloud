#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
validation_id="${OPENCLOUD_VALIDATION_ID:-019f8d57}"
postgres_name="opencloud-phase2-frontend-pg-${validation_id}"
network_name="opencloud-phase2-frontend-net-${validation_id}"
database_url="postgres://opencloud:opencloud@${postgres_name}:5432/opencloud?sslmode=disable"

case "$validation_id" in
  *[!a-zA-Z0-9_-]* | "") echo "invalid validation id" >&2; exit 2 ;;
esac

if docker container inspect "$postgres_name" >/dev/null 2>&1; then
  echo "refusing to reuse existing container: $postgres_name" >&2
  exit 1
fi
if docker network inspect "$network_name" >/dev/null 2>&1; then
  echo "refusing to reuse existing network: $network_name" >&2
  exit 1
fi

cleanup() {
  docker container rm -f "$postgres_name" >/dev/null 2>&1 || true
  docker network rm "$network_name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create \
  --label "opencloud.validation=${validation_id}" \
  "$network_name" >/dev/null
docker run -d \
  --name "$postgres_name" \
  --network "$network_name" \
  --label "opencloud.validation=${validation_id}" \
  --tmpfs /var/lib/postgresql \
  -e POSTGRES_USER=opencloud \
  -e POSTGRES_PASSWORD=opencloud \
  -e POSTGRES_DB=opencloud \
  --health-cmd "pg_isready -U opencloud -d opencloud" \
  --health-interval 2s \
  --health-timeout 2s \
  --health-retries 30 \
  postgres:18-alpine >/dev/null

for _ in $(seq 1 40); do
  if [ "$(docker inspect -f '{{.State.Health.Status}}' "$postgres_name")" = "healthy" ]; then
    break
  fi
  sleep 1
done
test "$(docker inspect -f '{{.State.Health.Status}}' "$postgres_name")" = "healthy"

for migration in "${repo_root}"/backend/migrations/*.up.sql; do
  docker run --rm \
    --network "$network_name" \
    -v "${migration}:/migration.sql:ro" \
    postgres:18-alpine \
    psql "$database_url" -v ON_ERROR_STOP=1 -f /migration.sql >/dev/null
done

docker run --rm \
  --network "$network_name" \
  -v "${repo_root}:/app" \
  -v /app/node_modules \
  -w /app \
  -e ENV=test \
  -e MAIL_PROVIDER=memory \
  -e "DATABASE_URL=${database_url}" \
  -e BETTER_AUTH_SECRET=validation-only-secret-at-least-32-characters \
  -e BETTER_AUTH_URL=http://localhost:3000 \
  node:22-bookworm \
  sh -c "
    npm ci --no-audit &&
    npm run auth:migrate &&
    npm run auth:migrate &&
    npm run test:auth &&
    npm run test:ui &&
    npm run auth:check-providers &&
    npm run lint &&
    npx tsc --noEmit &&
    ENV=build MAIL_PROVIDER=log npm run build
  "

test "$(
  docker run --rm --network "$network_name" postgres:18-alpine \
    psql "$database_url" -Atc \
    "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'auth'"
)" -ge 4

echo "Phase 2 clean frontend install, auth integration, and build validation passed."
