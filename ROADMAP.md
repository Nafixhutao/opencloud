# Roadmap

Phased delivery plan for OpenCloud. Each phase is shippable on its own and builds
on the last. Status legend: ✅ done · 🚧 in progress · ⏳ planned.

> This roadmap is directional, not a contract. Re-prioritize as reality demands,
> but keep it current — it's how the team and Claude know what's built.

---

## Current status

The Next.js dashboard now has working registration/login and an authenticated
shell; Compose brings up dashboard, API, worker, PostgreSQL, Redis, and both
migration gates. Phase 0 is complete: Docker/Caddy is the validated MVP hosting
backend (ADR 0008), its repeatable VPS spike is recorded green, and Hestia is
preserved as a documented fallback. Phase 1 account and tenant work is next.

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

## Phase 1 — Auth & accounts ⏳

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
  — 🚧 backend JWT-validation middleware landed (`middleware.Auth` + JWKS via
  `keyfunc/v3`); BFF foundation (PostgreSQL `auth` schema, email/password,
  session handler/client) landed; `jwt()` plugin (JWKS at `/api/auth/jwks`) and
  login/register UI landed; JWT tenant claims (`account_id`/`role`) pending
- Social login (Google + GitHub) + email/password via better-auth
  (ADR 0006 — supersedes ADR 0005)
  — 🚧 provider integration and conditional UI landed; production OAuth
  credentials are still pending
- RBAC (`customer`, `admin`) enforced in middleware
  — 🚧 backend `middleware.RequireRole` landed (403 on mismatch); role-gated
  routes wired when the first admin/customer endpoint lands
- Account + user management (signup, login, profile, password reset)
- Admin panel shell with role-gated routes
- Audit logging for sensitive actions
- Rate limiting on auth endpoints

**Exit criteria:** secure auth end to end; admin can see and manage users.

## Phase 2 — Provisioning core ⏳

The heart of the platform: drive Docker/Caddy through a provider-neutral boundary.

- `provisioner` package: idempotent Docker/Caddy adapter + fake for tests; Hestia
  remains an optional adapter behind the same interface
- `nodes` registry + simple least-loaded placement
- Postgres-backed job queue (`jobs` table + `SKIP LOCKED`) + worker with
  retries/backoff and compensating cleanup
- Site lifecycle: create → active → suspend → delete (async, status-polled)
- Database lifecycle: scoped PostgreSQL/MariaDB DB + user provisioning
- Reconciliation job: detect/repair control-plane ↔ node drift
- Basic control-plane backups: scheduled `pg_dump` + one rehearsed restore

**Exit criteria:** a customer can create and delete a working website from the
dashboard, backed by a real isolated site container — and the control-plane DB is backed up
on a schedule with a tested restore.

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
