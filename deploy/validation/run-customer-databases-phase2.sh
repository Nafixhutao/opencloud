#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
validation_id="${OPENCLOUD_VALIDATION_ID:-019f8d57}"
postgres_name="opencloud-phase2-customer-pg-${validation_id}"
mariadb_name="opencloud-phase2-customer-maria-${validation_id}"
network_name="opencloud-phase2-customer-net-${validation_id}"
go_cache_volume="opencloud-phase2-customer-go-cache-${validation_id}"
go_mod_volume="opencloud-phase2-customer-go-mod-${validation_id}"
validation_password="validation-only-customer-database-password"

case "$validation_id" in
  *[!a-zA-Z0-9_-]* | "") echo "invalid validation id" >&2; exit 2 ;;
esac

for container in "$postgres_name" "$mariadb_name"; do
  if docker container inspect "$container" >/dev/null 2>&1; then
    echo "refusing to reuse existing container: $container" >&2
    exit 1
  fi
done
if docker network inspect "$network_name" >/dev/null 2>&1; then
  echo "refusing to reuse existing network: $network_name" >&2
  exit 1
fi
for volume in "$go_cache_volume" "$go_mod_volume"; do
  if docker volume inspect "$volume" >/dev/null 2>&1; then
    echo "refusing to reuse existing volume: $volume" >&2
    exit 1
  fi
done

cleanup() {
  docker container rm -f "$postgres_name" "$mariadb_name" >/dev/null 2>&1 || true
  docker network rm "$network_name" >/dev/null 2>&1 || true
  docker volume rm "$go_cache_volume" "$go_mod_volume" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker volume create --label "opencloud.validation=${validation_id}" "$go_cache_volume" >/dev/null
docker volume create --label "opencloud.validation=${validation_id}" "$go_mod_volume" >/dev/null
docker network create \
  --label "opencloud.validation=${validation_id}" \
  "$network_name" >/dev/null

docker run -d \
  --name "$postgres_name" \
  --network "$network_name" \
  --label "opencloud.validation=${validation_id}" \
  --tmpfs /var/lib/postgresql \
  -e POSTGRES_USER=opencloud \
  -e "POSTGRES_PASSWORD=${validation_password}" \
  -e POSTGRES_DB=postgres \
  --health-cmd "pg_isready -U opencloud -d postgres" \
  --health-interval 2s \
  --health-timeout 2s \
  --health-retries 40 \
  postgres:18-alpine >/dev/null

docker run -d \
  --name "$mariadb_name" \
  --network "$network_name" \
  --label "opencloud.validation=${validation_id}" \
  --tmpfs /var/lib/mysql \
  -e "MARIADB_ROOT_PASSWORD=${validation_password}" \
  --health-cmd "healthcheck.sh --connect --innodb_initialized" \
  --health-interval 2s \
  --health-timeout 2s \
  --health-retries 60 \
  mariadb:11.8.8 >/dev/null

for container in "$postgres_name" "$mariadb_name"; do
  for _ in $(seq 1 80); do
    if [ "$(docker inspect -f '{{.State.Health.Status}}' "$container")" = "healthy" ]; then
      break
    fi
    sleep 1
  done
  test "$(docker inspect -f '{{.State.Health.Status}}' "$container")" = "healthy"
done

docker run --rm \
  --network "$network_name" \
  -e DATABASE_PROVISIONER_INTEGRATION=1 \
  -e "CUSTOMER_POSTGRES_ADMIN_URL=postgres://opencloud:${validation_password}@${postgres_name}:5432/postgres?sslmode=disable" \
  -e "CUSTOMER_POSTGRES_HOST=${postgres_name}" \
  -e CUSTOMER_POSTGRES_PORT=5432 \
  -e "CUSTOMER_MARIADB_ADMIN_DSN=root:${validation_password}@tcp(${mariadb_name}:3306)/?parseTime=true" \
  -e "CUSTOMER_MARIADB_HOST=${mariadb_name}" \
  -e CUSTOMER_MARIADB_PORT=3306 \
  -v "${repo_root}:/src:ro" \
  -v "${go_cache_volume}:/root/.cache/go-build" \
  -v "${go_mod_volume}:/go/pkg/mod" \
  -w /src/backend \
  golang:1.26.5-bookworm \
  go test ./internal/provisioner \
    -run TestSQLDatabaseProvisionerPostgresAndMariaDBLifecycle \
    -count=1

echo "Phase 2 disposable PostgreSQL/MariaDB customer lifecycle validation passed."
