# Architecture

System design for OpenCloud — the custom cloud shared-hosting platform. This is
the high-level map; component-level depth lives in the [`docs/`](docs/) topic
files. Decisions are recorded as ADRs in [`docs/adr/`](docs/adr/).

---

## 1. Design goals

1. **Custom UX** — own the dashboard; hide hosting complexity from customers.
2. **Tenant isolation** — a customer can never reach another's data, files, or processes.
3. **Automation-first** — provisioning/suspension/teardown are API-driven, no manual steps.
4. **Observability** — every account and node is measurable.
5. **Secure by default** — least privilege, hardened nodes, no plaintext secrets.
6. **Horizontal scale** — add hosting nodes without rearchitecting the control plane.
7. **Self-hosted first** — prefer open components on our own hardware; third-party
   services only where physics or licensing force it, each recorded in an ADR
   ([0004](docs/adr/0004-external-services-at-launch.md), [0009](docs/adr/0009-direct-caddy-customer-domains.md)).

**Non-goals:** building our own web server, DNS server, or mail stack. We
orchestrate Docker Engine and Caddy behind a provider-neutral provisioner;
customer-managed DNS is the universal domain path, while Cloudflare and Hestia
remain optional future/fallback adapters (ADRs 0008 and 0009).

## 2. Control plane vs. data plane

OpenCloud separates **what it owns** from **what it orchestrates**:

- **Control plane** (we build): the Go API, PostgreSQL, Redis, the worker, the
  Next.js dashboard, and the monitoring stack. System of record for accounts,
  billing, plans, and orchestration state. Containerized with Docker.
- **Data plane** (we orchestrate): isolated Docker site containers, networks, and
  volumes behind Caddy. It serves customer traffic and holds customer site data;
  scale-out nodes or a fallback Hestia adapter use the same provisioner contract.

The control plane drives the data plane exclusively through the **provisioner**
(Docker/Caddy for the MVP). Nothing else touches a hosting backend. See
[ADR 0008](docs/adr/0008-docker-caddy-provisioning-backend.md). The implemented
domain path gives customers provider-neutral manual DNS instructions and lets
the worker manage exact-host Caddy routes. Cloudflare DNS/Tunnel is optional and
unavailable until a future per-tenant authorization contract is implemented
([ADR 0009](docs/adr/0009-direct-caddy-customer-domains.md)).

## 3. System diagram

```
                       ┌───────────────────────────────┐
                       │      Customers / Admins        │
                       └───────────────┬───────────────┘
                                       │ HTTPS
                       ┌───────────────▼───────────────┐
                       │ Caddy :443 · public ACME       │   ← direct ingress (ADR 0009)
                       └───────────────┬───────────────┘
                                       │ exact-host route
                       ┌───────────────▼───────────────┐
                       │   Next.js Dashboard (SSR/BFF)  │   ← control plane
                       └───────────────┬───────────────┘
                                       │ REST /api/v1 (JSON, JWT via httpOnly cookie)
                       ┌───────────────▼───────────────┐
                       │     Go / Gin Control Plane     │
                       │  handler · service · repo      │
                       │  middleware · queue · provisioner
                       └───┬───────────┬───────────┬────┘
                           │           │           │
                ┌──────────▼─┐  ┌──────▼─────┐  ┌──▼──────────────┐
                │ PostgreSQL │  │   Redis    │  │ Docker + Caddy  │  ← data plane
                │ (Bun ORM)  │  │   cache    │  │ sites + routes  │
                │ system of  │  │ ·sessions  │  └──┬──────────────┘
                │  record +  │  │ ·ratelimit │     │
                │ job queue  │  └────────────┘     │ provisions / manages
                └─────┬──────┘                     │
                      │ polls jobs         site containers · networks ·
                ┌─────▼──────────┐         volumes · domains · TLS
                │  Worker (jobs) │
                └────────────────┘

      Monitoring: Prometheus scrapes API + nodes → Grafana dashboards
      Host hardening: Fail2ban + UFW on every node
      DNS: customer proves TXT ownership, then adds A at any provider; API observes DNS
      TLS: Caddy asks an internal indexed permission endpoint before issuance
```

The diagram shows the production target, not current deployment state. The
Phase 3 implementation has only been validated with disposable containers and a
local CA. Production requires public ports 80/443, a stable public IPv4, public
DNS/ACME reachability, an external verification key, and certificate monitoring.
The Caddy admin and permission listeners stay on loopback/private transport.

## 4. Backend layering

A strict, one-directional dependency flow. Each layer may only call the one
below it (plus the provisioner from services).

```
┌─────────────────────────────────────────────────────────────┐
│ handler (Gin)   HTTP ↔ domain. Bind + validate DTOs.         │
│                 No business logic. No DB access.             │
├─────────────────────────────────────────────────────────────┤
│ service         Business rules. Owns transactions. The only  │
│                 layer that spans repositories + provisioner. │
├──────────────────────────────┬──────────────────────────────┤
│ repository (Bun)             │ provisioner (provider adapter)│
│ All DB access. Always        │ Only caller of hosting nodes. │
│ account_id-scoped.           │ Idempotent + reconcilable.    │
├──────────────────────────────┴──────────────────────────────┤
│ PostgreSQL · Redis           │ Docker/Caddy data plane       │
└─────────────────────────────────────────────────────────────┘
```

Cross-cutting concerns live in **middleware** (auth, request-id, logging,
recovery, rate-limit) and **config** (Viper, loaded once at startup). Details:
[`docs/BACKEND.md`](docs/BACKEND.md).

