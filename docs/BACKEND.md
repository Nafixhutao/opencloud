# Backend — Go Control Plane

The OpenCloud backend is a Go service that acts as the **system of record** and
orchestrates provider-neutral hosting backends. Docker + Caddy is the MVP backend
and Hestia is a fallback (ADR 0008). This document is the deep dive; the contract lives in
[`../CLAUDE.md`](../CLAUDE.md) and the system map in [`../ARCHITECTURE.md`](../ARCHITECTURE.md).

**Stack:** Go 1.26+ · Gin · Bun ORM · PostgreSQL · Redis · Viper · Zap.

---

## 1. Package layout

```
backend/
├── cmd/
│   ├── api/main.go        # HTTP server entrypoint
│   ├── worker/main.go     # background job worker entrypoint
│   └── migrate/main.go    # migration runner
├── internal/              # private; not importable by other modules
│   ├── config/            # Viper loading → typed Config struct
│   ├── server/            # router wiring + dependency injection
│   ├── handler/           # Gin handlers, one file per domain
│   ├── service/           # business logic + transactions
│   ├── repository/        # Bun data access, account-scoped
│   ├── model/             # domain structs + Bun models
│   ├── provisioner/       # provider-neutral contract + Docker/Hestia/fake adapters
│   ├── middleware/        # auth, request-id, logging, recovery, ratelimit
│   ├── queue/             # Postgres-backed job queue + job handlers
│   ├── dto/               # request/response shapes
│   └── apperr/            # typed application errors
├── pkg/                   # genuinely reusable, import-safe packages
└── migrations/            # SQL migrations (timestamped)
```

`internal/` keeps the app private. `pkg/` is only for code that is safe to import
elsewhere — don't dump app logic there.

## 2. Layering rules

