# Changelog

All notable changes to OpenCloud are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Change groups: **Added**, **Changed**, **Deprecated**, **Removed**, **Fixed**,
**Security**. Keep entries human-readable and link PRs/issues where useful. Add to
**[Unreleased]** as you work; move it under a version + date on release.

---

## [Unreleased]

### Added
- **Encrypted Phase 2 control-plane backup/restore (stacked review branch, not
  deployed):** a non-root opt-in Compose scheduler streams PostgreSQL 18 custom
  dumps through chunked authenticated AES-256-GCM encryption, atomically
  publishes SHA-256 sidecars, applies allowlisted pair-aware retention, and
  requires exact destructive confirmation for restores. CI performs a real
  two-Postgres rehearsal, verifies the encrypted archive catalog, restores a
  sentinel, asserts plaintext is absent, and cleans every disposable resource.
- **Phase 2 site-provisioning core (review branch, not deployed):** additive
  `nodes`, `sites`, and durable `jobs` schema; atomically reserved least-loaded
  placement; tenant-scoped asynchronous site create/suspend/resume/delete APIs;
  audited admin node management; retry/backoff, stale-job recovery, cleanup, and
  reconciliation workers; an ownership-checked Docker/Caddy adapter with a
  concurrency-safe fake; a constrained static-site runtime; and a responsive
  dashboard using TanStack Query plus real Testing Library interaction tests.
- **Phase 2 integration coverage:** concurrent placement and idempotency,
  transactional audit failure rollback, job claim uniqueness, exact capacity
  release, delete-versus-in-flight-provision ordering, full fake-provider
  lifecycle, and an opt-in real disposable Docker/Caddy lifecycle test.
- **Phase 1 auth hardening:** required email verification, one-time verification
  claims, real configurable Nodemailer SMTP delivery with TLS, authenticated
  password-change and login-failure audit events, safe admin name/email
  projection, and concurrent Postgres integration coverage for membership and
  last-active-admin invariants.
- **Phase 1 — Auth & accounts:** tenant `account_memberships` + append-only
  `audit_logs` migrations; signup ensures customer membership; Better Auth JWT
  `definePayload` emits trusted `account_id`/`role`; Go `GET/PATCH /api/v1/me`
  and admin `/api/v1/admin/users` with RBAC, self-lockout, and last-admin rules;
  `cmd/bootstrap-admin` for explicit admin promotion; password forgot/reset/change
  via Better Auth with configurable `MAIL_PROVIDER` (`log`/`memory` for
  development/test, real `smtp` transport for production);
  dashboard `/account` and `/admin/users`; Redis + Better Auth rate limits; audit
  events for login, password reset, profile, and admin role/status changes.

### Changed
- The backend build and CI toolchain now require Go 1.26.5, closing standard
  library vulnerabilities reported against the prior 1.26.2 build.
- Phase 1 is technically complete and staging-verified; production activation is
  explicitly deferred. Phase 2 remains incomplete until customer database
  lifecycle is implemented, both review branches pass review/CI, and the
  production backup key/off-host destination are configured and verified.
- Bun `up` now assigns one rollback group per migration instead of grouping all
  pending files. Production stays forward-only; a development `down` can only
  target the newest migration. Production mail configuration fails fast unless
  `MAIL_PROVIDER=smtp` and required sender/auth credentials are complete.
- Compose API defaults `AUTH_JWKS_URL` to the dashboard service and waits for
  JWKS readiness; dashboard receives optional mail env vars.

### Added (prior unreleased)
- **Docker + Caddy Phase 0 provisioning spike** (ADR 0008): a disposable,
  idempotent `apply|verify|destroy` harness creates one labeled, constrained,
  non-root site container with a dedicated network/volume and routes it through
  Caddy over HTTPS without touching unrelated OpenCloud resources. The Go backend
  now exposes a provider-neutral `SiteProvisioner` contract, deterministic site
  resource names, and validated `docker|hestia|fake` backend selection (`fake` is
  rejected in production). `docs/HESTIA_FALLBACK.md` preserves the Hestia
  security spike, adoption triggers, and reversible migration plan.