## 5. Request lifecycle (synchronous read)

```
GET /api/v1/sites
 → middleware: request-id → log → recover → rate-limit → authenticate (validate better-auth JWT — ADR 0006) → authorize
 → handler: parse query params
 → service: SiteService.List(ctx, accountID, filters)
 → repository: SELECT … WHERE account_id = $1   (scoped, indexed, paginated)
 → handler: wrap in { "data": [...], "meta": {...} } → 200
```

## 6. Provisioning lifecycle (asynchronous write)

Provisioning touches a real node and can take seconds, so it never runs inside the
request:

```
POST /api/v1/sites
 → handler validates, service creates a `sites` row (status=provisioning) AND a
   `jobs` row ("provision_site") in the same tx, returns 202 + site id
   — one transaction, so a site row can never exist without its job (or vice versa)
 → worker picks up the job:
     provisioner.CreateSite(ctx, spec)         // idempotent provider calls
     on success → mark site active
     on failure → retry with backoff; after N retries mark failed + enqueue cleanup
 → client polls GET /api/v1/sites/{id} until status is active|failed
```

This gives fast responses, retryable failures, and no half-created accounts (the
service rolls back or marks `failed` — never leaves orphaned state). See
[`docs/HOSTING.md`](docs/HOSTING.md) for the provisioning flows.

## 7. Data model overview

PostgreSQL is the **system of record** (and the job queue); Redis is a disposable cache.

- `accounts` — the tenant boundary. Every customer-owned row carries `account_id`.
- `auth.user` — identity (email, `role`), owned by **better-auth** in the `auth`
  schema, not a Bun-managed table ([ADR 0006](docs/adr/0006-better-auth-identity-provider.md)).
- `plans` / `subscriptions` — what a customer is entitled to.
- `nodes` — hosting capacity registered by backend driver.
- `sites`, `domains`, `databases`, `mailboxes`, `dns_zones`, `certificates` —
  customer resources, each linked to an `account_id` and a `node_id`.
- `jobs` — **the job queue itself**: async work + status, claimed by the worker
  with `FOR UPDATE SKIP LOCKED` ([ADR 0002](docs/adr/0002-postgres-backed-job-queue.md)).
- `audit_logs` — append-only record of sensitive actions.

Full schema, conventions, and migration rules: [`docs/DATABASE.md`](docs/DATABASE.md).

## 8. Multi-tenancy & isolation

Isolation is the platform's #1 invariant, enforced at three layers:

1. **Application** — every repository query is scoped by `account_id`; the JWT
   carries the caller's account, and services pass it down. A missing scope is a
   security defect, not a style issue.
2. **Database** — foreign keys and `account_id` columns make cross-tenant joins
   impossible by accident; admin cross-account access is an explicit, audited path.
3. **Data plane** — each site gets dedicated Docker network/volume ownership,
   non-root runtime policy, and CPU/memory/PID limits. Hestia can provide stronger
   per-customer Linux-user isolation if fallback triggers are met.

## 9. Scaling

- **API & worker** are stateless → scale horizontally behind a load balancer.
  Sessions live in Redis and the job queue in PostgreSQL, not in process memory;
  `SKIP LOCKED` lets multiple workers claim jobs without double-processing.
- **PostgreSQL** scales vertically first, then with read replicas for reporting.
- **Hosting nodes** scale out: register a new `nodes` row, and the scheduler places
  new accounts on the least-loaded node. Existing accounts are unaffected.
- **Redis** can move to a managed deployment as cache/session volume grows. If job
  volume ever outgrows the Postgres queue, introducing a dedicated queue is a new
  ADR superseding [ADR 0002](docs/adr/0002-postgres-backed-job-queue.md).

## 10. Failure handling

- The provisioner is **idempotent** so jobs can be retried safely; the control
  plane reconciles drift between its state and a node's actual state.
- A central recovery middleware turns panics into `500`s without killing the process.
- Compensating actions clean up partial provisioning instead of leaving orphans.
- Datastores have timeouts/deadlines on every call so one slow node can't exhaust pools.

## 11. Technology rationale (brief)

| Choice | Why |
|---|---|
| **Go + Gin** | Fast, concurrent, simple deploys; great for an orchestration API. |
| **Bun** | Lightweight, explicit SQL-first ORM — no heavy magic. |
| **PostgreSQL** | Strong constraints + transactions for the system of record — and the job queue (`SKIP LOCKED`), so enqueueing is transactional with the write that triggers it. |
| **Redis** | Cache, sessions, and rate limiting. |
| **Viper/Zap** | Standard, structured config + logging. |
| **Next.js** | SSR dashboard + BFF; hosts **better-auth** (identity provider — ADR 0006), holds the JWT in an httpOnly cookie. |
| **shadcn/ui + Tailwind** | Own the components; no heavyweight UI dependency. |
| **Docker + Caddy** | Reuse the deployed host for isolated HTTP workloads, provider-neutral lifecycle, ingress, and automatic HTTPS (ADR 0008). |
| **Customer DNS + direct Caddy** | Provider-neutral TXT/A records, exact-host routes, and allowlisted On-Demand TLS; Cloudflare remains an optional future adapter (ADR 0009). |
| **Prometheus/Grafana** | De-facto standard metrics + dashboards. |

Each significant choice should also have an ADR in [`docs/adr/`](docs/adr/).
