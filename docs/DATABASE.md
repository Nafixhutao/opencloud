# Database — PostgreSQL & Redis

PostgreSQL is the **system of record** — and the job queue (ADR 0002); Redis is
a disposable cache, session store, and rate limiter. Data access rules are part of the contract
([`../CLAUDE.md`](../CLAUDE.md)); this is the deep dive.

**Stack:** PostgreSQL 18 · Bun ORM (`uptrace/bun`) · Redis 8.

---

## 1. Principles

1. **PostgreSQL holds the only copy of truth.** Anything in Redis must be
   reconstructable from PostgreSQL — never store the sole copy of data in Redis.
2. **Constraints live in the database**, not only in app code: `NOT NULL`,
   `UNIQUE`, foreign keys, `CHECK`. The DB is the last line of integrity defense.
3. **Every customer-owned row carries `account_id`** and every query is scoped by
   it. This is the tenant-isolation boundary, enforced in repositories.
4. **Migrations are the only way to change schema** — forward-only in production,
   reviewed, committed.

## 2. Conventions

| Rule | Detail |
|---|---|
| Table names | `snake_case`, **plural** — `hosting_accounts`, `dns_zones` |
| Column names | `snake_case` — `created_at`, `account_id` |
| Primary key | `id` — UUID (`gen_random_uuid()`) for customer-facing entities |
| Timestamps | `created_at`, `updated_at` as `timestamptz`, stored UTC |
| Soft delete | `deleted_at timestamptz NULL` only where history matters; else hard delete |
| Money | integer minor units (cents) or `NUMERIC` — **never** float |
| Booleans | predicate names — `is_active`, `has_ssl` |
| Foreign keys | always indexed; named `<entity>_id` |
| Enums | text + `CHECK`, or Postgres enum type for stable sets |

No `SELECT *` in production paths — select the columns you need.

## 3. Core schema

The tenant boundary is `accounts`. Customer resources reference both an
`account_id` (who owns it) and a `node_id` (where it lives).

```sql
-- accounts: the tenant boundary
CREATE TABLE accounts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active','suspended','closed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Identity (users, sessions, OAuth links, email verification) is owned by
-- **better-auth** in a separate `auth` schema — `auth.user`, `auth.session`,
-- `auth.account`, `auth.verification` — managed by better-auth's own migrations,
-- NOT Bun (ADR 0006). Bun only bootstraps the empty `auth` schema; it owns the
-- `public.*` domain tables here. `role`
-- lives on `auth.user`; the tenant boundary stays `public.accounts`. Domain rows
-- reference `auth.user.id` by id (no cross-schema FK). Note: better-auth's
-- `auth.account` (a provider credential link) is not the tenant `public.accounts`.

-- nodes: hosting capacity registered by backend driver (Docker or fallback Hestia)
CREATE TABLE nodes (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hostname          TEXT NOT NULL UNIQUE,
    backend           TEXT NOT NULL CHECK (backend IN ('docker','hestia','fake')),
    status            TEXT NOT NULL DEFAULT 'online'
                      CHECK (status IN ('online','draining','offline')),
    capacity_sites    INT  NOT NULL CHECK (capacity_sites > 0),
    used_sites        INT  NOT NULL DEFAULT 0
                      CHECK (used_sites BETWEEN 0 AND capacity_sites),
    provider_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- sites: a customer website on a node
CREATE TABLE sites (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id           UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    node_id              UUID NOT NULL REFERENCES nodes(id),
    domain               TEXT NOT NULL,
    image                TEXT NOT NULL,
    internal_port        INT NOT NULL,
    memory_bytes         BIGINT NOT NULL,
    nano_cpus            BIGINT NOT NULL,
    status               TEXT NOT NULL DEFAULT 'provisioning'
                         CHECK (status IN ('provisioning','active','suspending',
                         'suspended','resuming','deleting','deleted','failed')),
    idempotency_key      TEXT,
    last_error           TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at           TIMESTAMPTZ,
    capacity_released_at TIMESTAMPTZ,
    UNIQUE (domain)
);
CREATE INDEX idx_sites_account_id ON sites(account_id);
CREATE INDEX idx_sites_node_id    ON sites(node_id);

-- jobs: THE async job queue (ADR 0002) — workers claim with FOR UPDATE SKIP LOCKED.
-- Enqueue is an INSERT in the same tx as the triggering write, so resource + job
-- commit or roll back together.
CREATE TABLE jobs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id  UUID REFERENCES accounts(id) ON DELETE SET NULL,
    kind        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'queued'
                CHECK (status IN ('queued','running','succeeded','failed')),
    attempts    INT  NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 5,
    run_at      TIMESTAMPTZ NOT NULL DEFAULT now(),  -- future = retry backoff
    locked_at   TIMESTAMPTZ,
    locked_by   TEXT,
    payload     JSONB NOT NULL,
    last_error  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_jobs_claim ON jobs(status, run_at);  -- matches the worker's claim query

-- audit_logs: append-only record of sensitive actions
CREATE TABLE audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    account_id  UUID REFERENCES accounts(id) ON DELETE SET NULL,
    actor_id    UUID,   -- references auth.user.id (better-auth — ADR 0006); no cross-schema FK
    action      TEXT NOT NULL,
    target      TEXT,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_account_created ON audit_logs(account_id, created_at DESC);
```

