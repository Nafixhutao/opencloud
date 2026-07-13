# Roadmap

Phased delivery plan for OpenCloud. Each phase is shippable on its own and builds
on the last. Status legend: ✅ done · 🚧 in progress · ⏳ planned.

> This roadmap is directional, not a contract. Re-prioritize as reality demands,
> but keep it current — it's how the team and Claude know what's built.

---

## Current status

A minimal frontend shell exists on the Next.js App Router at the repo root (Vite
fully removed); the marketing and dashboard surfaces are still planned. The Go
backend scaffold and local datastores are wired, while the domain schema and
provisioning remain greenfield. We are in **Phase 0 → Phase 1**.

---

## Phase 0 — Foundations 🚧

Stand up the skeleton everything else hangs off.

- ✅ Documentation set + conventions (`CLAUDE.md`, `docs/`)
- ✅ Minimal Next.js App Router shell; legacy Vite artifacts removed
- ✅ Go backend scaffold: `cmd/api`, `cmd/worker`, layered packages, config (Viper), logging (Zap)
- ✅ PostgreSQL + Redis wired via Docker Compose
- ✅ Bun migration tooling complete; initial domain schema `public.accounts` created
  (first migration). Identity tables (`auth.*`) are owned by better-auth's
  migrations, not Bun (ADR 0006)
- ✅ Health/readiness endpoints + Prometheus metrics endpoint
- ⏳ Hestia integration spike (local VM): validate API auth/key-scoping and
  idempotency assumptions (ADR 0001) before Phase 2 builds on them
- ✅ CI: frontend (oxlint · tsc · build · audit) and backend
  (gofmt · lint · vet · test · vulnerability scan · Docker build)

**Exit criteria:** `docker compose up` brings up dashboard + API + datastores; a
user can register and log in.

## Phase 1 — Auth & accounts ⏳

- **shadcn/ui initialized** (Tailwind v4 preset — blank `tailwind.config`,
  `cssVariables: true`; verified compatible with Next 16 + React 19) as the
  dashboard component baseline. The login/register screen is its first consumer;
  primitives are added per-need (`button input card form dialog` first), not all
  at once. Landing stays GSAP-only — no GSAP in `app/(dashboard)`/`app/(admin)`.
  The **marketing** surface adopts **Astryx** when reworked (ADR 0007); one
  component system per route group.
- **better-auth** owns sessions + JWT (httpOnly cookies, JWKS); the Go backend
  validates JWTs and issues none (ADR 0006)
- Social login (Google + GitHub) + email/password via better-auth
  (ADR 0006 — supersedes ADR 0005)
- RBAC (`customer`, `admin`) enforced in middleware
- Account + user management (signup, login, profile, password reset)
- Admin panel shell with role-gated routes
- Audit logging for sensitive actions
- Rate limiting on auth endpoints

**Exit criteria:** secure auth end to end; admin can see and manage users.

## Phase 2 — Provisioning core ⏳

The heart of the platform: drive Hestia.

- `provisioner` package: idempotent Hestia API/CLI client + fake for tests
- `nodes` registry + simple least-loaded placement
- Postgres-backed job queue (`jobs` table + `SKIP LOCKED`) + worker with
  retries/backoff and compensating cleanup
- Site lifecycle: create → active → suspend → delete (async, status-polled)
- Database lifecycle: MariaDB DB + user provisioning
- Reconciliation job: detect/repair control-plane ↔ node drift
- Basic control-plane backups: scheduled `pg_dump` + one rehearsed restore

**Exit criteria:** a customer can create and delete a working website from the
dashboard, backed by a real Hestia node — and the control-plane DB is backed up
on a schedule with a tested restore.

## Phase 3 — Domains, DNS & SSL ⏳

- Domain management + linking to sites (bring-your-own-domain — ADR 0004)
- Cloudflare DNS zone + record management through the provisioner (ADR 0003)
- Cloudflare Tunnel ingress (`cloudflared`) for dashboard, API, and customer sites
- Automatic Let's Encrypt issuance/renewal via Certbot
- DNS propagation + certificate status surfaced in the UI

**Exit criteria:** a customer can point a domain, get DNS + HTTPS, with renewals automated.

## Phase 4 — Email, FTP/SSH & cron ⏳

- Mailbox provisioning and management — gated on a clean-IP outbound mail path;
  never sent from residential IPs (ADR 0004)
- FTP/SSH account lifecycle (web file manager first; raw FTP/SFTP needs
  non-tunnel ingress — ADR 0003)
- Cron job management per account
- File usage and quota enforcement surfaced in the UI
- Usage metering pipeline: worker polls per-account stats (disk, bandwidth —
  `v-list-user-stats`) from nodes into Postgres, so Phase 5 billing has history

## Phase 5 — Billing & plans ⏳

- Plans + subscriptions + entitlements
- Usage metering (disk, bandwidth) tied to plan limits (pipeline from Phase 4)
- Payments: manual bank transfer + admin confirmation first, then payment
  gateway integration + invoices (ADR 0004)
- Suspension/dunning workflow on non-payment

## Phase 6 — Observability & ops ⏳

- Per-account resource dashboards in Grafana
- Alerting (node down, disk pressure, cert expiry, failed jobs)
- Node hardening automation (Fail2ban, UFW) as repeatable bootstrap
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
