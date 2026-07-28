# Testing Strategy

How OpenCloud is tested. The goal is confidence per token of effort: cover the
risky paths thoroughly, keep the suite fast, and never test a real hosting node.

---

## 1. The pyramid

```
        ╱ e2e ╲          few — critical user journeys (Playwright)
      ╱─────────╲
    ╱ integration ╲      some — repos/migrations vs real Postgres; API vs fake node
  ╱─────────────────╲
 ╱       unit         ╲  many — services, provisioner logic, components (fast, mocked)
╱───────────────────────╲
```

Most tests are fast units. Integration tests cover the seams. A thin e2e layer
proves the whole thing holds together.

## 2. What to test thoroughly

Prioritize by risk. These get real coverage:

- **Auth** — JWT validation against the JWKS (signature, `exp`/`iss`/`aud`,
  rejection of forged/expired tokens), RBAC. Token issuance, refresh/rotation, and
  password hashing live in better-auth (BFF), not the Go backend — ADR 0006.
- **Tenant scoping** — every repository rejects cross-account access.
- **Money** — billing/usage math, integer-cent handling.
- **Provisioning** — service orchestration, job retries, compensating cleanup.
- **Validation** — boundary input handling.

Glue code and trivial getters don't need dedicated tests. **Coverage is a signal,
not a target** — don't chase 100% on plumbing.

## 3. Go — unit tests

- Test **services** and **provisioner logic** with mocked dependencies, so business
  rules run without a DB or a live node.
- **Table-driven** tests; mock at interface boundaries (repos, provisioner, queue).
- Pure functions get direct assertions.

```go
func TestSiteService_Create_RollsBackOnEnqueueFailure(t *testing.T) {
    repo := &fakeSiteRepo{}
    q := &failingQueue{}                 // Enqueue returns an error
    svc := NewSiteService(db, repo, q, fakeProvisioner{})

    _, err := svc.Create(ctx, acct, dto.CreateSiteRequest{Domain: "x.com"})

    require.Error(t, err)
    require.Empty(t, repo.saved, "tx must roll back; nothing persisted")
}
```

## 4. Go — integration tests

- Run **repositories and migrations** against a **disposable PostgreSQL** (Docker /
  Testcontainers), behind a build tag so unit runs stay fast:
  ```bash
  go test ./...                      # unit
  go test -tags=integration ./...    # + integration
  ```
- Each test gets a clean schema (migrate up) and isolated data; tests don't share
  mutable state.
- Verify the things mocks can't: real SQL, constraints, indexes, `account_id`
  scoping actually filtering at the DB.

## 5. Provisioner and hosting backends