Other resource tables (`domains`, `databases`, `mailboxes`, `dns_zones`,
`certificates`, `plans`, `subscriptions`) follow the same pattern: `id`,
`account_id`, optional `node_id`, `status`, timestamps, scoped indexes.

Phase 2 implements `databases` as a tenant-owned desired-state row with
`postgres|mariadb` engine, internal deterministic physical database/user names,
async status, and an account-scoped idempotency key. A separate
`database_credentials` one-to-one row holds only a versioned AES-256-GCM
ciphertext. Its deletion is the durable one-time-reveal marker; plaintext never
enters the control-plane schema. List/detail queries project only safe fields
plus `credential_available` and never return the physical identifiers.
Database checks independently enforce the customer-label and physical-name
allowlists, idempotency-key bound, deleted-status timestamp invariant, and
minimum authenticated-envelope size.

Database jobs extend the existing queue with `provision_database`,
`delete_database`, and compensating `cleanup_database`. Their JSON payload is
exactly a server-generated `database_id`. Provider work occurs outside a
transaction; completion status, encrypted credential publication, audit, and
job completion commit together afterward. Provider calls for the same database
are serialized across workers with a session advisory lock held on a dedicated
connection, without keeping a SQL transaction open during the external call.

## 4. Bun models

Each table maps to a struct in `internal/model`. Keep tags explicit.

