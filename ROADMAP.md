# Roadmap

Phased delivery plan for OpenCloud. Each phase is shippable on its own and builds
on the last. Status legend: ✅ done · 🚧 in progress · ⏳ planned.

> This roadmap is directional, not a contract. Re-prioritize as reality demands,
> but keep it current — it's how the team and Claude know what's built.

---

## Current status

Phase 1 (Auth & accounts) is technically implemented, hardened, and verified in
staging. Production activation is deliberately deferred; external mail/OAuth
credentials and a production release approval remain operational gates, not
missing Phase 1 code. Phase 2 code is **complete and merged** into `main` through
PR #26: site provisioning, encrypted control-plane backup/restore, and the
opt-in PostgreSQL/MariaDB customer database lifecycle are present. Nothing in
Phase 2 is production-deployed. Dedicated verified-TLS database targets,
external encryption-key custody, off-host production backup storage, staging
verification, and release approval remain operational gates.
Phase 3 code is complete and has passed disposable PostgreSQL, browser/build,
and real Caddy On-Demand TLS validation. It is not production-deployed. A public
ingress IPv4, inbound ports 80/443,
public DNS and ACME reachability, external verification-key custody, monitoring,
and explicit release approval remain blockers to production activation.

---

## Phase 0 — Foundations ✅

Stand up the skeleton everything else hangs off.

- ✅ Documentation set + conventions (`CLAUDE.md`, `docs/`)
- ✅ Minimal Next.js App Router shell; legacy Vite artifacts removed
- ✅ Go backend scaffold: `cmd/api`, `cmd/worker`, layered packages, config (Viper), logging (Zap)
- ✅ PostgreSQL + Redis wired via Docker Compose
- ✅ Bun migration tooling complete; initial domain schema `public.accounts` created
  (first migration). Identity tables (`auth.*`) are owned by better-auth's
  migrations, not Bun (ADR 0006)
- ✅ Health/readiness endpoints + Prometheus metrics endpoint
- ✅ Docker/Caddy integration spike validated constrained container ownership,
  HTTPS routing, idempotent create/retry/delete, recovery, and safe cleanup on
  the target VPS (ADR 0008; `docs/spikes/2026-07-21-docker-caddy-phase0.md`).
- ✅ CI: frontend (oxlint · tsc · build · audit) and backend
  (gofmt · lint · vet · test · vulnerability scan · Docker build)

**Exit criteria:** `docker compose up` brings up dashboard + API + datastores; a
user can register and log in; the selected MVP hosting backend has a documented,
repeatable idempotency spike.

## Phase 1 — Auth & accounts ✅

- **shadcn/ui initialized** (Tailwind v4 preset — blank `tailwind.config`,
  `cssVariables: true`; verified compatible with Next 16 + React 19) as the
  dashboard component baseline. The login/register screen is its first consumer;
  primitives are added per-need (`button input card form dialog` first), not all
  at once. Landing stays GSAP-only — no GSAP in `app/(dashboard)`/`app/(admin)`.
  The **marketing** surface adopts **Astryx** when reworked (ADR 0007); one
  component system per route group.
  — ✅ initialized with the login/register screens (`button input label card
  field`; the registry ships `field` instead of the old `form` wrapper)
- **better-auth** owns sessions + JWT (httpOnly cookies, JWKS); the Go backend
  validates JWTs and issues none (ADR 0006)
  — ✅ JWT plugin with server-side `definePayload` claims `account_id` + `role`
  from `public.account_memberships`; Go `middleware.Auth` validates signature,
  iss/aud/exp, UUID `account_id`, and role ∈ {customer,admin}
- Social login (Google + GitHub) + email/password via better-auth
  (ADR 0006 — supersedes ADR 0005)
  — ✅ provider integration and conditional UI landed; production OAuth
  credentials are still an external operator dependency (not claimed active)
- RBAC (`customer`, `admin`) enforced in middleware
  — ✅ `RequireRole` on `/api/v1/admin/*`; admin UI gated server-side; signup
  always creates `customer` membership; admin only via `bootstrap-admin`
- Account + user management (signup, login, profile, password reset)
  — ✅ `GET/PATCH /api/v1/me`, `/account` profile + audited password change,
  required email verification, and single-use verification/reset links. SMTP is
  a real TLS transport; production refuses non-delivery adapters or incomplete
  credentials.
