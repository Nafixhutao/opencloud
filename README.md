# OpenCloud

> A modern, custom-built cloud **shared-hosting platform** — an alternative to
> Hostinger, cPanel, and DirectAdmin with a bespoke dashboard and Go control
> plane, powered underneath by Hestia Control Panel.

OpenCloud lets customers provision and manage websites, domains, databases,
email, DNS, and SSL through a fast custom dashboard — while operators manage
servers, plans, and customers from a dedicated admin panel. The stock Hestia UI
is never exposed; OpenCloud's Go backend is the system of record and drives
Hestia as a provisioning backend.

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
- **Databases** — provision and manage MariaDB databases and users.
- **SSL** — automatic Let's Encrypt issuance and renewal via Certbot.
- **Email, FTP/SSH, cron** — full account lifecycle per customer (email lands
  post-launch — [ADR 0004](docs/adr/0004-external-services-at-launch.md)).
- **Resource monitoring** — per-account CPU, RAM, disk, and bandwidth in Grafana.
- **Multi-tenant isolation** — strict per-customer separation enforced end to end.
- **Automation-first** — provisioning, suspension, and teardown are fully API-driven.

## Tech Stack

| Layer | Technologies |
|---|---|
| **Backend** | Go · Gin · Bun ORM · PostgreSQL · Redis · Viper · Zap |
| **Frontend** | Next.js · React · TypeScript · Tailwind CSS · shadcn/ui · Lucide React · GSAP |
| **Hosting** | Hestia Control Panel · Nginx · Apache · PHP-FPM · MariaDB · Certbot · Cloudflare (DNS + Tunnel) |
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
└── docker-compose.yml    # backend + migration gate + datastores
```

> **Status:** the repo contains a minimal Next.js shell and the Phase 0 Go
> backend scaffold. See [`ROADMAP.md`](ROADMAP.md) for what is implemented.

## Quick Start

### Prerequisites

- **Docker** 24+ and **Docker Compose** v2
- **Go** 1.26+ (for backend dev outside Docker)
- **Node.js** 20+ and **npm** (for frontend dev)
- A Linux host running **Hestia Control Panel** for real provisioning
  (optional for UI/backend dev — the provisioner can run against a fake)

### Run the backend stack (Docker)

```bash
git clone <repo-url> opencloud
cd opencloud
cp .env.example .env          # then edit secrets
docker compose up --build
```

The current Compose stack runs migrations, then starts the API, worker,
PostgreSQL, and Redis. Frontend and monitoring services land in later phases.

| Service | URL |
|---|---|
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
| `HESTIA_API_URL` / `HESTIA_API_KEY` | Hosting node access |
| `LOG_LEVEL` | `debug` / `info` / `warn` / `error` |

See [`docs/INFRASTRUCTURE.md`](docs/INFRASTRUCTURE.md) for the full reference.

## Common Tasks

```bash
# Backend (from repo root)
(cd backend && go test ./...)                       # run tests
(cd backend && golangci-lint run)                   # lint
(cd backend && go run ./cmd/migrate up)             # apply DB migrations

# Frontend
npm run lint                        # oxlint
npm run build                       # production build

# Stack
docker compose logs -f api          # tail the API service
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
| [`docs/HOSTING.md`](docs/HOSTING.md) | Hestia and the hosting stack |
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