- Services are tested against a **fake provider-neutral provisioner** implementing
  the same interface as Docker/Caddy and fallback Hestia
  ([`BACKEND.md`](BACKEND.md#8-provisioner)).
- CI never receives a Docker socket, Caddy admin endpoint, or real hosting-node
  credentials. A disposable, explicitly labeled staging target may be used for a
  controlled spike/e2e run only.
- Test **idempotency** explicitly: calling create twice succeeds; retry after a
  simulated mid-failure converges to the right state.
- Test ownership guards: an object with missing/mismatched OpenCloud labels is
  never adopted, changed, or deleted.

## 6. Frontend tests

- Tooling: **Vitest + React Testing Library** (added when the dashboard starts —
  the marketing surface doesn't warrant it).
- **Component tests** for logic-bearing components (forms, state, conditional
  rendering) — render + assert behavior, not implementation details.
- Mock the API client; assert components handle loading/empty/error/success states.
- Keep pure presentational components light on tests.

## 7. End-to-end (e2e)

- **Playwright** covers the critical journeys against a running stack (staging or
  ephemeral): **signup → login → create site → see it active → delete**.
- e2e is the smallest layer — it's slow and broad. Keep it to journeys that would
  hurt most if broken.
- A subset runs as a post-deploy smoke test ([`DEPLOYMENT.md`](DEPLOYMENT.md#10-post-deploy-verification)).

## 8. Test data & fixtures

- Build domain objects with small **factory helpers**, not giant shared fixtures.
- No reliance on test execution order; each test sets up and tears down its own data.
- No real secrets or customer data in tests.

## 9. CI

- Unit tests, lint, and type-check run on every PR. CI runs the Bun migration
  up/idempotent-up/down/up round trip and applies Better Auth migrations twice
  against disposable PostgreSQL services.
  Repository integration suites are added when repositories land.
- Pipeline details: [`DEPLOYMENT.md`](DEPLOYMENT.md#2-ci-pipeline).

## 10. Conventions

- **Every bug fix ships with a regression test** that fails before the fix.
- Non-trivial logic (a branch, loop, parser, money/auth path) leaves at least one
  runnable check behind — per [`../CLAUDE.md`](../CLAUDE.md) AI rules.
- Tests are first-class code: readable, named for what they assert, no frameworks or
  fixtures beyond what the test needs.
- Prefer assertion helpers (`testify/require`) for clear failures; fail fast on
  setup errors.

## Phase 1 tests

Frontend: `npm run test:auth`, `npm run auth:check-providers`, lint, tsc, build,
audit. Real Postgres tests cover concurrent membership convergence/no orphans,
unverified-login rejection, verification expiry/single-use, reset delivery via
the memory adapter, reset single-use, old/new password behavior, and an
authenticated password-change audit failure path.
Backend: gofmt, golangci-lint, vet, `go test ./...` (integration needs DATABASE_URL), govulncheck, Docker build.
Migrations: immutable Phase 1 checksums plus up/idempotent-up/latest-down/up
schema comparison in CI; sentinel membership/audit rows must survive latest
down. Tenant isolation, stale-token demote/suspend behavior, transactional audit
failure rollback, N+1-free admin projection, concurrent membership creation,
and concurrent last-active-admin rules run against disposable PostgreSQL.

## Phase 2 site-provisioning tests

Frontend:

```bash
npm run test:ui
npm run lint
npx tsc --noEmit
npm run build
```

The UI suite renders the real site dashboard and asserts validation blocks an
invalid create request, while an accepted mutation invalidates the query and
shows the queued site. These are behavior tests, not TypeScript value checks.

Backend unit tests cover fake-provider concurrency and Docker/Caddy
idempotency/ownership/security configuration. With a disposable PostgreSQL in
`DATABASE_URL`, service integration tests cover concurrent least-loaded
placement, idempotency convergence, unique job claims, audit-trigger rollback,
exact capacity release, the full fake lifecycle, and delete winning over an
in-flight provision result.

The real Docker/Caddy lifecycle is opt-in and must target disposable resources:

```bash
DOCKER_INTEGRATION=1 \
DOCKER_SITE_IMAGE=opencloud/site-static:phase2-validation \
CADDY_INTEGRATION_URL=http://127.0.0.1:22019 \
CADDY_INTEGRATION_PUBLIC_URL=http://127.0.0.1:22443 \
go test -tags=integration ./internal/provisioner \
  -run TestDockerCaddyLifecycleAgainstDisposableBackend -count=1
```

The test creates twice, suspends twice, resumes twice, deletes twice, verifies
HTTP routing, and checks final absence. It must never point at the active Caddy
admin API.

## Phase 2 customer database tests

Frontend behavior tests validate the create form, selected engine payload,
transitional status, explicit one-time reveal confirmation, returned credential
display, and typed delete confirmation.

Real control-plane PostgreSQL integration covers concurrent idempotent create,
tenant isolation, job payload secrecy, delete winning over in-flight
provisioning, concurrent one-time reveal, and audit-trigger failures that roll
back create, completion, and credential consumption. The fake data-plane
provider proves the full durable job lifecycle without external credentials.

The real adapter integration test requires disposable targets:

```bash
bash deploy/validation/run-postgres-phase2.sh
bash deploy/validation/run-customer-databases-phase2.sh
```

The first script proves migration `up`/idempotent `up`/latest `down`/`up`,
preserves earlier Phase 1/2 data, and runs service integration. The second
creates isolated real targets; for both engines it creates a scoped
database/user, proves the credential can write only its database, retries create
with password rotation while preserving data, rejects the old password, deletes
twice, and confirms both physical database and login are absent. CI pins
disposable PostgreSQL 18 and MariaDB 11.8.8 services. Both scripts use unique
resource names, refuse to reuse existing objects, and remove only their own
containers, network, and cache volumes. No test may point at the control-plane
database or an active customer target.

## Phase 2 control-plane backup tests

`internal/backup` tests authenticated multi-chunk and empty-stream round trips,
tamper/truncation/wrong-key/trailing-data rejection, strict key parsing,
checksum tamper detection, symlink/traversal rejection, exclusive scheduler
locking, and pair-aware retention that leaves unknown files untouched.

The CI/VPS rehearsal is:

```bash
bash deploy/validation/run-control-plane-backup-phase2.sh
```

It creates two uniquely named disposable PostgreSQL 18 containers and a private
network/volume, applies all public migrations to the source, inserts a sentinel,
starts the real scheduler, verifies the encrypted custom archive, proves the
plaintext sentinel is absent from it, restores into the second database, and
checks the sentinel/schema. A trap deletes the exact containers, network,
volume, and image on success or failure. The script never targets an active
OpenCloud database.
