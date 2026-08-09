#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
validation_id="${OPENCLOUD_VALIDATION_ID:-phase3}"
postgres_name="opencloud-phase3-pg-${validation_id}"
network_name="opencloud-phase3-net-${validation_id}"
go_cache_volume="opencloud-phase3-go-cache-${validation_id}"
go_mod_volume="opencloud-phase3-go-mod-${validation_id}"
database_url="postgres://opencloud:opencloud@${postgres_name}:5432/opencloud?sslmode=disable"
psql_database_url="postgres://opencloud:opencloud@${postgres_name}:5432/opencloud_psql?sslmode=disable"

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
for volume in "$go_cache_volume" "$go_mod_volume"; do
  if docker volume inspect "$volume" >/dev/null 2>&1; then
    echo "refusing to reuse existing volume: $volume" >&2
    exit 1
  fi
done

cleanup() {
  docker container rm -f "$postgres_name" >/dev/null 2>&1 || true
  docker network rm "$network_name" >/dev/null 2>&1 || true
  docker volume rm "$go_cache_volume" "$go_mod_volume" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker volume create --label "opencloud.validation=${validation_id}" "$go_cache_volume" >/dev/null
docker volume create --label "opencloud.validation=${validation_id}" "$go_mod_volume" >/dev/null
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

go_in_validation() {
  docker run --rm \
    --network "$network_name" \
    -e "DATABASE_URL=${database_url}" \
    -v "${repo_root}:/src" \
    -v "${go_cache_volume}:/root/.cache/go-build" \
    -v "${go_mod_volume}:/go/pkg/mod" \
    -w /src/backend \
    golang:1.26.5 \
    sh -c "$1"
}

psql_value() {
  docker run --rm --network "$network_name" postgres:18-alpine \
    psql "$database_url" -v ON_ERROR_STOP=1 -Atc "$1"
}

psql_file() {
  docker run --rm -i --network "$network_name" postgres:18-alpine \
    psql "$1" -v ON_ERROR_STOP=1 -f - < "$2"
}

schema_hash_for_url() {
  docker run --rm --network "$network_name" postgres:18-alpine \
    pg_dump "$1" --schema-only --no-owner --no-privileges |
    sed '/^\\restrict /d; /^\\unrestrict /d' |
    sha256sum |
    awk '{print $1}'
}

schema_hash() {
  schema_hash_for_url "$database_url"
}

# Exercise the same direct-psql path used by frontend CI. Explicit transaction
# boundaries in Phase 3 must survive up/down/up without relying on Bun.
psql_value "CREATE DATABASE opencloud_psql" >/dev/null
for migration in "$repo_root"/backend/migrations/*.up.sql; do
  psql_file "$psql_database_url" "$migration" >/dev/null
done
psql_schema_up="$(schema_hash_for_url "$psql_database_url")"
test "$(docker run --rm --network "$network_name" postgres:18-alpine psql "$psql_database_url" -Atc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('domains','hostname_claims')")" = "2"
psql_file "$psql_database_url" "$repo_root/backend/migrations/20260730010000_create_domains.down.sql" >/dev/null
test "$(docker run --rm --network "$network_name" postgres:18-alpine psql "$psql_database_url" -Atc "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('domains','hostname_claims')")" = "0"
psql_file "$psql_database_url" "$repo_root/backend/migrations/20260730010000_create_domains.up.sql" >/dev/null
test "$psql_schema_up" = "$(schema_hash_for_url "$psql_database_url")"

go_in_validation "go test ./migrations -run TestCommittedMigrationChecksums -count=1"
go_in_validation "go run ./cmd/migrate up"
test "$(psql_value "SELECT count(*) = count(DISTINCT group_id) FROM bun_migrations")" = "t"
schema_up="$(schema_hash)"

psql_value "
  INSERT INTO accounts (id, name, status) VALUES
    ('00000000-0000-4000-8000-000000000301', 'phase3 owner', 'active'),
    ('00000000-0000-4000-8000-000000000302', 'phase3 other', 'active');
  INSERT INTO account_memberships (account_id, user_id, role, status)
  VALUES ('00000000-0000-4000-8000-000000000301', 'phase3-migration-user', 'customer', 'active');
  INSERT INTO audit_logs (account_id, actor_id, action, target)
  VALUES ('00000000-0000-4000-8000-000000000301', 'phase3-migration-user', 'migration.phase3.sentinel', 'phase3-migration-sentinel');
  INSERT INTO nodes (
    id, hostname, backend, status, capacity_sites, used_sites, provider_metadata
  ) VALUES (
    '00000000-0000-4000-8000-000000000303', 'phase3-node.invalid',
    'fake', 'online', 10, 2, '{}'::jsonb
  );
  INSERT INTO sites (
    id, account_id, node_id, domain, image, internal_port,
    memory_bytes, nano_cpus, status
  ) VALUES (
    '00000000-0000-4000-8000-000000000304',
    '00000000-0000-4000-8000-000000000301',
    '00000000-0000-4000-8000-000000000303',
    'phase3-primary.example.com', 'opencloud/site-static:phase2',
    8080, 134217728, 250000000, 'active'
  );
" >/dev/null

psql_value "
  INSERT INTO domains (
    id, account_id, site_id, hostname, status,
    verification_token_digest, verification_expires_at
  ) VALUES (
    '00000000-0000-4000-8000-000000000315',
    '00000000-0000-4000-8000-000000000301',
    '00000000-0000-4000-8000-000000000304',
    'phase3-primary.example.com', 'pending', decode(repeat('11', 32), 'hex'),
    now() + interval '1 hour'
  );
" >/dev/null
test "$(psql_value "SELECT count(*) FROM hostname_claims WHERE hostname='phase3-primary.example.com' AND domain_id IS NOT NULL")" = "0"
if psql_value "
  UPDATE domains
  SET status='dns_pending',
      verified_at=now(),
      verification_consumed_at=now()
  WHERE id='00000000-0000-4000-8000-000000000315';
" >/dev/null 2>&1; then
  echo "verified custom domain replaced a live primary hostname claim" >&2
  exit 1
fi

if psql_value "
  INSERT INTO domains (
    id, account_id, site_id, hostname, status,
    verification_token_digest, verification_expires_at
  ) VALUES (
    gen_random_uuid(),
    '00000000-0000-4000-8000-000000000302',
    '00000000-0000-4000-8000-000000000304',
    'cross-tenant.example.com', 'pending', decode(repeat('11', 32), 'hex'),
    now() + interval '1 hour'
  );
" >/dev/null 2>&1; then
  echo "domain composite FK accepted a cross-tenant site" >&2
  exit 1
fi

for invalid_sql in \
  "INSERT INTO domains (id,account_id,site_id,hostname,status,verification_token_digest,verification_expires_at) VALUES (gen_random_uuid(),'00000000-0000-4000-8000-000000000301','00000000-0000-4000-8000-000000000304','expired.example.com','pending',decode(repeat('11',32),'hex'),now()-interval '1 minute')" \
  "INSERT INTO domains (id,account_id,site_id,hostname,status,verification_token_digest,verification_expires_at) VALUES (gen_random_uuid(),'00000000-0000-4000-8000-000000000301','00000000-0000-4000-8000-000000000304','unverified.example.com','dns_pending',decode(repeat('11',32),'hex'),now()+interval '1 hour')" \
  "INSERT INTO domains (id,account_id,site_id,hostname,status,verification_token_digest,verification_expires_at,cert_status) VALUES (gen_random_uuid(),'00000000-0000-4000-8000-000000000301','00000000-0000-4000-8000-000000000304','no-expiry.example.com','pending',decode(repeat('11',32),'hex'),now()+interval '1 hour','active')"; do
  if psql_value "$invalid_sql" >/dev/null 2>&1; then
    echo "Phase 3 domain constraint accepted invalid state" >&2
    exit 1
  fi
done

psql_value "
  INSERT INTO domains (
    id, account_id, site_id, hostname, status,
    verification_token_digest, verification_expires_at
  ) VALUES (
    '00000000-0000-4000-8000-000000000305',
    '00000000-0000-4000-8000-000000000301',
    '00000000-0000-4000-8000-000000000304',
    'reusable.example.com', 'pending', decode(repeat('11', 32), 'hex'),
    now() + interval '1 hour'
  );
  UPDATE domains
  SET status = 'deleted', deleted_at = now()
  WHERE id = '00000000-0000-4000-8000-000000000305';
  INSERT INTO sites (
    id, account_id, node_id, domain, image, internal_port,
    memory_bytes, nano_cpus, status
  ) VALUES (
    '00000000-0000-4000-8000-000000000306',
    '00000000-0000-4000-8000-000000000301',
    '00000000-0000-4000-8000-000000000303',
    'reusable.example.com', 'opencloud/site-static:phase2',
    8080, 134217728, 250000000, 'active'
  );
  INSERT INTO domains (
    id, account_id, site_id, hostname, status,
    verification_token_digest, verification_expires_at,
    verified_at, verification_consumed_at
  ) VALUES (
    '00000000-0000-4000-8000-000000000307',
    '00000000-0000-4000-8000-000000000301',
    '00000000-0000-4000-8000-000000000304',
    'live-domain.example.com', 'dns_pending', decode(repeat('22', 32), 'hex'),
    now() + interval '1 hour', now(), now()
  );
  INSERT INTO jobs (id, account_id, kind, status, payload)
  VALUES
    (gen_random_uuid(), '00000000-0000-4000-8000-000000000301', 'reconcile_site', 'queued', jsonb_build_object('site_id', '00000000-0000-4000-8000-000000000304')),
    (gen_random_uuid(), '00000000-0000-4000-8000-000000000301', 'provision_domain', 'queued', jsonb_build_object('domain_id', '00000000-0000-4000-8000-000000000307'));
" >/dev/null

if psql_value "
  INSERT INTO sites (
    id, account_id, node_id, domain, image, internal_port,
    memory_bytes, nano_cpus, status
  ) VALUES (
    gen_random_uuid(),
    '00000000-0000-4000-8000-000000000301',
    '00000000-0000-4000-8000-000000000303',
    'live-domain.example.com', 'opencloud/site-static:phase2',
    8080, 134217728, 250000000, 'active'
  );
" >/dev/null 2>&1; then
  echo "site primary hostname reused a live custom domain" >&2
  exit 1
fi

if psql_value "
  INSERT INTO jobs (id, account_id, kind, status, payload)
  VALUES (
    gen_random_uuid(),
    '00000000-0000-4000-8000-000000000301',
    'provision_domain', 'running',
    jsonb_build_object('domain_id', '00000000-0000-4000-8000-000000000307')
  );
" >/dev/null 2>&1; then
  echo "active domain job uniqueness accepted duplicate work" >&2
  exit 1
fi

psql_value "
  BEGIN;
  INSERT INTO domains (
    id, account_id, site_id, hostname, status,
    verification_token_digest, verification_expires_at
  ) VALUES (
    '00000000-0000-4000-8000-000000000310',
    '00000000-0000-4000-8000-000000000301',
    '00000000-0000-4000-8000-000000000304',
    'rollback.example.com', 'pending', decode(repeat('44', 32), 'hex'),
    now() + interval '1 hour'
  );
  ROLLBACK;
" >/dev/null
test "$(psql_value "SELECT count(*) FROM hostname_claims WHERE hostname='rollback.example.com'")" = "0"

psql_value "
  INSERT INTO sites (
    id, account_id, node_id, domain, image, internal_port,
    memory_bytes, nano_cpus, status
  ) VALUES (
    '00000000-0000-4000-8000-000000000311',
    '00000000-0000-4000-8000-000000000301',
    '00000000-0000-4000-8000-000000000303',
    'cascade-primary.example.com', 'opencloud/site-static:phase2',
    8080, 134217728, 250000000, 'active'
  );
  INSERT INTO domains (
    id, account_id, site_id, hostname, status,
    verification_token_digest, verification_expires_at,
    verified_at, verification_consumed_at
  ) VALUES (
    '00000000-0000-4000-8000-000000000312',
    '00000000-0000-4000-8000-000000000301',
    '00000000-0000-4000-8000-000000000311',
    'cascade-custom.example.com', 'dns_pending', decode(repeat('55', 32), 'hex'),
    now() + interval '1 hour', now(), now()
  );
  DELETE FROM sites WHERE id='00000000-0000-4000-8000-000000000311';
" >/dev/null
test "$(psql_value "SELECT count(*) FROM hostname_claims WHERE hostname IN ('cascade-primary.example.com','cascade-custom.example.com')")" = "0"
test "$(psql_value "SELECT count(*) FROM domains WHERE id='00000000-0000-4000-8000-000000000312'")" = "0"

psql_value "
  INSERT INTO sites (
    id, account_id, node_id, domain, image, internal_port,
    memory_bytes, nano_cpus, status
  ) VALUES (
    '00000000-0000-4000-8000-000000000313',
    '00000000-0000-4000-8000-000000000301',
    '00000000-0000-4000-8000-000000000303',
    'soft-delete-primary.example.com', 'opencloud/site-static:phase2',
    8080, 134217728, 250000000, 'active'
  );
  INSERT INTO domains (
    id, account_id, site_id, hostname, status,
    verification_token_digest, verification_expires_at,
    verified_at, verification_consumed_at
  ) VALUES (
    '00000000-0000-4000-8000-000000000314',
    '00000000-0000-4000-8000-000000000301',
    '00000000-0000-4000-8000-000000000313',
    'soft-delete-custom.example.com', 'dns_pending', decode(repeat('66', 32), 'hex'),
    now() + interval '1 hour', now(), now()
  );
  UPDATE sites
  SET status='deleted', deleted_at=now()
  WHERE id='00000000-0000-4000-8000-000000000313';
" >/dev/null
test "$(psql_value "SELECT count(*) FROM hostname_claims WHERE hostname='soft-delete-primary.example.com'")" = "0"
test "$(psql_value "SELECT count(*) FROM hostname_claims WHERE hostname='soft-delete-custom.example.com'")" = "1"
psql_value "
  UPDATE domains
  SET status='deleted', deleted_at=now()
  WHERE id='00000000-0000-4000-8000-000000000314';
" >/dev/null
test "$(psql_value "SELECT count(*) FROM hostname_claims WHERE hostname='soft-delete-custom.example.com'")" = "0"

# Create unverified intent first, then race a primary-site insert against
# verification. Pending intent has no global claim; claim consumption must
# allow exactly one transaction to commit.
psql_value "
  INSERT INTO domains (
    id, account_id, site_id, hostname, status,
    verification_token_digest, verification_expires_at
  ) VALUES (
    '00000000-0000-4000-8000-000000000309',
    '00000000-0000-4000-8000-000000000301',
    '00000000-0000-4000-8000-000000000304',
    'concurrent.example.com', 'pending', decode(repeat('33', 32), 'hex'),
    now() + interval '1 hour'
  );
" >/dev/null
test "$(psql_value "SELECT count(*) FROM hostname_claims WHERE hostname='concurrent.example.com'")" = "0"

set +e
docker run --rm --network "$network_name" postgres:18-alpine \
  psql "$database_url" -v ON_ERROR_STOP=1 -c "
    INSERT INTO sites (
      id, account_id, node_id, domain, image, internal_port,
      memory_bytes, nano_cpus, status
    ) VALUES (
      '00000000-0000-4000-8000-000000000308',
      '00000000-0000-4000-8000-000000000301',
      '00000000-0000-4000-8000-000000000303',
      'concurrent.example.com', 'opencloud/site-static:phase2',
      8080, 134217728, 250000000, 'active'
    );
  " >/dev/null 2>&1 &
site_claim_pid=$!
docker run --rm --network "$network_name" postgres:18-alpine \
  psql "$database_url" -v ON_ERROR_STOP=1 -c "
    UPDATE domains
    SET status='dns_pending',
        verified_at=now(),
        verification_consumed_at=now()
    WHERE id='00000000-0000-4000-8000-000000000309';
  " >/dev/null 2>&1 &
domain_claim_pid=$!
wait "$site_claim_pid"
site_claim_status=$?
wait "$domain_claim_pid"
domain_claim_status=$?
set -e
if { [ "$site_claim_status" -eq 0 ] && [ "$domain_claim_status" -eq 0 ]; } ||
   { [ "$site_claim_status" -ne 0 ] && [ "$domain_claim_status" -ne 0 ]; }; then
  echo "cross-table hostname race did not produce exactly one winner" >&2
  exit 1
fi
test "$(psql_value "SELECT count(*) FROM domains WHERE hostname='concurrent.example.com'")" = "1"
test "$(psql_value "SELECT count(*) FROM hostname_claims WHERE hostname='concurrent.example.com'")" = "1"
test "$(psql_value "SELECT (SELECT count(*) FROM sites WHERE domain='concurrent.example.com') + (SELECT count(*) FROM domains WHERE hostname='concurrent.example.com' AND verified_at IS NOT NULL)")" = "1"

test "$(psql_value "SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname IN ('idx_domains_account_hostname_live','idx_domains_caddy_permission','idx_domains_reconcile','idx_domains_site_account_fk','idx_sites_reconcile','idx_jobs_active_domain_kind')")" = "6"
test "$(psql_value "SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname IN ('sites_hostname_claim','domains_hostname_claim','sites_hostname_claim_delete','domains_hostname_claim_delete')")" = "4"
test "$(psql_value "SELECT count(*) FROM hostname_claims WHERE hostname IN ('phase3-primary.example.com','reusable.example.com','live-domain.example.com')")" = "3"
test "$(psql_value "SELECT convalidated FROM pg_constraint WHERE conrelid='domains'::regclass AND conname='domains_site_account_fk'")" = "t"
test "$(psql_value "SELECT count(*) FROM pg_constraint WHERE conrelid='hostname_claims'::regclass AND contype='f' AND convalidated AND condeferrable")" = "2"
test "$(psql_value "SELECT count(*) FROM hostname_claims c WHERE (c.site_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sites s WHERE s.id=c.site_id AND s.deleted_at IS NULL)) OR (c.domain_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM domains d WHERE d.id=c.domain_id AND d.deleted_at IS NULL))")" = "0"

go_in_validation "go run ./cmd/migrate up"
test "$schema_up" = "$(schema_hash)"
test "$(psql_value "SELECT count(*) FROM audit_logs WHERE action='migration.phase3.sentinel'")" = "1"
test "$(psql_value "SELECT count(*) FROM domains WHERE id='00000000-0000-4000-8000-000000000307'")" = "1"

# migrate down rolls back only the newest migration group. Slices added after
# Phase 3 may have appended groups, so roll back until the domains schema is
# gone before asserting the pre-Phase 3 (databases-slice) state.
for _ in $(seq 1 8); do
  if [ "$(psql_value "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='domains'")" = "0" ]; then
    break
  fi
  go_in_validation "go run ./cmd/migrate down"
done
test "$(psql_value "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='domains'")" = "0"
test "$(psql_value "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='hostname_claims'")" = "0"
test "$(psql_value "SELECT count(*) FROM sites WHERE id IN ('00000000-0000-4000-8000-000000000304','00000000-0000-4000-8000-000000000306')")" = "2"
test "$(psql_value "SELECT count(*) FROM audit_logs WHERE action='migration.phase3.sentinel'")" = "1"
test "$(psql_value "SELECT count(*) FROM jobs WHERE kind='reconcile_site'")" = "1"
test "$(psql_value "SELECT count(*) FROM jobs WHERE kind IN ('verify_domain','provision_domain','deprovision_domain','reconcile_domain','observe_domain_certificate')")" = "0"
test "$(psql_value "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('databases','database_credentials')")" = "2"
test "$(psql_value "SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='sites' AND column_name='last_reconciled_at'")" = "0"

go_in_validation "go run ./cmd/migrate up"
test "$schema_up" = "$(schema_hash)"
test "$(psql_value "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='domains'")" = "1"
test "$(psql_value "SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='sites' AND column_name='last_reconciled_at'")" = "1"
test "$(psql_value "SELECT count(*) FROM domains")" = "0"
test "$(psql_value "SELECT count(*) FROM sites WHERE id IN ('00000000-0000-4000-8000-000000000304','00000000-0000-4000-8000-000000000306')")" = "2"
test "$(psql_value "SELECT count(*) FROM audit_logs WHERE action='migration.phase3.sentinel'")" = "1"
test "$(psql_value "SELECT convalidated AND pg_get_constraintdef(oid) LIKE '%verify_domain%' AND pg_get_constraintdef(oid) LIKE '%observe_domain_certificate%' FROM pg_constraint WHERE conrelid='jobs'::regclass AND conname='jobs_kind_check'")" = "t"

# Keep the pre-Phase 3 sentinel row while making it ineligible for the worker
# claim used by the isolated service integration tests below.
psql_value "UPDATE jobs SET status='succeeded', updated_at=now() WHERE kind='reconcile_site'" >/dev/null
go_in_validation "go test ./internal/service -run TestPhase3 -count=1"

echo "Phase 3 PostgreSQL migration and integration validation passed."
