#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
validation_id="${OPENCLOUD_VALIDATION_ID:-phase3-frontend}"
postgres_name="opencloud-phase3-frontend-pg-${validation_id}"
node_name="opencloud-phase3-frontend-node-${validation_id}"
psql_name="opencloud-phase3-frontend-psql-${validation_id}"
network_name="opencloud-phase3-frontend-net-${validation_id}"
database_url="postgres://opencloud:opencloud@${postgres_name}:5432/opencloud?sslmode=disable"

case "$validation_id" in
  *[!a-zA-Z0-9_-]* | "") echo "invalid validation id" >&2; exit 2 ;;
esac

for container in "$postgres_name" "$node_name" "$psql_name"; do
  if docker container inspect "$container" >/dev/null 2>&1; then
    echo "refusing to reuse existing container: $container" >&2
    exit 1
  fi
done
if docker network inspect "$network_name" >/dev/null 2>&1; then
  echo "refusing to reuse existing network: $network_name" >&2
  exit 1
fi

cleanup() {
  docker container rm -f "$node_name" "$psql_name" "$postgres_name" >/dev/null 2>&1 || true
  docker network rm "$network_name" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker network create --label "opencloud.validation=${validation_id}" "$network_name" >/dev/null
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

psql_file() {
  docker run --rm -i \
    --name "$psql_name" \
    --network "$network_name" \
    --label "opencloud.validation=${validation_id}" \
    postgres:18-alpine \
    psql "$database_url" -v ON_ERROR_STOP=1 -f - < "$1"
}

node_in_validation() {
  docker exec "$node_name" "$@"
}

docker run -d \
  --name "$node_name" \
  --network "$network_name" \
  --label "opencloud.validation=${validation_id}" \
  --user node \
  --tmpfs /work:rw,exec,size=2g,uid=1000,gid=1000,mode=0755 \
  -e "DATABASE_URL=${database_url}" \
  -e BETTER_AUTH_SECRET=phase3-test-secret-at-least-32-characters \
  -e BETTER_AUTH_URL=http://localhost:3000 \
  -e API_URL=http://127.0.0.1:8080 \
  -e ENV=test \
  -e MAIL_PROVIDER=memory \
  -e NEXT_TELEMETRY_DISABLED=1 \
  -v "${repo_root}:/src:ro" \
  -w /work \
  node:22-alpine \
  sh -c "tar -C /src \
    --exclude='.git' \
    --exclude='.env' \
    --exclude='.env.*' \
    --exclude='node_modules' \
    --exclude='.next' \
    --exclude='*.tsbuildinfo' \
    --exclude='coverage' \
    --exclude='out' \
    -cf - . | tar -C /work -xf - && exec sleep infinity" >/dev/null

node_in_validation npm ci
for migration in "$repo_root"/backend/migrations/*.up.sql; do
  psql_file "$migration" >/dev/null
done
node_in_validation npm run auth:migrate
node_in_validation npm run auth:migrate
test "$(docker run --rm --name "$psql_name" --network "$network_name" postgres:18-alpine psql "$database_url" -Atc "SELECT count(*) FROM information_schema.tables WHERE table_schema='auth'")" -ge 4

node_in_validation npm run test:auth
node_in_validation npm run test:ui
node_in_validation npm run auth:check-providers
node_in_validation npm run lint
node_in_validation npx tsc --noEmit
node_in_validation env ENV=build MAIL_PROVIDER=log npm run build
node_in_validation npm audit --audit-level=high

echo "Phase 3 frontend, BFF, auth, build, and audit validation passed."