- Admin panel shell with role-gated routes
  — ✅ `/admin/users` list/detail/role/status with safe name/email identity,
  explicit audited cross-account access, self-lockout protection, and a
  transaction/advisory-lock last-active-admin guard.
- Audit logging for sensitive actions
  — ✅ DB-enforced append-only `public.audit_logs`; profile, bootstrap, role/status,
  password, login, verification, reset, and platform-admin reads are audited.
- Rate limiting on auth endpoints
  — ✅ better-auth rateLimit on sign-in/up/reset/change-password; Redis limits on API
- Migration and token hardening
  — ✅ shipped Phase 1 migration checksums are immutable; each new migration gets
  its own rollback group; concurrent membership creation converges without
  orphans; protected Go routes re-check current DB role/status so stale bearer
  JWTs cannot retain admin access.

**Exit criteria:** met in code, disposable integration, and operator staging
verification. Production release remains a separate, deferred approval gate; no
credential or production deployment is claimed by this roadmap.

## Phase 2 — Provisioning core ✅

The heart of the platform: drive Docker/Caddy through a provider-neutral boundary.

- ✅ `provisioner` package: idempotent, ownership-checked Docker/Caddy adapter +
  concurrency-safe fake for tests.
- ✅ `nodes` registry + transactionally reserved least-loaded placement
- ✅ Postgres-backed job queue (`jobs` table + `SKIP LOCKED`) + worker with
  retries/backoff, stale-job recovery, and compensating cleanup
- ✅ Site lifecycle: create → active → suspend/resume → delete, exposed through
  tenant-scoped APIs and a status-polled dashboard. The workspace overview uses
  one tenant-scoped aggregate query, so counts do not truncate at collection
  page boundaries.
- ✅ Database lifecycle: additive tenant-scoped schema, idempotent asynchronous
  PostgreSQL/MariaDB database + least-privilege user provisioning, encrypted
  one-time credential reveal, per-database serialized provider operations,
  delete/cleanup compensation, canonical bounded pagination metadata, and
  paginated dashboard flows. The merged implementation defaults the feature off;
  production still requires dedicated verified-TLS targets and key custody.
- ✅ Reconciliation job: detect/repair managed site state without adopting or
  deleting unrelated Docker/Caddy resources
- ✅ Basic control-plane backups: an opt-in non-root Compose scheduler now streams
  `pg_dump` into authenticated AES-256-GCM chunk encryption, publishes atomic
  checksummed artifacts, prunes only generated archive pairs, and has a real
  disposable restore rehearsal. Production still needs external key custody and
  an off-host encrypted destination.

**Exit criteria:** a customer can create and delete a working website and a
scoped PostgreSQL/MariaDB database from the dashboard, backed by isolated real
providers, and the control-plane DB is backed up on a schedule with a tested
restore. These code and merge criteria are met. Production activation remains
separate and blocked on secret custody, TLS targets, off-host storage, staging
verification, and release approval.

## Phase 3 — Domains, DNS & SSL 🚧

- ✅ Tenant-safe domain attachment, global hostname claims, ownership challenge
  rotation/expiry, verification, detach, audit, and durable lifecycle jobs
- ✅ Universal staged manual DNS instructions: TXT ownership proof first, then
  an A record to the configured direct-Caddy ingress address only after proof
  is consumed (ADR 0009)
- ✅ Exact-host Caddy routes and a metrics-listener-only On-Demand TLS permission
  endpoint; unknown/inactive hostnames and database failures deny issuance
- ✅ DNS, lifecycle, certificate, renewal, error, retry, copy, and typed-detach
  states surfaced through the Next.js BFF and accessible site domain dashboard
- ✅ Disposable real-PostgreSQL migration/race/rollback proof, full Go and
  frontend gates, official Caddy validation, local-CA handshakes, routing checks,
  and database-outage fail-closed proof
- ⏳ Merge/release approval and production prerequisites: public IPv4, ports
  80/443, public DNS and ACME reachability, secret custody, renewal/error
  monitoring, backups, and rollback rehearsal
- ⏳ Cloudflare Tunnel/DNS automation is an optional future adapter. Its feature
  flag refuses to start until real per-tenant authorization exists.

**Exit criteria:** met in code and disposable validation; not yet met for
production because no public DNS, certificate, or deployment has been activated.

## Phase 4 — Universal application platform 🚧

- ✅ **4A Projects, services, and deployment records:** additive, tenant-scoped
  control-plane project/service/deployment/event model; Projects dashboard and
  authenticated API. Existing sites remain unchanged; source acquisition,
  builds, registry, and runtime deployment remain future slices.
