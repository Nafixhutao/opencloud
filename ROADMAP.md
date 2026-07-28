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
missing Phase 1 code. Phase 2 is **in progress**: the site-provisioning core is
merged into `main`, while PR #26 is the active review branch for encrypted
control-plane backup/restore and the opt-in PostgreSQL/MariaDB customer database
lifecycle. Nothing in Phase 2 is deployed. Review and green CI on the hardened
PR head, dedicated verified-TLS database targets, external encryption-key
custody, and off-host production backup storage remain release gates.

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
  Hestia requirements and migration triggers remain in `docs/HESTIA_FALLBACK.md`
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

## Phase 2 — Provisioning core 🚧

The heart of the platform: drive Docker/Caddy through a provider-neutral boundary.

- 🚧 `provisioner` package: idempotent, ownership-checked Docker/Caddy adapter +
  concurrency-safe fake for tests. The current review slice still requires CI
  approval; Hestia remains an unimplemented optional adapter.
- 🚧 `nodes` registry + transactionally reserved least-loaded placement
- 🚧 Postgres-backed job queue (`jobs` table + `SKIP LOCKED`) + worker with
  retries/backoff, stale-job recovery, and compensating cleanup
- 🚧 Site lifecycle: create → active → suspend/resume → delete, exposed through
  tenant-scoped APIs and a status-polled dashboard
- 🚧 Database lifecycle: additive tenant-scoped schema, idempotent asynchronous
  PostgreSQL/MariaDB database + least-privilege user provisioning, encrypted
  one-time credential reveal, per-database serialized provider operations,
  delete/cleanup compensation, and paginated dashboard flows. The PR #26 review
  branch defaults the feature off and still requires review, green CI on the
  hardened head, and disposable real-engine validation before merge.
- 🚧 Reconciliation job: detect/repair managed site state without adopting or
  deleting unrelated Docker/Caddy resources
- 🚧 Basic control-plane backups: an opt-in non-root Compose scheduler now streams
  `pg_dump` into authenticated AES-256-GCM chunk encryption, publishes atomic
  checksummed artifacts, prunes only generated archive pairs, and has a real
  disposable restore rehearsal. It remains a review-branch feature; production
  still needs external key custody and an off-host encrypted destination.

**Exit criteria:** a customer can create and delete a working website and a
scoped PostgreSQL/MariaDB database from the dashboard, backed by isolated real
providers, and the control-plane DB is backed up on a schedule with a tested
restore. This remains in progress until the stacked branches are reviewed,
green in CI, and merged. Production activation has separate secret, TLS,
off-host storage, staging verification, and approval gates.

## Phase 3 — Domains, DNS & SSL ⏳

- Domain management + linking to sites (bring-your-own-domain — ADR 0004)
- Cloudflare DNS zone + record management through the provisioner (ADR 0003)
- Cloudflare Tunnel ingress (`cloudflared`) for dashboard, API, and customer sites
- Automatic certificate issuance/renewal through Caddy with an allowlisted
  On-Demand TLS permission endpoint
- DNS propagation + certificate status surfaced in the UI

**Exit criteria:** a customer can point a domain, get DNS + HTTPS, with renewals automated.

## Phase 4 — Email, FTP/SSH & cron ⏳

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
