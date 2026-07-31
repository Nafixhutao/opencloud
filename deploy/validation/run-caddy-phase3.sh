#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
validation_id="${OPENCLOUD_VALIDATION_ID:-phase3-caddy}"
postgres_name="opencloud-phase3-caddy-pg-${validation_id}"
redis_name="opencloud-phase3-caddy-redis-${validation_id}"
api_name="opencloud-phase3-caddy-api-${validation_id}"
caddy_name="opencloud-phase3-caddy-${validation_id}"
client_name="opencloud-phase3-caddy-client-${validation_id}"
psql_name="opencloud-phase3-caddy-psql-${validation_id}"
probe_name="opencloud-phase3-caddy-probe-${validation_id}"
network_name="opencloud-phase3-caddy-net-${validation_id}"
caddy_data_volume="opencloud-phase3-caddy-data-${validation_id}"
go_cache_volume="opencloud-phase3-caddy-go-cache-${validation_id}"
go_mod_volume="opencloud-phase3-caddy-go-mod-${validation_id}"
database_url="postgres://opencloud:opencloud@${postgres_name}:5432/opencloud?sslmode=disable"
phase3_step="preflight"

case "$validation_id" in
  *[!a-zA-Z0-9_-]* | "") echo "invalid validation id" >&2; exit 2 ;;
esac

for container in \
  "$postgres_name" "$redis_name" "$api_name" "$caddy_name" \
  "$client_name" "$psql_name" "$probe_name"; do
  if docker container inspect "$container" >/dev/null 2>&1; then
    echo "refusing to reuse existing container: $container" >&2
    exit 1
  fi
done
if docker network inspect "$network_name" >/dev/null 2>&1; then
  echo "refusing to reuse existing network: $network_name" >&2
  exit 1
fi
for volume in "$caddy_data_volume" "$go_cache_volume" "$go_mod_volume"; do
  if docker volume inspect "$volume" >/dev/null 2>&1; then
    echo "refusing to reuse existing volume: $volume" >&2
    exit 1
  fi
done

cleanup() {
  docker container rm -f \
    "$probe_name" "$client_name" "$caddy_name" "$api_name" \
    "$psql_name" "$redis_name" "$postgres_name" >/dev/null 2>&1 || true
  docker network rm "$network_name" >/dev/null 2>&1 || true
  docker volume rm \
    "$caddy_data_volume" "$go_cache_volume" "$go_mod_volume" >/dev/null 2>&1 || true
}
on_error() {
  status=$?
  set +e
  echo "Phase 3 Caddy validation failed at step: ${phase3_step}" >&2
  docker logs "$api_name" >&2 || true
  docker logs "$caddy_name" >&2 || true
  exit "$status"
}
trap cleanup EXIT
trap on_error ERR

docker network create --label "opencloud.validation=${validation_id}" "$network_name" >/dev/null
for volume in "$caddy_data_volume" "$go_cache_volume" "$go_mod_volume"; do
  docker volume create --label "opencloud.validation=${validation_id}" "$volume" >/dev/null
done

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
docker run -d \
  --name "$redis_name" \
  --network "$network_name" \
  --label "opencloud.validation=${validation_id}" \
  redis:8-alpine >/dev/null

for _ in $(seq 1 40); do
  if [ "$(docker inspect -f '{{.State.Health.Status}}' "$postgres_name")" = "healthy" ]; then
    break
  fi
  sleep 1
done
test "$(docker inspect -f '{{.State.Health.Status}}' "$postgres_name")" = "healthy"
for _ in $(seq 1 30); do
  if docker exec "$redis_name" redis-cli ping 2>/dev/null | grep -qx PONG; then
    break
  fi
  sleep 1
done
test "$(docker exec "$redis_name" redis-cli ping)" = "PONG"

psql_file() {
  docker run --rm -i \
    --name "$psql_name" \
    --network "$network_name" \
    --label "opencloud.validation=${validation_id}" \
    postgres:18-alpine \
    psql "$database_url" -v ON_ERROR_STOP=1 -f - < "$1"
}