- ✅ **Build abstraction:** static and generic Railpack planning detect only a
  validated source manifest. Build execution remains disabled until a dedicated
  isolated builder can enforce resource, network, and credential boundaries.
- ⏳ **4B–4F:** source acquisition, generic build provider, isolated builder,
  private registry, immutable revision rollout, health checks, and rollback.
  The isolated-builder service foundation now enforces a fail-closed BuildKit
  contract, lifecycle, limits, cancellation, and cleanup; it has no source or
  registry transport yet, so it is not a runnable deployment path. The registry
  and runtime-deployment foundations now enforce digest-only artifact identity,
  lifecycle persistence, health-before-Caddy-switch sequencing, retirement,
  and immutable rollback behind injected worker-only providers. Source
  acquisition, a real private transport, and hardened runtime deployment are
  still required before any deployment is enabled.
- ✅ **4G customer logs foundation:** tenant-scoped Loki query contract,
  collector configuration, historical Go API, SSE live tail, authenticated BFF,
  and interactive project Logs Viewer. Runtime/build/source adapters remain
  disabled, so production log emission is not claimed yet.
- ✅ **4H ENV/SECRETS:** tenant-scoped, service-scoped, environment-scoped
  (production/preview/development) environment variables and secrets. Secrets
  encrypted at rest (AES-256-GCM), never logged, redacted at boundaries,
  explicit rotation, and access audit. No NEXT_PUBLIC exposure unless
  explicitly configured. Frontend UI for managing env/secrets per
  project/service/environment (an earlier unused UI was removed; the manager
  was rebuilt wired through authenticated BFF routes).
- 🚧 **4I–4M foundations (not production-deployed):** per-service persistent
  storage quotas; short-lived database console sessions with an audited SQL
  Console and a phpMyAdmin redirect for MariaDB (no public
  `db.<platform-domain>` gateway yet); Git source fields plus a GitHub webhook
  route as the start of source acquisition/monorepo support; S3-compatible
  object storage with buckets/objects, quotas, presigned URLs, async storage
  jobs, and a bucket/object browser UI; and preview-deployment records wired
  into the build job handlers. None of these claim a live production backend.

## Later platform work — Email, FTP/SSH & cron ⏳

- Mailbox provisioning and management — gated on a clean-IP outbound mail path;
  never sent from residential IPs (ADR 0004)
- FTP/SSH account lifecycle (web file manager first; raw FTP/SFTP needs
  non-tunnel ingress — ADR 0003)
- Cron job management per account
- File usage and quota enforcement surfaced in the UI
- Usage metering pipeline: worker records per-account container/volume stats
  (disk, CPU, memory, bandwidth) into Postgres, so Phase 5 billing has history

## Phase 5 — Billing & plans ⏳

- Plans + subscriptions + entitlements
- Usage metering (disk, bandwidth) tied to plan limits (pipeline from Phase 4)
- Payments: manual bank transfer + admin confirmation first, then payment
  gateway integration + invoices (ADR 0004)
- Suspension/dunning workflow on non-payment

## Phase 6 — Observability & ops ⏳

- Per-account resource dashboards in Grafana
- Alerting (node down, disk pressure, cert expiry, failed jobs)
- Host/worker hardening automation (Docker authorization boundary, Caddy admin
  isolation, Fail2ban, UFW) as repeatable bootstrap
- Full backup strategy + restore runbooks (basic `pg_dump` lands in Phase 2)
- Incident runbooks in `docs/runbooks/`

## Phase 7 — Scale & polish ⏳

- Multi-node scheduling improvements + node draining
- Read replicas for reporting
- Performance pass (caching, query tuning, bundle size)
- Accessibility audit (WCAG AA) across the dashboard
- Localization / i18n if required

---

## Longer-term ideas (unscheduled)

- Reseller accounts (sub-tenants under a customer)
- Registrar (reseller API) integration to sell domains directly — launch is
  bring-your-own-domain (ADR 0004)
- One-click app installers (WordPress, etc.)
- Staging environments / git-based deploys for customer sites
- Public API + API keys for power users
- Marketplace for add-ons

---

## How to update this file

When a milestone lands, flip its marker and add a line to
[`CHANGELOG.md`](CHANGELOG.md). When priorities shift, move items between phases
rather than deleting them — the history of intent is useful.