- **Geist Light auth and dashboard experience:** responsive `/login` and
  `/register` flows use Better Auth email/password plus conditionally-enabled
  Google/GitHub providers, shared Zod validation, actionable callback errors,
  and a restrained developer-tool interface with traceable deployment states.
  `/dashboard` now ships a responsive authenticated shell, resource overview,
  guided deployment rail, honest empty states, loading skeletons, and error
  recovery. Unauthenticated traffic and sign-out return to `/login`; focused
  auth validation runs via `npm run test:auth`.

- **Dependency adoption blueprint** (`docs/DEPENDENCIES.md`): centralizes the
  adopted, phase-planned, conditional, and deliberately excluded frontend,
  backend, testing, and API-contract tooling. Planned libraries are installed
  only with their first real consumer and still require dependency approval.
- **Auth foundation + JWKS** (ROADMAP Phase 0 exit criteria, ADR 0006/0007):
  Better Auth `jwt()` plugin in `lib/auth.ts` — public keys at
  `/api/auth/jwks`, session JWTs via `GET /api/auth/token` (EdDSA, iss/aud =
  `BETTER_AUTH_URL`, matching `.env.example`). shadcn/ui initialized (Tailwind
  v4, `components.json`, `app/globals.css` tokens, `@/*` path alias) with
  per-need primitives (`button input label card field`). Dashboard route
  `app/(dashboard)/dashboard` with server-side session guard and sign-out.
  New deps (approved list): `react-hook-form`, `zod`, `@hookform/resolvers` +
  shadcn runtime (`@base-ui/react`, `class-variance-authority`, `clsx`,
  `tailwind-merge`, `tw-animate-css`), with the `shadcn` CLI as development
  tooling. Docs updated: `docs/FRONTEND.md` §1, `ROADMAP.md` Phase 1 status.
