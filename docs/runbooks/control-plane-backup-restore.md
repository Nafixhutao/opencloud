# Control-plane PostgreSQL backup and restore

This runbook covers OpenCloud's Phase 2 encrypted logical backup baseline. It
does not authorize a production restore. Production restores require an incident
owner, a target backup of the current database, and explicit release approval.

## Guarantees and limits

- `pg_dump` uses PostgreSQL 18 custom format and never writes plaintext to the
  persistent backup destination.
- Archives use chunked authenticated AES-256-GCM encryption and have SHA-256
  sidecars. The checksum detects storage corruption early; GCM is the
  cryptographic authenticity boundary.
- The scheduler runs immediately, then at the configured interval. A Linux
  advisory file lock prevents two processes writing the same destination at
  once and is released automatically on process exit/crash.
- Restore plaintext exists only in the container's bounded ephemeral `/tmp`,
  mode `0600`, and is removed on every return path.
- A local named volume is not disaster recovery. Production needs an encrypted,
  access-controlled off-host destination or immediate off-host replication.
- This baseline is logical backup, not point-in-time recovery. WAL archiving/PITR
  remains later operations work.

## Key custody

Generate a random 32-byte key outside the repository:

```bash
openssl rand -base64 32
```

Store the value in the orchestrator secret manager and inject it as
`CONTROL_PLANE_BACKUP_KEY`. Never print it, put it in shell history, commit it,
or bake it into an image. Losing a key loses every archive encrypted with it.
During rotation, retain the old key until its final archive has expired and a
new-key restore rehearsal has passed.

## Enable the scheduler

Before enabling it, confirm:

1. `/backups` resolves to the intended mode-`0700` destination.
2. The destination is off-host/encrypted or has monitored immediate replication.
3. The secret manager injects a valid backup key.
4. Retention and interval match policy.

Then:

```bash
docker compose --profile backup build control-plane-backup
docker compose --profile backup up -d control-plane-backup
docker compose --profile backup logs --tail=20 control-plane-backup
```

Each successful run emits one JSON record containing only the archive basename,
encrypted SHA-256 digest, creation time, and database name. It never emits a
connection URL or key.

## Verify an archive

Select a basename matching:

```text
opencloud-YYYYMMDDTHHMMSSZ-<16 lowercase hex>.dump.ocb
```

Run catalog verification with the same key that encrypted it:

```bash
docker compose --profile backup run --rm \
  -e BACKUP_FILE=opencloud-YYYYMMDDTHHMMSSZ-0000000000000000.dump.ocb \
  control-plane-backup /app/control-plane-backup verify
```

Verification must succeed both before and after the archive is copied off-host.
Do not accept “file exists” or checksum-only as restore evidence.

## Rehearse a restore

Use an isolated, empty PostgreSQL instance with no production route. Inject its
connection URL as `RESTORE_DATABASE_URL` through the secret mechanism, then run:

```bash
docker compose --profile backup run --rm \
  -e BACKUP_FILE=opencloud-YYYYMMDDTHHMMSSZ-0000000000000000.dump.ocb \
  -e RESTORE_CONFIRM_DATABASE=opencloud_restore_rehearsal \
  -e ALLOW_DESTRUCTIVE_RESTORE=restore-to-confirmed-empty-target \
  control-plane-backup /app/control-plane-backup restore
```

The database name in `RESTORE_CONFIRM_DATABASE` must exactly equal the database
component of `RESTORE_DATABASE_URL`. After restore:

1. Run schema/table counts.
2. Check a known non-sensitive sentinel.
3. Start an isolated API against the restored database and run readiness/login
   smoke tests.
4. Record archive basename, key ID (not key), Git SHA, PostgreSQL versions,
   duration, and result.
5. Destroy the rehearsal database and verify no plaintext temp archive remains.

The repository's disposable proof runs:

```bash
bash deploy/validation/run-control-plane-backup-phase2.sh
```

It must never be pointed at an active OpenCloud database.

## Production restore procedure

Stop before proceeding unless an incident owner has explicitly approved a
production restore.

1. Freeze writes and stop API/worker/dashboard processes that can mutate data.
2. Create and preserve a fresh encrypted backup of the current target database,
   even if it is suspected damaged.
3. Copy the chosen off-host archive and sidecar into an isolated verification
   environment; run `verify` and a full rehearsal first.
4. Confirm PostgreSQL client/server compatibility and available disk space.
5. Record the exact target database name and independently review
   `RESTORE_DATABASE_URL`; never infer it from a shell default.
6. Run restore with both explicit gates shown above.
7. Apply no migrations until the restored Git/application version is identified.
8. Start services in migration → auth-migration → API/worker/dashboard order.
9. Verify `/healthz`, `/readyz`, login, tenant scoping, queue state, and recent
   audit records before reopening traffic.
10. Preserve incident evidence and document the restore result.

If any verification fails, keep traffic closed and restore into another isolated
database for investigation. Never repeatedly restore over the only surviving
copy.
