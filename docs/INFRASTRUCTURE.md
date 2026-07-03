# Infrastructure — Docker, Monitoring & Environments

How the OpenCloud **control plane** is packaged, run, configured, and observed.
The hosting **data plane** (Hestia nodes) is covered in [`HOSTING.md`](HOSTING.md);
release process in [`DEPLOYMENT.md`](DEPLOYMENT.md).

**Stack:** Docker · Docker Compose · Prometheus · Grafana · (host) Fail2ban · UFW.

---

## 1. What runs where

| Plane | Components | Runtime |
|---|---|---|
| **Control plane** | Go API, worker, Next.js frontend, PostgreSQL, Redis, Prometheus, Grafana | Docker containers |
| **Data plane** | Hestia + Nginx/Apache/PHP-FPM/MariaDB/Certbot (BIND9 installed, fallback only) | Host OS on each hosting node (not containerized) |
| **Edge** | Cloudflare DNS + Tunnel ([ADR 0003](adr/0003-cloudflare-dns-and-ingress.md)) | Cloudflare network; `cloudflared` runs on our hardware |

Only the control plane is containerized. Hosting nodes run Hestia directly on the
host for performance and OS-level tenant isolation.

## 2. Docker Compose topology

```
docker-compose.yml
├── frontend     # Next.js (port 3000)
├── api          # Go API (port 8080)        depends_on: postgres, redis
├── worker       # Go job worker             depends_on: postgres, redis
├── postgres     # PostgreSQL 16 (volume: pgdata)
├── redis        # Redis 8 (volume: redisdata)
├── prometheus   # scrapes api /metrics + node exporters (port 9090)
└── grafana      # dashboards (port 3001)    depends_on: prometheus
```

- `api` and `worker` share the same image (different command); built from
  `backend/Dockerfile` (multi-stage: build → minimal runtime image).
- `frontend` is built from the repo-root `Dockerfile` (Next.js standalone output).
- Datastores use named volumes so data survives `docker compose down`.
- Internal services talk over the Compose network; only the frontend, API, and
  Grafana are exposed to the host.

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
| `DATABASE_URL` | api, worker | PostgreSQL DSN |
| `REDIS_URL` | api, worker | Redis connection |
| `JWT_SECRET` | api | access-token signing key |
| `JWT_ACCESS_TTL` / `JWT_REFRESH_TTL` | api | token lifetimes |
| `OAUTH_GOOGLE_CLIENT_ID` / `OAUTH_GOOGLE_CLIENT_SECRET` | api | Google login (ADR 0005) |
| `OAUTH_GITHUB_CLIENT_ID` / `OAUTH_GITHUB_CLIENT_SECRET` | api | GitHub login (ADR 0005) |
| `OAUTH_REDIRECT_BASE_URL` | api | public base URL for OAuth callbacks |
| `HESTIA_API_URL` | api, worker | hosting node API base |
| `HESTIA_API_KEY` | api, worker | node API credential |
| `LOG_LEVEL` | all | `debug`/`info`/`warn`/`error` |
| `API_URL` | frontend | backend base URL (server-side) |
| `CORS_ORIGINS` | api | allowlist (no `*` in production) |
| `RATE_LIMIT_RPS` | api | per-client request budget |

Secrets in production come from a secret manager / orchestrator secrets, not a
checked-in file. Rotate on exposure.

## 4. Environments

| Environment | Purpose | Notes |
|---|---|---|
| **development** | local | Docker Compose; fake provisioner; verbose logs |
| **staging** | pre-prod | mirrors prod config; real (test) Hestia node; safe data |
| **production** | live | hardened; secrets from manager; backups + alerting on |

Configuration differs **only** by environment variables, not by code paths.

## 5. Monitoring (Prometheus + Grafana)

- The API exposes **Prometheus metrics** at `/metrics` (internal network only).
- Prometheus scrapes the API, the worker, and node exporters on hosting nodes.
- Grafana dashboards visualize:
  - **Control plane:** request rate/latency/errors (RED), queue depth + job
    success/failure, DB/Redis connection pool usage, Go runtime (GC, goroutines).
  - **Data plane:** per-node CPU/RAM/disk/bandwidth, and per-account resource usage
    for customer dashboards.
- **Alerting** (Phase 6, [`../ROADMAP.md`](../ROADMAP.md)): node down, disk
  pressure, certificate expiry, failed-job spikes, error-rate SLO breaches.

### What to instrument
- HTTP: request count, duration histogram, status classes — labeled by route.
- Jobs: enqueued/started/succeeded/failed counters, processing duration.
- External calls: DB, Redis, Hestia latency + error counters.
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

- A reverse proxy (Nginx in `deploy/nginx/`) terminates TLS for the control plane
  and routes to frontend/API.
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