- **Dashboard in the Compose stack** (ROADMAP Phase 0 exit criteria): root
  `Dockerfile` (multi-stage — `runner` serves the Next.js `output: 'standalone'`
  build as non-root on :3000; `auth-migrate` runs `npm run auth:migrate`) plus
  two new Compose services: one-shot `auth-migrate` (Better Auth identity
  tables, after Bun's `migrate` creates the `auth` schema — ADR 0006) and
  `dashboard` (starts after `auth-migrate` succeeds, loopback-bound
  `127.0.0.1:3000`). Compose now fails fast when `BETTER_AUTH_SECRET` is unset.
  `backend/smoketest.sh` also builds/probes the dashboard and asserts the
  `auth.*` tables exist. Docs updated: `README.md`, `docs/INFRASTRUCTURE.md` §2,
  `docs/DEPLOYMENT.md` §1/§5.
- **Go backend scaffold** (`backend/`, first ROADMAP Phase 0 code item): layered
  module `github.com/nazxf/opencloud/backend` with three entrypoints — `cmd/api`
  (Gin HTTP server, graceful SIGTERM shutdown), `cmd/worker` (job-loop skeleton
  polling the future Postgres `jobs` queue — ADR 0002), and `cmd/migrate`
  (`up`/`down`/`status` via Bun's migrator; registry empty until the first schema
  migration, so `up` is a no-op). Internal packages: `config` (Viper → typed,
  fail-fast on missing `DATABASE_URL`/`REDIS_URL`), `logging` (Zap structured
  JSON), `database` (pgx pool + Bun/pgdialect + `InTx` helper), `cache` (go-redis
  v9), `metrics` (Prometheus registry + HTTP instrumentation), `middleware`
  (request-id → logger → recovery), `handler` (`/healthz`, `/readyz` with
  Postgres+Redis checks, fail-closed 503), `server` (DI wiring, `/metrics`,
  `/api/v1` group), and `app` (shared bootstrap). Health handler covered by tests.
  Dependencies are all from the approved list in `docs/BACKEND.md` §13 (no new
  libraries); versions verified via Context7.
- **`docker-compose.yml`** (repo root): `postgres:18`, `redis:8`, `api`, `worker`
  services with health-gated startup ordering. Frontend and Prometheus/Grafana
  services deferred to a later item.
- **`backend/Dockerfile`**: multi-stage static build → distroless nonroot image
  carrying all three binaries; `.dockerignore` and `backend/.golangci.yml` added.
- **Backend CI job** in `.github/workflows/ci.yml` (replacing the placeholder):
  gofmt · golangci-lint · `go vet` · `go test` · govulncheck · docker build.
- **First domain migration** `20260713120000_create_accounts` (ROADMAP Phase 0):
  creates `public.accounts` — the tenant boundary (`docs/DATABASE.md` §3) with the
  `active`/`suspended`/`closed` status CHECK. The `migrations` package now embeds
  its `*.sql` files and registers them via Bun's `Migrations.Discover`, so
  `migrate up` applies real schema. `auth.*` identity tables stay owned by
  better-auth's own migrations, not Bun (ADR 0006). Verified end to end against
  Postgres 18: up → idempotent re-up → down round-trips cleanly.
- `docs/BACKEND.md` §13 names the concrete **JWKS fetcher** the resource-server
  auth path needs: `MicahParks/keyfunc/v3` (auto-refreshing JWK Set client) supplies
  the `jwt.Keyfunc` that `golang-jwt/jwt/v5` uses to verify better-auth's JWTs — the
  `+ JWKS` in the earlier entry was left unnamed (ADR 0006). Library choice verified
  via Context7: `keyfunc/v3` is the golang-jwt-native JWK Set wrapper.
- **JWT auth middleware** (`backend/internal/middleware/auth.go`, ROADMAP Phase 1):
  `Auth` makes the Go API a resource server (ADR 0006) — it validates a bearer JWT
  against better-auth's JWKS (asymmetric algs only; HMAC/`none` rejected), enforces
  `exp` and, when configured, `iss`/`aud`, then puts the caller's `user_id` (sub),
  tenant `account_id` (UUID — the `docs/SECURITY.md` §4 scoping key) and `role` on
  the request context via `AccountID`/`UserID`/`Role` helpers. `NewJWKS` builds the
  background-refreshing `jwt.Keyfunc` from `AUTH_JWKS_URL`. New config
  `AUTH_ISSUER`/`AUTH_AUDIENCE` — empty skips that check in dev, but **required in
  production**: API startup fails fast when either is empty under `ENV=production`
  so iss/aud validation is never a silent no-op (Codex review, PR #11). Worker and
  migration startup do not require API-only auth config. Failures return `401` in
  the standard error envelope without revealing which check failed.
  Not yet mounted on `/api/v1` — wired when the first protected endpoint lands.
  Table-driven tests cover valid, expired, missing-expiry, wrong iss/aud, bad
  signature, unknown kid, HMAC, missing/invalid `account_id`, missing sub, and
  missing/malformed headers.
- **Better Auth BFF foundation** (ROADMAP Phase 0/1 boundary): Next.js now mounts
  `/api/auth/*`, enables email/password sessions, and connects Better Auth to an
  isolated PostgreSQL `auth` schema. Bun bootstraps the namespace; Better Auth's
  programmatic migration API owns and idempotently migrates its identity tables.
- **Migration deployment gate:** Compose now runs a one-shot `migrate` service
  before API/worker startup, and CI verifies up/idempotent-up/down/up against a
  disposable PostgreSQL 18 service.
- **WSL-safe checkout rules:** `.gitattributes` keeps Go, shell, SQL, and YAML
  files on LF across Windows/WSL checkouts.
- **RBAC middleware** `middleware.RequireRole(allowed...)` (ROADMAP Phase 1):
  gates a route to callers whose validated token carries an allowed role
  (`customer`/`admin` — RBAC enforced server-side, `docs/SECURITY.md` §4). Runs
  after `Auth`; a non-matching role gets `403 Forbidden` ("authenticated but not
  allowed", `docs/API.md` §3), distinct from the `404` used for tenant resources.
  On its own (no `Auth` ahead) it forbids, so it can never be a privilege
  escalation. Tests cover admin-allowed, customer-forbidden, multi-role match,
  and empty/unknown role, plus the no-`Auth` case.

### Fixed
- Last-active-admin count/update is serialized in one transaction with a
  transaction-scoped PostgreSQL advisory lock; self-demotion/self-disable remain
  forbidden. Membership creation uses advisory locking plus `ON CONFLICT` and
  never re-queries an aborted transaction. Admin listing no longer performs an
  account query per row, and stale bearer JWT role/status is re-checked against
  the database on protected Go routes.
- Profile, bootstrap, role/status, and platform-admin read paths no longer ignore
  audit failures. Domain mutations and audit rows share a transaction; the
  Better Auth password boundary reports `AUDIT_FAILED` rather than claiming an
  unaudited success when its cross-schema audit append fails.
- Cleared the backend's current `golangci-lint` findings by checking migration
  database-close errors, using the supported Prometheus collector package, and
  documenting exported provisioner states.
- Scoped production issuer/audience validation to the API binary. Worker startup
  no longer requires unused auth settings, and migrations require only PostgreSQL
  configuration (Codex review, PR #13).
- The smoke test now uses an isolated Compose project, cleans up through a trap,
  and fails when readiness, metrics, or worker startup checks fail.
- API/worker receive only their service-specific environment variables; migration
  config requires PostgreSQL but no longer requires Redis.
- Prometheus metrics moved from the public API router to a separate `:9090`
  listener; recovered panics now log a stack trace and return the documented
  `INTERNAL` error code. JWT failures use `UNAUTHENTICATED` consistently.
- Stale paths, commands, CI claims, anchors, and ADR links in project docs now
  match the implemented repository.
- Restored a minimal Next.js App Router shell so frontend lint, type-check, and
  production build can run while the full marketing surface is rebuilt.
- Reordered HTTP middleware so recovered panics are included in request logs and
  Prometheus 5xx metrics; added regression coverage for the behavior.
- Readiness responses no longer expose raw PostgreSQL or Redis errors, and the
  HTTP server now enforces read, write, idle, header-read, and header-size limits.
- Prometheus HTTP method labels are bounded to standard methods (unknown methods
  collapse to `OTHER`), and response status labels now match the documented
  `2xx`/`4xx`/`5xx` class format.
- Inbound request IDs are length- and character-validated before entering logs.
- Local Compose ports now bind to loopback instead of every host interface.

- `docs/TESTING.md` §2 auth-testing targets corrected for **ADR 0006**: the Go
  backend tests **JWT validation** (signature, `exp`/`iss`/`aud`, forged/expired
  rejection) + RBAC — not token issue/refresh/rotation or password hashing, which
  are better-auth's (BFF), not the backend's.
- Config layer brought in line with **ADR 0006** (better-auth), which the earlier
  update missed: `.env.example`, `docs/INFRASTRUCTURE.md` §3, and
  `docs/DEPLOYMENT.md` §9 dropped the superseded Go-owned auth vars
  (`JWT_SECRET`, `JWT_ACCESS_TTL/REFRESH_TTL`, `OAUTH_*`, the
  `/api/v1/auth/oauth/{provider}/callback` URL) and now describe the resource-server
  split — the Go API validates JWTs via `AUTH_JWKS_URL`; better-auth in the BFF owns
  `BETTER_AUTH_SECRET`/`BETTER_AUTH_URL` and the `GOOGLE_/GITHUB_CLIENT_*` social
  credentials. Env names verified against better-auth's official docs.
- Doc consistency with accepted ADRs: `docs/DATABASE.md` intro no longer lists
  Redis as a job queue (the queue is the Postgres `jobs` table — ADR 0002);
  `docs/SECURITY.md` names argon2id as the password hash (matching
  `docs/BACKEND.md` §13, was "bcrypt or argon2id"); `docs/HOSTING.md`
  provisioner interface drops the vestigial `node` parameter from
  `CreateDNSZone` (zones live in Cloudflare, not on a node — ADR 0003).

### Security
- Phase 2 customer site responses omit account/node placement, image, internal
  port, and runtime-limit fields. Repeated customer delete requests converge on
  the original terminal state without a second job or capacity decrement.
- Phase 1 migration files are pinned by committed SHA-256 checksums and explicit
  immutable-history tests. `audit_logs` UPDATE/DELETE is rejected by database
  triggers, while verification storage contains only SHA-256 token digests—never
  raw tokens, URLs, cookies, passwords, or secrets.
- Upgraded indirect `github.com/quic-go/quic-go` from 0.59.0 to 0.59.1 to fix
  GO-2026-5676; `govulncheck ./...` now reports no reachable vulnerabilities.
- Forced the vulnerable PostCSS copy nested under Next.js to 8.5.10; `npm audit`
  now reports zero known production dependency vulnerabilities.

### Changed
- Docker Engine + Caddy are the primary MVP data plane; Hestia is retained as a
  separate-node fallback behind the same provisioner boundary. Hosting,
  architecture, infrastructure, security, testing, deployment, environment, and
  roadmap documentation now describe the new decision and its safety limits.
- Version pins refreshed to current stable (greenfield — verified this cycle):
  **PostgreSQL 16 → 18** (`docs/DATABASE.md`, `docs/INFRASTRUCTURE.md`; 18.4 is the
  current series, 16 still supported but a new project should start on 18) and the
  **Go floor 1.22 → 1.25** (`docs/BACKEND.md`, `README.md`; 1.25/1.26 are the
  maintained releases). Redis 8, React 19, Next.js 16, and Tailwind v4 were already
  current and unchanged.
- Planned Redis version bumped **7 → 8** (`docs/DATABASE.md`,
  `docs/INFRASTRUCTURE.md`) — current supported series, tri-licensed incl.
  AGPLv3; the cache/session/rate-limit usage is unchanged.
- `ROADMAP.md` Phase 1 now records **when shadcn/ui is initialized** (Tailwind v4
  preset, verified compatible with Next 16 + React 19 via Context7) — with the
  first authenticated screen, primitives added per-need, not up front. Reaffirms
  GSAP stays landing-only (none in `app/(dashboard)`/`app/(admin)`).
  `docs/FRONTEND.md §5` adds the per-phase shadcn component rollout table
  (which primitives `add` in which phase).

### Added
- `docs/HOSTING.md` now defines the `SiteSpec`/`DBSpec` provisioner inputs that the
  `Provisioner` interface already referenced, closing a blueprint gap flagged in a
  docs audit. Flags `SiteSpec.HestiaUser` (account→Hestia-user mapping) as the one
  field still to be settled in the Phase 2 Hestia spike rather than adding a schema
  column prematurely.
- ADR 0006: **better-auth is the identity provider** (supersedes ADR 0005) — auth
  moves from the Go backend into the Next.js BFF. better-auth owns email/password
  + Google/GitHub social + sessions and four `auth.*` tables (`user`, `session`,
  `account`, `verification`, on its own migrations); its `jwt` plugin issues JWTs
  via a JWKS endpoint, and the **Go backend becomes a resource server** that only
  validates them. Drops the 0005 plan (`x/oauth2`, argon2id passwords,
  `user_identities`, Go token issuance). Docs updated: `ARCHITECTURE.md`,
  `CLAUDE.md`, `docs/SECURITY.md`, `docs/BACKEND.md`, `docs/API.md`,
  `docs/DATABASE.md`, `docs/FRONTEND.md`, `ROADMAP.md`.
- ADR 0007: **Astryx (React + StyleX) alongside shadcn/ui**, split by route group —
  Astryx owns `app/(marketing)`, shadcn/ui owns `app/(dashboard)`/`app/(admin)`;
  they coexist via CSS layers but never share a screen. Relaxes the "single
  component library / no CSS-in-JS" rule to a per-route-group boundary. Docs
  updated: `CLAUDE.md` (§2, §6), `docs/FRONTEND.md`, `docs/UI_GUIDELINES.md`,
  `ROADMAP.md`.
- ADR 0005: **social login (Google & GitHub)** alongside password auth
  (**superseded by ADR 0006**) — OAuth
  authorization-code flow handled by the Go backend, which issues the same
  JWT/refresh pair; identities in a new `user_identities` table,
  `users.password_hash` now nullable. `golang.org/x/oauth2` added to approved
  deps. Docs updated: `docs/DATABASE.md`, `docs/API.md`, `docs/SECURITY.md`,
  `docs/BACKEND.md`, `docs/INFRASTRUCTURE.md`, `ROADMAP.md` (Phase 1).
  A **Frontend & UX** section records the login-screen design: social buttons are
  a top-level redirect (not a `fetch` — sanctioned `FRONTEND.md §3` exception),
  Google/GitHub brand SVGs are the sole exception to the Lucide-only icon rule,
  the `/login?error=<code>` → plain-language copy map, and the anti-lockout guard
  on Settings → Connected accounts.
- ADR 0003: **Cloudflare is the authoritative DNS and ingress path** — customer
  zones managed via the Cloudflare API by the provisioner; inbound traffic via
  Cloudflare Tunnel (`cloudflared`), so self-hosted nodes work behind CGNAT
  without a static IP. BIND9 demoted to documented fallback. Docs updated:
  `ARCHITECTURE.md` (adds the self-hosted-first design goal), `CLAUDE.md`,
  `README.md`, `docs/HOSTING.md`, `docs/INFRASTRUCTURE.md`, `ROADMAP.md`.
- ADR 0004: launch scope for services that can't be self-hosted — email
  deferred until a clean-IP mail path exists, domains are bring-your-own,
  payments start as manual bank transfer (gateway lands in Phase 5).
- Approved supporting libraries recorded in the stack: Go — `golang-jwt/jwt/v5`,
  `x/crypto` (argon2id), `google/uuid`, `prometheus/client_golang`,
  `go-redis/redis_rate`, `testify` (`docs/BACKEND.md` §13); frontend (dashboard
  phase) — TanStack Query/Table, react-hook-form + zod, Recharts, Vitest +
  Testing Library (`docs/FRONTEND.md`, `docs/TESTING.md`).
- Roadmap: Hestia integration spike added to Phase 0; basic `pg_dump` backups
  moved from Phase 6 into Phase 2's exit criteria; usage metering pipeline made
  an explicit Phase 4 item so Phase 5 billing has historical data.
- `.env.example` with documented, non-secret defaults (referenced by README and
  `docs/INFRASTRUCTURE.md` but previously missing).
- CI workflow (`.github/workflows/ci.yml`): frontend lint, type-check, build,
  and dependency audit on every PR and push to `main`. Go jobs are added when
  the backend scaffold lands.
- Complete project documentation set: `README.md`, `CLAUDE.md`, `ARCHITECTURE.md`,
  `ROADMAP.md`, `CHANGELOG.md`, and `docs/` (backend, frontend, database, API,
  hosting, infrastructure, security, deployment, coding standards, testing, UI
  guidelines, contributing).
- Architecture Decision Records: `docs/adr/` with template and ADR 0001
  (Hestia as a provisioning backend).
- ADR 0002: the `jobs` table in PostgreSQL **is** the job queue (claimed via
  `FOR UPDATE SKIP LOCKED`); Redis no longer plays a queue role. Fixes the
  dual-write flaw where a Redis enqueue was not transactional with the
  PostgreSQL write that triggered it. Docs updated: `ARCHITECTURE.md`,
  `CLAUDE.md`, `docs/BACKEND.md`, `docs/DATABASE.md` (adds `jobs.run_at` +
  claim index), `docs/HOSTING.md`, `ROADMAP.md`.

### Removed
- Deleted the leftover starter frontend so the dashboard/marketing UI can be
  rebuilt from a clean, planned base (Astryx + shadcn per ADR 0006/0007): removed
  `src/` (a stray "AI gateway" template — `App.tsx`, its components, `index.css`,
  `assets/`), the `app/page.tsx`/`app/layout.tsx` entry that imported it, and the
  OpenCloud logo + template icon assets under `public/` (`logo*.png/svg`,
  `favicon.svg`, `icons.svg`). Kept the partner brand SVGs in `public/brands/` and
  the frontend toolchain (`package.json`, `next.config.ts`, `tsconfig.json`,
  `postcss`, oxlint). The frontend does not build until it is rebuilt — intended.

### Changed
- GSAP (`@gsap/react`) and Geist fonts (`@fontsource`) — already in use by the
  landing page — are now part of the documented frontend stack (GSAP scoped to
  the marketing surface only).
- `package.json` name corrected from `frontend` to `opencloud`.
- `CLAUDE.md` restructured from a single large document into a concise contract +
  index that links to the detailed `docs/` topic files (single source of truth
  per topic).
- Frontend moved from `frontend/` to the **repo root** (`app/`, `src/`, `public/`,
  `package.json`, configs). Docs updated to match the new paths.

### Removed
- Legacy Vite frontend artifacts (`vite.config.ts`, `index.html`, `src/main.tsx`,
  `tsconfig.{app,node}.json`) superseded by the Next.js App Router (`app/`).
- Build/scratch artifacts (`.next/`, `dist/`, `output/`, `.playwright-cli/`,
  `verify-*.mjs`, `verify.png`) — now git-ignored, not committed.
- Remaining Vite scaffold leftovers: `src/assets/vite.svg`, `src/assets/react.svg`
  (unreferenced) and the empty `frontend/` directory.

---

## [0.0.0] — project inception

### Added
- Initial frontend scaffold and landing page.

---

[Unreleased]: https://example.com/opencloud/compare/v0.0.0...HEAD
[0.0.0]: https://example.com/opencloud/releases/tag/v0.0.0
