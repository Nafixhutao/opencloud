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
└── dashboard     # Next.js standalone (:3000); starts after auth-migrate succeeds
```

- `api`, `worker`, and `migrate` use `backend/Dockerfile`; the image carries all
  three binaries and each service selects one command.
- `dashboard` and `auth-migrate` use the root `Dockerfile` (multi-stage; the
  `runner` target serves the standalone Next.js build, the `auth-migrate` target
  runs `npm run auth:migrate`).
- PostgreSQL uses a named volume. Redis is disposable by design.
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
| `PROVISIONER_BACKEND` | worker | `docker` (default), `hestia`, or non-production `fake` |
| `DOCKER_SOCKET` | worker | local Docker Unix socket path; never exposed publicly |
| `CADDY_API_URL` | worker | private/loopback Caddy admin endpoint |
| `HESTIA_API_URL` | worker | optional fallback node API base |
| `HESTIA_ACCESS_KEY` / `HESTIA_SECRET_KEY` | worker | scoped fallback access pair |
| `HESTIA_API_KEY` | worker | deprecated legacy fallback credential only |
| `LOG_LEVEL` | all | `debug`/`info`/`warn`/`error` |
| `API_URL` | frontend | backend base URL (server-side) |
| `CORS_ORIGINS` | api | allowlist (no `*` in production) |
| `RATE_LIMIT_RPS` | api | per-client request budget |

Secrets in production come from a secret manager / orchestrator secrets, not a
checked-in file. Rotate on exposure.
The dashboard validates production mail configuration at startup and refuses
non-delivery adapters or incomplete SMTP credentials. Credentials must be
provisioned externally and verified in staging before email is considered live.

## 4. Environments

| Environment | Purpose | Notes |
|---|---|---|
| **development** | local | Docker Compose; Docker selected by default, fake available for tests; verbose logs |
| **staging** | pre-prod | disposable labeled Docker/Caddy resources; safe data |
| **production** | live | hardened; secrets from manager; backups + alerting on |

Configuration differs **only** by environment variables, not by code paths.

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