One-directional dependencies. See the diagram in [`../ARCHITECTURE.md`](../ARCHITECTURE.md#4-backend-layering).

| Layer | May call | Must NOT |
|---|---|---|
| handler | service | touch Bun, hold business logic |
| service | repository, provisioner, queue | import Gin, build SQL |
| repository | Bun / PostgreSQL | call services, ignore `account_id` |
| provisioner | Docker/Caddy, Cloudflare, fallback Hestia | be called by anything but a service/worker job |

A handler that needs data calls a service; a service that needs persistence calls
a repository. No shortcuts.

## 3. Entry points

- **`cmd/api`** — builds config → logger → DB pool → Redis → repositories →
  services → router, then serves HTTP with graceful shutdown on `SIGTERM`.
- **`cmd/worker`** — same wiring minus the HTTP server; polls the `jobs` table for
  queued work.
- **`cmd/migrate`** — runs Bun migrations (`up`, `down`, `status`).

Both `api` and `worker` share the same `internal/` code; they differ only in what
they start. This keeps business logic in one place.

```go
func main() {
    cfg := config.Load()                 // Viper, once
    log := logging.New(cfg)              // Zap
    db := database.Connect(cfg, log)     // Bun + pgx pool
    rdb := cache.Connect(cfg)            // Redis
    app := server.New(cfg, log, db, rdb) // wires repos → services → handlers
    app.Run()                            // graceful shutdown built in
}
```

## 4. Configuration (Viper)

- Loaded **once** at startup into a typed `Config` struct, then passed down via DI.
- Sources, in precedence: environment variables → `.env` (dev) → defaults.
- **Never** call `viper.Get*` deep in the code — pass the typed value.
- Required-but-missing secrets fail fast at boot.

```go
type Config struct {
    Env         string `mapstructure:"ENV"`
    HTTPAddr    string `mapstructure:"HTTP_ADDR"`
    MetricsAddr string `mapstructure:"METRICS_ADDR"`
    DatabaseURL string `mapstructure:"DATABASE_URL"`
    RedisURL    string `mapstructure:"REDIS_URL"`
    AuthJWKSURL string `mapstructure:"AUTH_JWKS_URL"` // better-auth JWKS — validate JWTs (ADR 0006)
    AuthIssuer   string `mapstructure:"AUTH_ISSUER"`
    AuthAudience string `mapstructure:"AUTH_AUDIENCE"`
    Provisioner ProvisionerConfig
    LogLevel    string `mapstructure:"LOG_LEVEL"`
}
```

Full variable reference: [`INFRASTRUCTURE.md`](INFRASTRUCTURE.md).

## 5. HTTP layer (Gin)

- Routes are versioned under `/api/v1` and grouped by domain in `server/`.
- Middleware order: `request-id → logger → recovery → cors → ratelimit → auth`.
- Handlers **bind + validate** request DTOs (`binding` tags), call one service
  method, and map the result to the standard envelope. Nothing else.

```go
func (h *SiteHandler) Create(c *gin.Context) {
    var req dto.CreateSiteRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        respondError(c, apperr.Validation(err)); return
    }
    acct := middleware.AccountID(c)
    site, err := h.sites.Create(c.Request.Context(), acct, req)
    if err != nil { respondError(c, err); return }
    c.JSON(http.StatusAccepted, gin.H{"data": site})
}
```

API conventions (envelopes, status codes, pagination): [`API.md`](API.md).

## 6. Services

- Own **business rules** and **transactions**. The only layer that may span
  multiple repositories or call the provisioner/queue.
- Accept `context.Context` first and an explicit `accountID` — never read the
  account from a request object; the handler extracts it and passes it in.
- Multi-write operations run in a single `bun.Tx` so partial state never persists.

```go
func (s *SiteService) Create(ctx context.Context, acct uuid.UUID, req dto.CreateSiteRequest) (*model.Site, error) {
    return database.InTx(ctx, s.db, func(tx bun.Tx) (*model.Site, error) {
        site := model.NewSite(acct, req)
        if err := s.repo.Create(ctx, tx, site); err != nil { return nil, err }
        if err := s.queue.Enqueue(ctx, tx, queue.ProvisionSite{SiteID: site.ID}); err != nil {
            return nil, err // same tx: site row and job row commit or roll back together
        }
        return site, nil
    })
}
```

## 7. Repositories (Bun)

- The **only** place that touches the database.
- Every customer query is **scoped by `account_id`**. This is a security boundary.
- Return domain models; never return `*gin.Context` or leak Bun internals upward.
- Accept an optional `bun.IDB` so the same method works inside or outside a tx.

```go
func (r *SiteRepo) List(ctx context.Context, acct uuid.UUID, p Page) ([]model.Site, error) {
    var sites []model.Site
    err := r.db.NewSelect().Model(&sites).
        Where("account_id = ?", acct).          // never optional
        Order("created_at DESC").
        Limit(p.Limit).Offset(p.Offset).
        Scan(ctx)
    return sites, err
}
```

Schema, migrations, and indexing rules: [`DATABASE.md`](DATABASE.md).

## 8. Provisioner

- The **single gateway** to Docker, Caddy, Cloudflare, or fallback Hestia. The
  dashboard and API never receive Docker daemon or Caddy admin access.
- **Idempotent:** re-running a step on an existing resource succeeds, not errors —
  this makes job retries and reconciliation safe.
- Defined behind an interface so services depend on the capability, not a provider:

```go
type SiteProvisioner interface {
    CreateSite(ctx context.Context, spec SiteSpec) error
    DeleteSite(ctx context.Context, ref SiteRef) error
    SuspendSite(ctx context.Context, ref SiteRef) error
    ResumeSite(ctx context.Context, ref SiteRef) error
    SiteStatus(ctx context.Context, ref SiteRef) (SiteState, error)
}
```

- `PROVISIONER_BACKEND` selects `docker`, `hestia`, or `fake`; `fake` is rejected
  in production. A fake implements the interface for tests, so CI never reaches a
  live Docker daemon or hosting node. Hosting flows: [`HOSTING.md`](HOSTING.md).

## 9. Async jobs (Postgres queue)

- Anything that can exceed ~1s (provisioning, SSL, backups, email) is **enqueued**,
  not run inline. The handler returns `202` + a resource/job id to poll.
- The **`jobs` table is the queue** ([ADR 0002](adr/0002-postgres-backed-job-queue.md)).
  `Enqueue` is an `INSERT` inside the caller's transaction, so a resource row and
  its job commit or roll back **together** — no dual-write against a second store.
- The worker claims work with `FOR UPDATE SKIP LOCKED`, so multiple workers never
  double-process a job:

```sql
UPDATE jobs SET status = 'running', attempts = attempts + 1, updated_at = now()
WHERE id = (
    SELECT id FROM jobs
    WHERE status = 'queued' AND run_at <= now()
    ORDER BY created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING *;
```

- **Retries:** on failure, set `status='queued'` and push `run_at` forward
  (exponential backoff). After N attempts mark the job `failed` and enqueue
  compensating cleanup. No orphaned half-provisioned resources.
- **Crash recovery:** a periodic reaper re-queues jobs stuck in `running` past a
  timeout — safe because the provisioner is idempotent.

```go
type Job interface { Kind() string }
type ProvisionSite struct{ SiteID uuid.UUID }
func (ProvisionSite) Kind() string { return "provision_site" }
```

## 10. Error handling

- Errors flow **up** wrapped with context: `fmt.Errorf("create site %q: %w", domain, err)`.
- Known cases are typed in `internal/apperr` (`apperr.NotFound`, `apperr.Conflict`,
  `apperr.Validation`, `apperr.Forbidden`).
- A single `respondError` maps typed errors → HTTP status + envelope, so every
  endpoint behaves identically.
- **Never leak internals** (SQL, stack traces, provider output) to clients; log the
  detail, return a clean message + stable `code`. Full policy: [`API.md`](API.md#5-errors).

## 11. Logging (Zap)

- One structured JSON logger, configured at startup, injected via DI. No `fmt.Println`.
- Every line carries `request_id`; add `account_id`, `user_id`, `job_id` where relevant.
- Levels: `debug` (dev), `info` (lifecycle/business events), `warn` (degraded),
  `error` (needs a human).
- **Never log secrets.** Redact at the boundary. Metrics, not high-cardinality
  logs, carry trends — emit Prometheus metrics for rates/latency/counts.

## 12. Conventions

- `gofmt` + `golangci-lint` gate CI. See [`CODING_STANDARDS.md`](CODING_STANDARDS.md).
- `context.Context` is the first arg on anything doing I/O, with timeouts on every
  external call.
- Accept interfaces, return concrete types; define interfaces in the consumer.
- No global mutable state except the config and logger singletons.
- Tests: table-driven units with mocks; integration against a disposable Postgres.
  See [`TESTING.md`](TESTING.md).

## 13. Approved dependencies

Beyond the core stack (Gin, Bun, go-redis, Viper, Zap), these are pre-approved —
anything else needs the justification + confirmation required by `CLAUDE.md` §5.4:

| Library | Use for | Not for |
|---|---|---|
| `golang-jwt/jwt/v5` | Verify better-auth JWT signature/claims; the backend issues no tokens (ADR 0006) | Fetching JWKS (it only takes a `Keyfunc`) |
| `MicahParks/keyfunc/v3` | Supplies the `jwt.Keyfunc` from better-auth's JWKS — auto-refreshing HTTP client + cache (ADR 0006) | — |
| `google/uuid` | Entity + job IDs | — |
| `prometheus/client_golang` | `/metrics` endpoint, counters/histograms | — |
| `go-redis/redis_rate` | Rate limiting middleware | Queueing (ADR 0002) |
| `stretchr/testify` | `require`/`assert` in tests | Mock generation frameworks |

Deliberately **not** used: a validation lib (Gin bundles `validator/v10`), a
migration tool (Bun has `bun/migrate`), an HTTP client lib (stdlib `net/http`),
a decimal lib (money is `int64` cents — see `CODING_STANDARDS.md`).

## Phase 1 account layer

- `internal/model`: Account, AccountMembership, AuditLog
- `internal/repository`: AccountRepo, AuditRepo
- `internal/service`: AccountService (me, profile, admin users, bootstrap)
- `internal/handler`: AccountHandler
- `cmd/bootstrap-admin`: explicit admin promotion
- Middleware: Auth validates JWT claims, then `RequireCurrentMembership`
  re-checks database role/status; RateLimit (Redis); CORS
- Admin is a global platform role. Admin list/detail are the only Phase 1
  cross-account paths; both are audited and return only safe identity fields.
- Role/status/bootstrap writes use a transaction-scoped PostgreSQL advisory lock
  plus row locking, so count/check/update cannot race past the last-active-admin
  invariant. Profile and privileged mutation audits share the owning transaction.
- Membership provisioning uses a per-user advisory lock and
  `INSERT ... ON CONFLICT`, so concurrent callers converge without orphan
  accounts or aborted-transaction re-reads.

## Phase 2 site-provisioning slice

- `internal/model`: Node, Site, Job
- `internal/repository`: NodeRepo, SiteRepo, JobRepo
- `internal/service`: tenant-scoped SiteService and global audited NodeService
- `internal/queue`: atomic `SKIP LOCKED` claim, exponential retry, stale-job
  reaper, compensating cleanup, and periodic reconciliation
- `internal/provisioner`: concurrency-safe fake and a Docker/Caddy adapter using
  deterministic resource names plus complete ownership-label checks
- `cmd/worker`: the only process that executes provider calls; provider work is
  never performed inside the API transaction

Create reserves node capacity, inserts the site and job, and appends the audit
row in one transaction. External Docker/Caddy work happens after commit. The
worker then commits the resulting status, job completion, and audit event in one
database transaction. Exact-once `capacity_released_at` accounting prevents
retries from decrementing node usage twice. A delete intent wins over an older
in-flight provision/suspend/resume result.

The current slice supports only the curated static-site template. Database
provisioning, backup/restore, Hestia, DNS ownership automation, and a production
Docker authorization boundary are deliberately not claimed.
