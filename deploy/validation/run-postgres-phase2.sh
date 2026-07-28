#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
validation_id="${OPENCLOUD_VALIDATION_ID:-019f8d57}"
postgres_name="opencloud-phase2-pg-${validation_id}"
network_name="opencloud-phase2-net-${validation_id}"
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

go_in_validation() {
  docker run --rm \
    --network "$network_name" \
    -e "DATABASE_URL=${database_url}" \
    -v "${repo_root}:/src" \
    -v "${repo_root}/.gocache:/root/.cache/go-build" \
    -v "${repo_root}/.gomodcache:/go/pkg/mod" \
    -w /src/backend \
    golang:1.26-bookworm \
    sh -c "$1"
}

psql_value() {
  docker run --rm --network "$network_name" postgres:18-alpine \
    psql "$database_url" -v ON_ERROR_STOP=1 -Atc "$1"
}

schema_hash() {
  docker run --rm --network "$network_name" postgres:18-alpine \
    pg_dump "$database_url" --schema-only --no-owner --no-privileges |
    sed '/^\\restrict /d; /^\\unrestrict /d' |
    sha256sum |
    awk '{print $1}'
}

go_in_validation "go run ./cmd/migrate up"
test "$(psql_value "SELECT count(*) = count(DISTINCT group_id) FROM bun_migrations")" = "t"
schema_up="$(schema_hash)"

psql_value "
  INSERT INTO public.accounts (id, name, status)
  VALUES ('00000000-0000-0000-0000-000000000257', 'phase2 migration sentinel', 'active');
  INSERT INTO public.account_memberships (account_id, user_id, role, status)
  VALUES ('00000000-0000-0000-0000-000000000257', 'phase2-migration-sentinel', 'customer', 'active');
  INSERT INTO public.audit_logs (account_id, actor_id, action, target)
  VALUES ('00000000-0000-0000-0000-000000000257', 'phase2-migration-sentinel', 'migration.phase2.sentinel', 'phase2-migration-sentinel');
" >/dev/null

go_in_validation "go run ./cmd/migrate up"
test "$schema_up" = "$(schema_hash)"
test "$(psql_value "SELECT count(*) FROM public.account_memberships WHERE user_id = 'phase2-migration-sentinel'")" = "1"

go_in_validation "go run ./cmd/migrate down"
test "$(psql_value "SELECT count(*) FROM public.account_memberships WHERE user_id = 'phase2-migration-sentinel'")" = "1"
test "$(psql_value "SELECT count(*) FROM public.audit_logs WHERE action = 'migration.phase2.sentinel'")" = "1"
test "$(psql_value "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('nodes','sites','jobs')")" = "0"

go_in_validation "go run ./cmd/migrate up"
test "$schema_up" = "$(schema_hash)"
test "$(psql_value "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('nodes','sites','jobs')")" = "3"
test "$(psql_value "SELECT count(*) FROM public.audit_logs WHERE action = 'migration.phase2.sentinel'")" = "1"

go_in_validation "go test ./internal/service/ ./internal/middleware/ ./internal/handler/ -count=1"

echo "Phase 2 PostgreSQL migration and integration validation passed."
