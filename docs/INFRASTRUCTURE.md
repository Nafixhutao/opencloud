# Infrastructure — Docker, Monitoring & Environments

How the OpenCloud **control plane** is packaged, run, configured, and observed.
The hosting **data plane** (Docker sites + Caddy, with Hestia fallback) is covered in [`HOSTING.md`](HOSTING.md);
release process in [`DEPLOYMENT.md`](DEPLOYMENT.md).

**Stack:** Docker · Docker Compose · Prometheus · Grafana · (host) Fail2ban · UFW.

---

## 1. What runs where

| Plane | Components | Runtime |
|---|---|---|
| **Control plane** | Go API, worker, Next.js frontend, PostgreSQL, Redis, Prometheus, Grafana | Docker containers |
| **Data plane** | Docker site containers + Caddy ingress; optional Hestia fallback node | Current Linux host for MVP; scale-out nodes later |
| **Edge** | Cloudflare DNS + Tunnel ([ADR 0003](adr/0003-cloudflare-dns-and-ingress.md)) | Cloudflare network; `cloudflared` runs on our hardware |

The MVP co-locates labeled site containers with the control plane while Caddy
remains the only public listener. Scale-out nodes use the same provider contract;
Hestia, if adopted, runs only on a separate clean host.

## 2. Docker Compose topology

```
docker-compose.yml
├── postgres      # PostgreSQL 18 (volume: pgdata)
├── redis         # Redis 8 (disposable cache)
├── migrate       # one-shot Bun migrations; depends_on: postgres
├── auth-migrate  # one-shot Better Auth migrations (auth.* — ADR 0006); after migrate
├── api           # Go API (:8080) + internal metrics (:9090)
├── worker        # Go job worker; starts after migrate succeeds
├── dashboard     # Next.js standalone (:3000); starts after auth-migrate succeeds
└── control-plane-backup # opt-in profile; encrypted scheduled pg_dump
```

- `api`, `worker`, and `migrate` use the `runtime` target in
  `backend/Dockerfile`; it also carries the operator-only `bootstrap-admin`
  binary. The separate `backup` target adds PostgreSQL client tools plus only
  the `control-plane-backup` orchestrator.
- `dashboard` and `auth-migrate` use the root `Dockerfile` (multi-stage; the
  `runner` target serves the standalone Next.js build, the `auth-migrate` target
  runs `npm run auth:migrate`).
- PostgreSQL uses a named volume. Redis is disposable by design.
- `control-plane-backup` is disabled unless the `backup` profile is selected.
  Its PostgreSQL-client image runs as the unprivileged `postgres` user with a
  read-only root filesystem, all capabilities dropped, `no-new-privileges`, a
  bounded tmpfs for restore verification, and a dedicated mode-`0700` volume.
- API, metrics, and dashboard ports bind to host loopback for local development.
- Prometheus and Grafana services land in later roadmap phases.

### Dockerfile conventions
- **Multi-stage** builds; final image is minimal (distroless/alpine) and runs as a
  **non-root** user.
- No secrets baked into images — everything comes from env at runtime.
- Pin base image versions; rebuild on dependency/security updates.

## 3. Configuration & environment variables

