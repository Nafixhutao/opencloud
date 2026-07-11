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
├── src/                  # components, styles, assets
├── public/               # static assets
├── next.config.ts        # Next.js config
├── package.json          # frontend dependencies + scripts
├── backend/              # Go control plane (planned)
├── deploy/               # prometheus, grafana, nginx, hestia bootstrap
├── docs/                 # detailed topic docs (see below)
└── docker-compose.yml    # control plane + datastores + monitoring
```

> **Status:** the Next.js frontend lives at the repo root (migrated off Vite); the
> Go backend is greenfield. `backend/`, `deploy/`, and `docker-compose.yml` above
> describe the target layout. See [`ROADMAP.md`](ROADMAP.md) for what's built.

## Quick Start

### Prerequisites

- **Docker** 24+ and **Docker Compose** v2
- **Go** 1.25+ (for backend dev outside Docker)
- **Node.js** 20+ and **npm** (for frontend dev)
- A Linux host running **Hestia Control Panel** for real provisioning
  (optional for UI/backend dev — the provisioner can run against a fake)

### Run the full stack (Docker)

```bash
git clone <repo-url> opencloud
cd opencloud
cp .env.example .env          # then edit secrets
docker compose up --build
```

Services come up at:

| Service | URL |
|---|---|
| Frontend (dashboard) | http://localhost:3000 |
| Backend API | http://localhost:8080/api/v1 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3001 |

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
| `AUTH_JWKS_URL` | better-auth JWKS the API validates JWTs against (issues none — ADR 0006) |
| `BETTER_AUTH_SECRET` / `BETTER_AUTH_URL` | better-auth (BFF) identity provider config |
| `HESTIA_API_URL` / `HESTIA_API_KEY` | Hosting node access |
| `LOG_LEVEL` | `debug` / `info` / `warn` / `error` |

See [`docs/INFRASTRUCTURE.md`](docs/INFRASTRUCTURE.md) for the full reference.

## Common Tasks

```bash
# Backend
go test ./...                       # run tests
golangci-lint run                   # lint
go run ./cmd/migrate up             # apply DB migrations

# Frontend
npm run lint                        # oxlint
npm run build                       # production build

# Stack
docker compose logs -f backend      # tail a service
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
