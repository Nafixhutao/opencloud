# OpenCloud

> A modern, custom-built cloud hosting platform with a bespoke dashboard and Go
> control plane, powered by Docker workloads and Caddy ingress.

OpenCloud lets customers provision and manage websites, domains, databases, DNS,
and SSL through a fast custom dashboard while operators manage capacity, plans,
and customers from a dedicated admin panel. The Go backend is the system of
record and drives provider-neutral provisioning; Docker + Caddy is the MVP
backend and Hestia is preserved as a fallback (ADR 0008).

---

## Table of Contents

- [Features](#features)
- [Tech Stack](#tech-stack)
- [Repository Layout](#repository-layout)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Common Tasks](#common-tasks)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [License](#license)

---

## Features

- **Website management** — create, suspend, and delete sites across hosting nodes.
- **Domains & DNS** — connect your own domain; zones and records managed via
  Cloudflare ([ADR 0003](docs/adr/0003-cloudflare-dns-and-ingress.md)).
- **Databases** — provision and manage scoped PostgreSQL/MariaDB databases and users.
- **SSL** — automatic certificate issuance and renewal through Caddy.
- **Deployments** — curated static, Node.js, PHP, Python, Go, and CMS containers;
  email and traditional FTP remain post-launch ([ADR 0004](docs/adr/0004-external-services-at-launch.md)).
- **Resource monitoring** — per-account CPU, RAM, disk, and bandwidth in Grafana.
- **Multi-tenant isolation** — strict per-customer separation enforced end to end.
- **Automation-first** — provisioning, suspension, and teardown are fully API-driven.

## Tech Stack

| Layer | Technologies |
|---|---|
| **Backend** | Go · Gin · Bun ORM · PostgreSQL · Redis · Viper · Zap |
| **Frontend** | Next.js · React · TypeScript · Tailwind CSS · shadcn/ui · Lucide React · GSAP |
| **Hosting** | Docker Engine · Caddy · PostgreSQL/MariaDB · Cloudflare DNS; Hestia fallback |
| **Platform** | Docker · Docker Compose |
| **Monitoring** | Prometheus · Grafana |
| **Security** | Fail2ban · UFW |

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for how these fit together.

## Repository Layout

```
opencloud/
├── README.md             # you are here
├── CLAUDE.md             # AI/dev contract + docs index
├── ARCHITECTURE.md       # system design
├── ROADMAP.md            # phased plan
├── CHANGELOG.md          # release history
├── app/                  # Next.js App Router (dashboard + landing)
├── public/               # static assets
├── next.config.ts        # Next.js config
├── package.json          # frontend dependencies + scripts
├── backend/              # Go control plane
├── docs/                 # detailed topic docs (see below)
├── Dockerfile            # dashboard + auth-migrate image targets
└── docker-compose.yml    # dashboard + backend + migration gates + datastores
```

> **Status:** Phase 1 auth/accounts is implemented, security-hardened, and
> verified in staging; its production release is deliberately deferred. The
> Phase 2 site-provisioning core is merged into `main` but is not deployed.
> PR #26 is the active review branch for encrypted scheduled control-plane
> backup/restore plus the opt-in PostgreSQL/MariaDB customer database lifecycle,
> one-time credential delivery, and a paginated database dashboard. Review,
> green CI on the hardened head, and explicit release approval remain required.
> See [`ROADMAP.md`](ROADMAP.md).

## Quick Start

### Prerequisites

- **Docker** 24+ and **Docker Compose** v2
- **Go** 1.26+ (for backend dev outside Docker)
- **Node.js** 22+ and **npm** (for frontend dev; matches the production image)
- Linux + Caddy for the real Docker provisioning spike (optional for UI/backend
  development, where the provisioner can use a fake)

### Run the stack (Docker)

```bash
git clone <repo-url> opencloud
cd opencloud
cp .env.example .env          # then edit secrets
docker compose up --build
```

The Compose stack runs the Bun and Better Auth migrations, then starts the API,
worker, dashboard, PostgreSQL, and Redis. Monitoring services land in later phases.

| Service | URL |
|---|---|
| Dashboard | http://localhost:3000 |
| Backend API | http://localhost:8080/api/v1 |
| Internal metrics | http://localhost:9090/metrics |

### Run services individually (local dev)

Backend:
```bash
cd backend
cp ../.env.example .env
go run ./cmd/api          # HTTP server
go run ./cmd/worker       # background job worker (separate terminal)
```

Frontend (repo root):
```bash
npm install
npm run dev               # http://localhost:3000
```

## Configuration

All configuration is environment-driven and loaded by **Viper**. Copy
`.env.example` to `.env` and fill in values. **Never commit `.env`.** Key groups:

| Variable | Purpose |
|---|---|
| `DATABASE_URL` | PostgreSQL connection string |
| `REDIS_URL` | Redis connection string |
| `METRICS_ADDR` | Separate internal Prometheus listener (default `:9090`) |
| `AUTH_JWKS_URL` | better-auth JWKS the API validates JWTs against (issues none — ADR 0006) |
| `AUTH_ISSUER` / `AUTH_AUDIENCE` | Optional JWT issuer and audience validation |
| `BETTER_AUTH_SECRET` / `BETTER_AUTH_URL` | better-auth (BFF) identity provider config |
| `MAIL_PROVIDER` / `MAIL_FROM` | `smtp` + sender identity in production; `log`/`memory` are non-delivery dev/test adapters |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_SECURE` / `SMTP_USER` / `SMTP_PASS` | Real verification/reset delivery; production fails fast when incomplete |
| `PROVISIONER_BACKEND` | `docker` (default), `hestia` fallback, or `fake` outside production |
| `DOCKER_SOCKET` / `CADDY_API_URL` | Docker/Caddy worker connection details |
| `HESTIA_API_URL` / `HESTIA_ACCESS_KEY` / `HESTIA_SECRET_KEY` | Optional fallback node access |
| `CONTROL_PLANE_BACKUP_KEY` | External base64 AES-256 key for the opt-in encrypted backup profile |
| `CONTROL_PLANE_BACKUP_RETENTION_DAYS` / `CONTROL_PLANE_BACKUP_INTERVAL_SECONDS` | Backup retention and schedule |
| `CUSTOMER_DATABASES_ENABLED` | Opt-in customer database lifecycle; disabled by default |
| `CUSTOMER_DATABASE_CREDENTIAL_KEY` | External base64 AES-256 key used only for pending one-time database credentials |
| `CUSTOMER_POSTGRES_ADMIN_URL` / `CUSTOMER_MARIADB_ADMIN_DSN` | Worker-only admin connections to dedicated customer database targets; PostgreSQL production uses `sslmode=verify-full` |
| `CUSTOMER_POSTGRES_HOST` / `CUSTOMER_MARIADB_HOST` | Public customer endpoints returned after credential reveal |
| `LOG_LEVEL` | `debug` / `info` / `warn` / `error` |

See [`docs/INFRASTRUCTURE.md`](docs/INFRASTRUCTURE.md) for the full reference.

## Common Tasks

```bash
# Backend (from repo root)
(cd backend && go test ./...)                       # run tests
(cd backend && golangci-lint run)                   # lint
(cd backend && go run ./cmd/migrate up)             # apply DB migrations

# Frontend
npm run auth:migrate                # migrate Better Auth tables (after Bun)
npm run test:auth                   # auth/mail/concurrency integration tests
npm run lint                        # oxlint
npm run build                       # production build

# Stack
docker compose logs -f api          # tail the API service
docker compose --profile backup up -d control-plane-backup
docker compose down                 # stop everything
```

## Documentation

| Document | Topic |
|---|---|
| [`CLAUDE.md`](CLAUDE.md) | AI-assisted development contract + index |
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | System architecture and data flow |
| [`ROADMAP.md`](ROADMAP.md) | Phased delivery plan |
| [`docs/BACKEND.md`](docs/BACKEND.md) | Go control plane |
| [`docs/FRONTEND.md`](docs/FRONTEND.md) | Next.js dashboard |
| [`docs/DATABASE.md`](docs/DATABASE.md) | Schema, migrations, Redis |
| [`docs/API.md`](docs/API.md) | REST API conventions and reference |
| [`docs/HOSTING.md`](docs/HOSTING.md) | Docker/Caddy hosting data plane |
| [`docs/HESTIA_FALLBACK.md`](docs/HESTIA_FALLBACK.md) | Hestia adoption triggers and migration plan |
| [`docs/INFRASTRUCTURE.md`](docs/INFRASTRUCTURE.md) | Docker, monitoring, environments |
| [`docs/SECURITY.md`](docs/SECURITY.md) | Security practices |
| [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) | Build, deploy, rollback |
| [`docs/CODING_STANDARDS.md`](docs/CODING_STANDARDS.md) | Code conventions |
| [`docs/TESTING.md`](docs/TESTING.md) | Testing strategy |
| [`docs/UI_GUIDELINES.md`](docs/UI_GUIDELINES.md) | Design and UX rules |
| [`docs/CONTRIBUTING.md`](docs/CONTRIBUTING.md) | Contribution workflow |

## Contributing

Read [`docs/CONTRIBUTING.md`](docs/CONTRIBUTING.md) before opening a PR. In short:
branch off `main`, keep PRs small, use Conventional Commits, and make CI pass.

## License

Proprietary — © OpenCloud. All rights reserved. (Replace with your chosen license.)