All config is environment-driven, loaded by **Viper** ([`BACKEND.md`](BACKEND.md#4-configuration-viper)).
Copy `.env.example` → `.env`; **never commit `.env`**.

| Variable | Service | Purpose |
|---|---|---|
| `ENV` | all | `development` / `staging` / `production` |
| `HTTP_ADDR` | api | listen address, e.g. `:8080` |
| `METRICS_ADDR` | api | separate internal metrics listener, e.g. `:9090` |
| `DATABASE_URL` | api, worker, migrate, frontend | PostgreSQL DSN (BFF reuses it for better-auth's `auth.*` tables — ADR 0006) |
| `REDIS_URL` | api, worker | Redis connection |
| `AUTH_JWKS_URL` | api | better-auth JWKS endpoint the API validates JWTs against; issues none (ADR 0006) |
| `AUTH_ISSUER` | api | expected JWT issuer; required in production |
| `AUTH_AUDIENCE` | api | expected JWT audience; required in production |
| `BETTER_AUTH_SECRET` | frontend | better-auth encryption/hashing key (≥32 chars) — ADR 0006 |
| `BETTER_AUTH_URL` | frontend | better-auth base URL (BFF origin) |
| `MAIL_PROVIDER` | frontend | `smtp` in production; `log`/`memory` only outside production |
| `MAIL_FROM` | frontend | verified sender identity for auth mail |
| `SMTP_HOST` / `SMTP_PORT` | frontend | production SMTP endpoint |
| `SMTP_SECURE` | frontend | `true` for implicit TLS (normally 465), otherwise required STARTTLS |
| `SMTP_USER` / `SMTP_PASS` | frontend | external SMTP credentials; both required in production |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | frontend | Google social login via better-auth (ADR 0006) |
| `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET` | frontend | GitHub social login via better-auth (ADR 0006) |
| `PROVISIONER_BACKEND` | worker | `fake` by default; reviewed deployments may select `docker`; `hestia` is not implemented |
| `DOCKER_SOCKET` | worker | local Docker Unix socket path; never exposed publicly |
| `CADDY_API_URL` | worker | private/loopback Caddy admin endpoint |
| `CADDY_SERVER_ID` | worker | Caddy HTTP server ID whose owned site routes may be changed |
| `SITE_DEFAULT_IMAGE` | worker | exact curated site image allowlist entry |
| `HESTIA_API_URL` | worker | optional fallback node API base |
| `HESTIA_ACCESS_KEY` / `HESTIA_SECRET_KEY` | worker | scoped fallback access pair |
| `HESTIA_API_KEY` | worker | deprecated legacy fallback credential only |
| `CUSTOMER_DATABASES_ENABLED` | api, worker | explicit customer PostgreSQL/MariaDB lifecycle opt-in; default `false` |
| `CUSTOMER_DATABASE_CREDENTIAL_KEY` | api, worker | external base64-encoded 32-byte AES-GCM key for unrevealed credentials |
| `CUSTOMER_POSTGRES_ADMIN_URL` | worker | worker-only admin URL for the dedicated customer PostgreSQL target |
| `CUSTOMER_MARIADB_ADMIN_DSN` | worker | worker-only admin DSN for the dedicated customer MariaDB target |
| `CUSTOMER_POSTGRES_HOST` / `CUSTOMER_POSTGRES_PORT` | api, worker | customer-visible PostgreSQL TLS endpoint |
| `CUSTOMER_MARIADB_HOST` / `CUSTOMER_MARIADB_PORT` | api, worker | customer-visible MariaDB TLS endpoint |
| `CUSTOMER_DATABASE_TLS_REQUIRED` | api, worker | advertised endpoint TLS requirement; production refuses `false` |
| `CONTROL_PLANE_BACKUP_KEY` | backup profile | external base64-encoded 32-byte AES-256 key; empty/malformed fails closed |
| `CONTROL_PLANE_BACKUP_RETENTION_DAYS` | backup profile | generated archive retention (default 14 days) |
| `CONTROL_PLANE_BACKUP_INTERVAL_SECONDS` | backup profile | schedule interval (default 86400; minimum 300) |
| `LOG_LEVEL` | all | `debug`/`info`/`warn`/`error` |
| `API_URL` | frontend | backend base URL (server-side) |
| `CORS_ORIGINS` | api | allowlist (no `*` in production) |
| `RATE_LIMIT_RPS` | api | per-client request budget |

Secrets in production come from a secret manager / orchestrator secrets, not a
checked-in file. Rotate on exposure.
The dashboard validates production mail configuration at startup and refuses
non-delivery adapters or incomplete SMTP credentials. Credentials must be
provisioned externally and verified in staging before email is considered live.

Customer database lifecycle is opt-in so an existing deployment cannot
accidentally provision into the control-plane PostgreSQL. When enabled, the API
fails fast unless the credential key and public endpoints are complete; the
worker additionally fails fast unless both data-plane admin targets are
configured and reachable. Admin URLs/DSNs and the encryption key come from the
orchestrator secret store. The worker rejects a customer PostgreSQL target that
resolves to the same configured host/port as the control plane. Production
admin connections must require certificate-verified TLS without plaintext
fallback, while customer endpoints use TLS and private worker-to-admin
networking. Rotate the envelope key only after pending credential rows are
revealed or revoked.

## 4. Environments

| Environment | Purpose | Notes |
|---|---|---|
| **development** | local | Docker Compose; fake provisioner by default; verbose logs |
| **staging** | pre-prod | disposable labeled Docker/Caddy resources; safe data |
| **production** | live | hardened; secrets from manager; backups + alerting on |

Configuration differs **only** by environment variables, not by code paths.
The local named backup volume is not an off-host production strategy. Production
activation requires secret-manager key injection, an encrypted off-host
destination/replication path, failure alerting, and a restore rehearsal from the
off-host copy.

## 5. Monitoring (Prometheus + Grafana)

- The API process exposes Prometheus metrics on the separate internal listener
  configured by `METRICS_ADDR` (`:9090/metrics` locally), not on the public API.
- Prometheus scrapes the API, the worker, and node exporters on hosting nodes.
- Grafana dashboards visualize:
  - **Control plane:** request rate/latency/errors (RED), queue depth + job
    success/failure, DB/Redis connection pool usage, Go runtime (GC, goroutines).
  - **Data plane:** container health, CPU/RAM/disk/bandwidth, Caddy route/certificate
    state, and per-account resource usage for customer dashboards.
- **Alerting** (Phase 6, [`../ROADMAP.md`](../ROADMAP.md)): node down, disk
  pressure, certificate expiry, failed-job spikes, error-rate SLO breaches.

### What to instrument
- HTTP: request count, duration histogram, status classes — labeled by route.
- Jobs: enqueued/started/succeeded/failed counters, processing duration.
- External calls: DB, Redis, Docker/Caddy, Cloudflare, and fallback-provider latency + error counters.
- Keep label cardinality low (no per-user labels). Trends are metrics; detail is logs.

## 6. Logging & observability

- Structured JSON logs (Zap) to stdout, captured by Docker; shipping/retention is
  an infra concern. See [`BACKEND.md`](BACKEND.md#11-logging-zap).
- Correlate logs and metrics via `request_id`.

## 7. Health & readiness

- `GET /healthz` — process is alive.
- `GET /readyz` — dependencies (PostgreSQL, Redis) reachable; used by orchestrators
  and load balancers to gate traffic.
- The frontend has its own health route for the platform's checks.

## 8. Networking & host security

- Caddy terminates TLS and routes dashboard/API/customer traffic. Its admin API
  remains private and configuration changes are validated before reload.
- **UFW** on every host allows only required ports (HTTP/HTTPS, SSH from bastion,
  internal scrape ports on the private network).
- **Fail2ban** bans abusive IPs (SSH, auth endpoints). Hardening details:
  [`SECURITY.md`](SECURITY.md).
- Datastore and metrics ports are bound to the private network, never the public
  internet.

## 9. Resource & capacity

- Set CPU/memory limits per container in Compose/orchestrator.
- Size PostgreSQL and Redis connection pools deliberately ([`BACKEND.md`](BACKEND.md));
  don't open per-request connections.
- Track node capacity in the `nodes` table; scale out by registering new nodes.
- The public API and dashboard never receive a Docker socket. The base Compose
  worker also receives none; a real worker requires a separate hardened
  rootless/restricted-socket deployment because Docker daemon access is
  host-equivalent.
