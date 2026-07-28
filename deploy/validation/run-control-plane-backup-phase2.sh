#!/usr/bin/env bash
set -Eeuo pipefail

validation_suffix="${VALIDATION_SUFFIX:-019f8d57}"
if [[ ! "${validation_suffix}" =~ ^[a-z0-9-]{4,32}$ ]]; then
  echo "VALIDATION_SUFFIX must match [a-z0-9-]{4,32}" >&2
  exit 2
fi

source_name="opencloud-backup-source-${validation_suffix}"
restore_name="opencloud-backup-restore-${validation_suffix}"
schedule_name="opencloud-backup-schedule-${validation_suffix}"
network_name="opencloud-backup-net-${validation_suffix}"
volume_name="opencloud-backup-artifacts-${validation_suffix}"
image_name="opencloud-backup-validation:${validation_suffix}"
source_database="opencloud_source"
restore_database="opencloud_restore"
validation_password="validation-only-password"
source_url="postgres://opencloud:${validation_password}@${source_name}:5432/${source_database}?sslmode=disable"
restore_url="postgres://opencloud:${validation_password}@${restore_name}:5432/${restore_database}?sslmode=disable"
validation_stage="initialization"

cleanup() {
  docker rm -f "${source_name}" "${restore_name}" "${schedule_name}" >/dev/null 2>&1 || true
  docker volume rm "${volume_name}" >/dev/null 2>&1 || true
  docker network rm "${network_name}" >/dev/null 2>&1 || true
  docker image rm "${image_name}" >/dev/null 2>&1 || true
}

finish() {
  status=$?
  trap - EXIT
  cleanup
  if [[ "${status}" -ne 0 ]]; then
    printf 'backup_validation_failed_stage=%s status=%d\n' "${validation_stage}" "${status}" >&2
  fi
  exit "${status}"
}

mark_stage() {
  validation_stage="$1"
  printf 'backup_validation_stage=%s\n' "${validation_stage}"
}

wait_for_database() {
  local container="$1"
  local database="$2"

  for _ in $(seq 1 60); do
    # The official PostgreSQL entrypoint briefly starts a temporary server
    # during initialization. pg_isready can succeed against that server just
    # before it is shut down. PID 1 becomes postgres only after initialization
    # is complete, and the SQL probe also proves the requested database exists.
    if docker exec "${container}" sh -euc \
      'test "$(cat /proc/1/comm)" = postgres' >/dev/null 2>&1 &&
      test "$(
        docker exec "${container}" psql \
          -U opencloud \
          -d "${database}" \
          -Atqc 'SELECT 1' 2>/dev/null
      )" = "1"; then
      return 0
    fi
    sleep 1
  done

  docker logs --tail 100 "${container}" >&2 || true
  return 1
}

trap finish EXIT
mark_stage cleanup
cleanup

mark_stage create_network
docker network create "${network_name}" >/dev/null
mark_stage create_volume
docker volume create "${volume_name}" >/dev/null

mark_stage start_source_database
docker run -d \
  --name "${source_name}" \
  --network "${network_name}" \
  -e POSTGRES_USER=opencloud \
  -e "POSTGRES_PASSWORD=${validation_password}" \
  -e "POSTGRES_DB=${source_database}" \
  postgres:18-alpine >/dev/null

mark_stage start_restore_database
docker run -d \
  --name "${restore_name}" \
  --network "${network_name}" \
  -e POSTGRES_USER=opencloud \
  -e "POSTGRES_PASSWORD=${validation_password}" \
  -e "POSTGRES_DB=${restore_database}" \
  postgres:18-alpine >/dev/null

mark_stage "wait_database_${source_name}"
wait_for_database "${source_name}" "${source_database}"
mark_stage "wait_database_${restore_name}"
wait_for_database "${restore_name}" "${restore_database}"