psql_command() {
  docker run --rm \
    --name "$psql_name" \
    --network "$network_name" \
    --label "opencloud.validation=${validation_id}" \
    postgres:18-alpine \
    psql "$database_url" -v ON_ERROR_STOP=1 -c "$1"
}

for migration in "$repo_root"/backend/migrations/*.up.sql; do
  psql_file "$migration" >/dev/null
done
psql_command "
  INSERT INTO accounts (id, name, status)
  VALUES ('00000000-0000-4000-8000-000000000401', 'Phase 3 Caddy', 'active');
  INSERT INTO nodes (id, hostname, backend, status, capacity_sites, used_sites, provider_metadata)
  VALUES (
    '00000000-0000-4000-8000-000000000402', 'phase3-caddy-node.example.com',
    'fake', 'online', 10, 1, '{}'::jsonb
  );
  INSERT INTO sites (
    id, account_id, node_id, domain, image, internal_port,
    memory_bytes, nano_cpus, status
  ) VALUES (
    '00000000-0000-4000-8000-000000000403',
    '00000000-0000-4000-8000-000000000401',
    '00000000-0000-4000-8000-000000000402',
    'primary.example.com', 'opencloud/site-static:phase2',
    8080, 134217728, 250000000, 'active'
  );
  INSERT INTO domains (
    id, account_id, site_id, hostname, status,
    verification_token_digest, verification_expires_at,
    verification_consumed_at, verified_at, cert_status
  ) VALUES
    (
      '00000000-0000-4000-8000-000000000404',
      '00000000-0000-4000-8000-000000000401',
      '00000000-0000-4000-8000-000000000403',
      'allowed.example.com', 'active', decode(repeat('77', 32), 'hex'),
      now() + interval '1 hour', now(), now(), 'none'
    ),
    (
      '00000000-0000-4000-8000-000000000405',
      '00000000-0000-4000-8000-000000000401',
      '00000000-0000-4000-8000-000000000403',
      'failclosed.example.com', 'active', decode(repeat('88', 32), 'hex'),
      now() + interval '1 hour', now(), now(), 'none'
    );
" >/dev/null

docker run -d \
  --name "$api_name" \
  --network "$network_name" \
  --network-alias opencloud-phase3-api \
  --label "opencloud.validation=${validation_id}" \
  -e ENV=development \
  -e HTTP_ADDR=:8080 \
  -e METRICS_ADDR=:9090 \
  -e "DATABASE_URL=${database_url}" \
  -e "REDIS_URL=redis://${redis_name}:6379/0" \
  -e DOMAINS_ENABLED=true \
  -e DOMAIN_INGRESS_IPV4=8.8.8.8 \
  -e DOMAIN_DNS_RESOLVER=1.1.1.1:53 \
  -e DOMAIN_VERIFICATION_KEY=MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE= \
  -v "${repo_root}:/src:ro" \
  -v "${go_cache_volume}:/root/.cache/go-build" \
  -v "${go_mod_volume}:/go/pkg/mod" \
  -w /src/backend \
  golang:1.26.5 \
  go run ./cmd/api >/dev/null

phase3_step="API readiness"
api_ready=false
for _ in $(seq 1 180); do
  if docker run --rm \
    --name "$client_name" \
    --network "$network_name" \
    caddy:2.10.2-alpine \
    wget -qO- "http://opencloud-phase3-api:8080/healthz" >/dev/null 2>&1; then
    api_ready=true
    break
  fi
  sleep 1
done
if [ "$api_ready" != "true" ]; then
  docker logs "$api_name" >&2 || true
  echo "Phase 3 API did not become ready" >&2
  exit 1
fi
docker run --rm \
  --name "$client_name" \
  --network "$network_name" \
  caddy:2.10.2-alpine \
  wget -qO- "http://opencloud-phase3-api:8080/healthz" >/dev/null

phase3_step="official Caddy config validation"
docker run --rm \
  --name "$client_name" \
  --network "$network_name" \
  -v "${repo_root}/deploy/caddy/caddy.json:/config/caddy.json:ro" \
  caddy:2.10.2-alpine \
  caddy validate --config /config/caddy.json >/dev/null

docker run --rm \
  --name "$client_name" \
  --network "$network_name" \
  -v "${repo_root}/deploy/validation/Caddyfile.phase3:/etc/caddy/Caddyfile:ro" \
  caddy:2.10.2-alpine \
  caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null

docker run -d \
  --name "$caddy_name" \
  --network "$network_name" \
  --network-alias allowed.example.com \
  --network-alias primary.example.com \
  --network-alias unknown.example.com \
  --network-alias failclosed.example.com \
  --label "opencloud.validation=${validation_id}" \
  -v "${repo_root}/deploy/validation/Caddyfile.phase3:/etc/caddy/Caddyfile:ro" \
  -v "${caddy_data_volume}:/data" \
  caddy:2.10.2-alpine \
  caddy run --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null

phase3_step="Caddy admin readiness"
caddy_ready=false
for _ in $(seq 1 60); do
  if docker exec "$caddy_name" wget -qO- http://127.0.0.1:2019/config/ >/dev/null 2>&1; then
    caddy_ready=true
    break
  fi
  sleep 1
done
test "$caddy_ready" = "true"
docker exec "$caddy_name" wget -qO- http://127.0.0.1:2019/config/ >/dev/null

permission_response() {
  docker exec "$caddy_name" \
    wget -S -O /dev/null \
    "http://opencloud-phase3-api:9090/caddy/permission?domain=$1" 2>&1 || true
}

permission_status() {
  permission_response "$1" | awk '
    {
      for (field = 1; field <= NF; field++) {
        if ($field ~ /^HTTP\/[0-9.]+$/ && $(field + 1) ~ /^[0-9][0-9][0-9]$/) {
          code = $(field + 1)
        }
      }
    }
    END { print code }
  '
}

tls_probe() {
  docker run --rm \
    --name "$probe_name" \
    --network "$network_name" \
    --label "opencloud.validation=${validation_id}" \
    -v "${repo_root}:/src:ro" \
    -v "${caddy_data_volume}:/caddy-data:ro" \
    -w /src \
    golang:1.26.5 \
    go run ./deploy/validation/tls-probe.go \
      -address "${caddy_name}:443" \
      -hostname "$1" \
      -ca /caddy-data/caddy/pki/authorities/local/root.crt
}

phase3_step="permission allow and deny"
allowed_status="$(permission_status allowed.example.com)"
primary_status="$(permission_status primary.example.com)"
unknown_status="$(permission_status unknown.example.com)"
echo "CADDY_PERMISSION_STATUS allowed=${allowed_status} primary=${primary_status} unknown=${unknown_status}"
test "$allowed_status" = "200"
test "$primary_status" = "200"
test "$unknown_status" = "403"

phase3_step="authorized exact route and TLS"
allowed_body="$(docker run --rm \
  --name "$client_name" \
  --network "$network_name" \
  caddy:2.10.2-alpine \
  wget --no-check-certificate -qO- https://allowed.example.com/)"
test "$allowed_body" = "opencloud-phase3-exact-route"
tls_probe allowed.example.com

phase3_step="authorized unmatched primary route"
primary_response="$(docker run --rm \
  --name "$client_name" \
  --network "$network_name" \
  caddy:2.10.2-alpine \
  wget --no-check-certificate -S -O /dev/null https://primary.example.com/ 2>&1 || true)"
echo "$primary_response" | grep -Eq 'HTTP/[0-9.]+ 421'
tls_probe primary.example.com

phase3_step="unknown hostname TLS denial"
if tls_probe unknown.example.com >/dev/null 2>&1; then
  echo "unknown hostname unexpectedly received a certificate" >&2
  exit 1
fi

phase3_step="permission database fail closed"
docker stop "$postgres_name" >/dev/null
failclosed_status="$(permission_status failclosed.example.com)"
echo "CADDY_PERMISSION_STATUS failclosed=${failclosed_status}"
test "$failclosed_status" = "503"
if tls_probe failclosed.example.com >/dev/null 2>&1; then
  echo "permission backend failure unexpectedly issued a certificate" >&2
  exit 1
fi

echo "Phase 3 Caddy config, exact routing, permission, and TLS validation passed."
