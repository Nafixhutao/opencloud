# Database — PostgreSQL & Redis

PostgreSQL is the **system of record** — and the job queue (ADR 0002); Redis is
a disposable cache, session store, and rate limiter. Data access rules are part of the contract
([`../CLAUDE.md`](../CLAUDE.md)); this is the deep dive.

**Stack:** PostgreSQL 16 · Bun ORM (`uptrace/bun`) · Redis 8.

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

-- users: belong to an account, carry a role
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id    UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT,                          -- NULL for OAuth-only users (ADR 0005)
    role          TEXT NOT NULL DEFAULT 'customer'
                  CHECK (role IN ('customer','admin')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_account_id ON users(account_id);

-- user_identities: linked OAuth logins (Google, GitHub — ADR 0005).
-- A user may have several; password + socials coexist on one user.
CREATE TABLE user_identities (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         TEXT NOT NULL CHECK (provider IN ('google','github')),
    provider_user_id TEXT NOT NULL,              -- provider's stable subject/id
    email            TEXT,                       -- email as reported by the provider
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_user_id)
);
CREATE INDEX idx_user_identities_user_id ON user_identities(user_id);

-- nodes: hosting servers running Hestia
CREATE TABLE nodes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hostname    TEXT NOT NULL UNIQUE,
    status      TEXT NOT NULL DEFAULT 'online'
                CHECK (status IN ('online','draining','offline')),
    capacity    INT  NOT NULL DEFAULT 0,
    used        INT  NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- sites: a customer website on a node
CREATE TABLE sites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id  UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    node_id     UUID NOT NULL REFERENCES nodes(id),
    domain      TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'provisioning'
                CHECK (status IN ('provisioning','active','suspended','failed','deleting')),
    php_version TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
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
    run_at      TIMESTAMPTZ NOT NULL DEFAULT now(),  -- future = retry backoff
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
    actor_id    UUID REFERENCES users(id) ON DELETE SET NULL,
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

- Tooling: Bun migrate, run via `cmd/migrate` (`up`, `down`, `status`).
- Files are **timestamped** and live in `backend/migrations/`:
  `20260630120000_create_sites.up.sql` / `.down.sql`.
- **Never edit a shipped migration** — add a new one. Production is forward-only.
- Every migration is reviewed and reversible where practical; destructive
  migrations are called out in the PR.
- Migrations run as a deploy step **before** the new app version starts. See
  [`DEPLOYMENT.md`](DEPLOYMENT.md).

```bash
go run ./cmd/migrate up        # apply pending
go run ./cmd/migrate status    # show state
go run ./cmd/migrate down      # roll back one (dev)
```

## 6. Indexing & performance

- Index every foreign key and every column you filter or sort on.
- Verify hot queries with `EXPLAIN ANALYZE`; watch for sequential scans on big tables.
- Paginate anything that can grow (keyset/cursor for large sets); never return
  unbounded lists.
- Composite indexes match query shape (e.g. `(account_id, created_at DESC)`).
- Open transactions late, commit early; **never hold a transaction across a Hestia
  call** — provisioning is async and goes through the queue.

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

- PostgreSQL: scheduled `pg_dump`/PITR; restores are rehearsed and documented in a
  runbook before they're needed.
- Redis: persistence (AOF) is for warm restart convenience, not as a source of
  truth — recovery always assumes PostgreSQL is authoritative.
- Customer data on nodes is backed up by Hestia; see [`HOSTING.md`](HOSTING.md).