mark_stage apply_migrations
docker run --rm \
  --network "${network_name}" \
  -v "$(pwd)/backend/migrations:/migrations:ro" \
  postgres:18-alpine \
  sh -euc '
    for migration in /migrations/*.up.sql; do
      psql "$1" -v ON_ERROR_STOP=1 -f "$migration" >/dev/null
    done
  ' sh "${source_url}"

sentinel_id="00000000-0000-0000-0000-00000000b257"
docker exec "${source_name}" psql -U opencloud -d "${source_database}" -v ON_ERROR_STOP=1 -c \
  "INSERT INTO accounts (id, name, status) VALUES ('${sentinel_id}', 'backup restore sentinel', 'active')" \
  >/dev/null

mark_stage build_backup_image
docker build --target backup -t "${image_name}" backend >/dev/null
backup_key="$(openssl rand -base64 32)"
mark_stage run_scheduler
docker run -d \
  --name "${schedule_name}" \
  --network "${network_name}" \
  -e "DATABASE_URL=${source_url}" \
  -e "BACKUP_ENCRYPTION_KEY=${backup_key}" \
  -e BACKUP_DIR=/backups \
  -e BACKUP_RETENTION_DAYS=14 \
  -e BACKUP_INTERVAL_SECONDS=300 \
  -e RESTORE_TEMP_DIR=/tmp \
  -v "${volume_name}:/backups" \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=256m,mode=0700,uid=70,gid=70 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  "${image_name}" \
  /app/control-plane-backup schedule >/dev/null

backup_result=""
for _ in $(seq 1 60); do
  backup_result="$(docker logs "${schedule_name}" 2>&1 || true)"
  if [[ "${backup_result}" == *'"file":"opencloud-'* ]]; then
    break
  fi
  if ! docker inspect -f '{{.State.Running}}' "${schedule_name}" | grep -qx true; then
    printf '%s\n' "${backup_result}" >&2
    echo "scheduled backup container exited before publishing an archive" >&2
    exit 3
  fi
  sleep 1
done
docker stop -t 10 "${schedule_name}" >/dev/null
docker rm "${schedule_name}" >/dev/null
backup_file="$(printf '%s' "${backup_result}" | sed -n 's/.*"file":"\([^"]*\)".*/\1/p')"
if [[ ! "${backup_file}" =~ ^opencloud-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{16}\.dump\.ocb$ ]]; then
  echo "backup command did not return a safe archive filename" >&2
  exit 4
fi

mark_stage verify_archive
docker run --rm \
  --network "${network_name}" \
  -e "BACKUP_ENCRYPTION_KEY=${backup_key}" \
  -e BACKUP_DIR=/backups \
  -e "BACKUP_FILE=${backup_file}" \
  -e RESTORE_TEMP_DIR=/tmp \
  -v "${volume_name}:/backups:ro" \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=256m,mode=0700,uid=70,gid=70 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  "${image_name}" \
  /app/control-plane-backup verify >/dev/null

if docker run --rm -v "${volume_name}:/backups:ro" alpine:3.23 \
  grep -a -q "backup restore sentinel" "/backups/${backup_file}"; then
  echo "plaintext sentinel found in encrypted archive" >&2
  exit 5
fi

mark_stage restore_archive
docker run --rm \
  --network "${network_name}" \
  -e "BACKUP_ENCRYPTION_KEY=${backup_key}" \
  -e BACKUP_DIR=/backups \
  -e "BACKUP_FILE=${backup_file}" \
  -e RESTORE_TEMP_DIR=/tmp \
  -e "RESTORE_DATABASE_URL=${restore_url}" \
  -e "RESTORE_CONFIRM_DATABASE=${restore_database}" \
  -e ALLOW_DESTRUCTIVE_RESTORE=restore-to-confirmed-empty-target \
  -v "${volume_name}:/backups:ro" \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=256m,mode=0700,uid=70,gid=70 \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  "${image_name}" \
  /app/control-plane-backup restore >/dev/null

restored_count="$(
  docker exec "${restore_name}" psql -U opencloud -d "${restore_database}" -Atc \
    "SELECT count(*) FROM accounts WHERE id='${sentinel_id}' AND name='backup restore sentinel'"
)"
test "${restored_count}" = "1"

table_count="$(
  docker exec "${restore_name}" psql -U opencloud -d "${restore_database}" -Atc \
    "SELECT count(*) FROM information_schema.tables WHERE table_schema IN ('public','auth')"
)"
test "${table_count}" -ge 7

artifact_listing="$(
  docker run --rm -v "${volume_name}:/backups:ro" alpine:3.23 \
    ls -1A /backups
)"
test "$(printf '%s\n' "${artifact_listing}" | grep -Ec '\.dump\.ocb$')" = "1"
test "$(printf '%s\n' "${artifact_listing}" | grep -Ec '\.dump\.ocb\.sha256$')" = "1"
if printf '%s\n' "${artifact_listing}" | grep -Eq '\.tmp$|\.dump$'; then
  echo "plaintext or temporary backup artifact remained" >&2
  exit 6
fi

mark_stage complete
printf 'backup_restore_rehearsal=passed\n'
