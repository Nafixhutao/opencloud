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

## 5. Provisioner & Hestia

- The provisioner is tested against a **fake Hestia client** implementing the
  `Provisioner` interface ([`BACKEND.md`](BACKEND.md#8-provisioner)).
- **Never run provisioning tests against a production node.** A dedicated test node
  may be used in staging e2e only.
- Test **idempotency** explicitly: calling create twice succeeds; retry after a
  simulated mid-failure converges to the right state.

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