```go
type Site struct {
    bun.BaseModel `bun:"table:sites,alias:s"`

    ID         uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()"`
    AccountID  uuid.UUID `bun:"account_id,notnull"`
    NodeID     uuid.UUID `bun:"node_id,notnull"`
    Domain     string    `bun:"domain,notnull"`
    Status     string    `bun:"status,notnull"`
    PHPVersion string    `bun:"php_version"`
    CreatedAt  time.Time `bun:"created_at,notnull,default:now()"`
    UpdatedAt  time.Time `bun:"updated_at,notnull,default:now()"`
}
```

## 5. Migrations

- Domain migrations use Bun via `cmd/migrate` (`up`, `down`, `status`). Bun also
  creates the empty `auth` namespace; Better Auth's migration API exclusively owns the
  tables inside it.
- Files are **timestamped** and live in `backend/migrations/`:
  `20260630120000_create_sites.up.sql` / `.down.sql`.
- **Never edit a shipped migration** — add a new one. Production is forward-only.
- Committed SHA-256 checksums pin every SQL file, with fixed checksums for the
  shipped Phase 1 membership/audit migrations. `cmd/migrate up` assigns each
  pending file its own Bun rollback group, so a development `down` cannot
  accidentally remove the whole migration history.
- Every migration is reviewed and reversible where practical; destructive
  migrations are called out in the PR.
- Migrations run as a deploy step **before** the new app version starts. See
  [`DEPLOYMENT.md`](DEPLOYMENT.md).

```bash
go run ./cmd/migrate up             # domain tables + auth namespace
npm run auth:migrate                # auth tables; run from repo root
go run ./cmd/migrate status         # show Bun-managed state
go run ./cmd/migrate down           # roll back one Bun migration (dev)
```

## 6. Indexing & performance

- Index every foreign key and every column you filter or sort on.
- Verify hot queries with `EXPLAIN ANALYZE`; watch for sequential scans on big tables.
- Paginate anything that can grow (keyset/cursor for large sets); never return
  unbounded lists.
- Composite indexes match query shape (e.g. `(account_id, created_at DESC)`).
- Open transactions late, commit early; **never hold a transaction across a
  hosting-provider call** — provisioning is async and goes through the queue.
- Least-loaded node selection and `used_sites` reservation execute under one
  transaction-scoped PostgreSQL advisory lock. Capacity release is marked on
  each site and can occur exactly once even when delete/cleanup jobs retry.

## 7. Multi-tenancy enforcement

- Repositories add `WHERE account_id = ?` to **every** customer query — see
  [`BACKEND.md`](BACKEND.md#7-repositories-bun).
- Admin cross-account access is a separate, explicit, audited code path.
- Foreign keys to `accounts` make accidental cross-tenant joins structurally hard.

## 8. Redis usage

Redis serves three roles; all are disposable. The **job queue is not one of them** —
it lives in the `jobs` table (§3, [ADR 0002](adr/0002-postgres-backed-job-queue.md))
so enqueueing stays transactional with the system of record.

| Role | Keys / pattern | Notes |
|---|---|---|
| **Cache** | `cache:plans`, `cache:node:{id}:status` | explicit TTLs; invalidate on write |
| **Sessions** | `session:{refresh_token_id}` | refresh-token store; enables revocation/rotation |
| **Rate limit** | `ratelimit:{ip}:{route}` | sliding window counters |

Rules:
- Always set a TTL on cache keys; never let them grow unbounded.
- Cache is invalidated on the corresponding write — stale truth is a bug.
- If Redis is lost, the system degrades (slower, re-login) but never loses data.

## 9. Backups

- The Phase 2 control-plane baseline is an opt-in `control-plane-backup` Compose
  profile. It runs `pg_dump --format=custom --no-owner --no-privileges`
  immediately at startup and then on `BACKUP_INTERVAL_SECONDS` (24 hours by
  default).
- The dump stream is never written plaintext to the backup volume. The Go
  backup binary encrypts 64 KiB chunks with AES-256-GCM, a fresh random nonce
  prefix, monotonic per-chunk nonces, and authenticated header/sequence/length
  metadata. It publishes the encrypted archive and SHA-256 sidecar atomically
  with mode `0600`.
- `BACKUP_ENCRYPTION_KEY` is an external base64-encoded 32-byte secret. Losing
  it makes the archives unrecoverable; rotating it requires retaining the old
  key until every archive encrypted with that key has expired.
- Retention removes only regular files matching OpenCloud's generated archive
  pattern and their regular checksum sidecars. It never traverses directories,
  follows symlinks, or runs broad prune commands.
- Restore first verifies the checksum, authenticates/decrypts to an ephemeral
  mode-`0600` temp file, asks `pg_restore` to parse the archive, and requires
  both an exact target database-name confirmation and the literal destructive
  gate. The rehearsal restores into a second disposable PostgreSQL instance.
- The Compose named volume is a development/staging default, not sufficient
  production disaster recovery. Production must mount an access-controlled,
  encrypted off-host destination (or replicate completed encrypted artifacts
  off-host), monitor scheduler failures, and rehearse from that copy.
- Redis: persistence (AOF) is for warm restart convenience, not as a source of
  truth — recovery always assumes PostgreSQL is authoritative.
- Customer site volumes and databases are backed up by the active provider; see
  [`HOSTING.md`](HOSTING.md).

Operational steps and the destructive restore gate are in
[`runbooks/control-plane-backup-restore.md`](runbooks/control-plane-backup-restore.md).

## 10. Phase 1 tenancy tables

- `public.account_memberships` — `user_id` (better-auth) → `account_id` with
  `role` (customer|admin) and `status` (active|suspended|disabled). Unique on
  `user_id` for MVP single-tenant-per-user.
- `public.audit_logs` — append-only sensitive action trail.
- `public.auth_token_consumptions` stores only SHA-256 email-verification token
  digests to make verification links single-use. Raw tokens and URLs never enter
  the domain database.
- `audit_logs` is append-only at the database boundary: UPDATE and DELETE are
  rejected by triggers. Privileged domain mutations append their audit row in
  the same transaction.

## 11. Phase 2 provisioning tables

Migration `20260726010000_create_provisioning_core` adds `nodes`, `sites`, and
`jobs` without changing any shipped Phase 1 migration. The checksum manifest
pins the new migration too. Site rows retain lifecycle history through
`deleted_at`; the worker stores only generic operational errors in `last_error`,
never credentials or provider secrets. Active site jobs are deduplicated by
`kind` plus the internally generated `payload.site_id`.

Migration `20260727010000_create_customer_databases` adds `databases` and
`database_credentials`, and extends the durable job-kind constraint, without
editing the provisioning-core or any shipped Phase 1 migration. The encrypted
credential row is deleted in the same transaction as its reveal audit, so
concurrent callers have at most one winner. The control-plane backup
implementation does not add schema or alter any shipped migration. No migration
was added or rewritten for provider-operation serialization or dashboard
pagination.
