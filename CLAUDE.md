# CLAUDE.md — OpenCloud

> **Single source of truth for AI-assisted development.** Claude must read and
> follow this file before generating or modifying code. It is the contract; the
> detailed topic docs under [`docs/`](docs/) are its long form. When a request
> conflicts with this file, ask before deviating. Keep this file true.

This file stays **concise and authoritative**. Depth lives in the linked docs —
update the doc, not a copy here.

---

## 1. What this project is

OpenCloud is a custom **cloud shared-hosting platform** (a Hostinger/cPanel
alternative) with a bespoke dashboard and a Go control plane, driving **Hestia
Control Panel** as a provisioning backend. The Go backend is the system of
record; Hestia is never exposed to customers.

Full context: [`README.md`](README.md) · [`ARCHITECTURE.md`](ARCHITECTURE.md) ·
[`ROADMAP.md`](ROADMAP.md)

## 2. Tech stack (authoritative)

- **Backend:** Go · Gin · Bun ORM · PostgreSQL · Redis · Viper · Zap
  — approved libs: `golang-jwt/jwt/v5` (validate better-auth JWTs — ADR 0006) · `google/uuid` · `prometheus/client_golang` · `go-redis/redis_rate` · `testify` (tests). Full list: [`docs/BACKEND.md`](docs/BACKEND.md#13-approved-dependencies)
- **Frontend:** Next.js (App Router) · React · TypeScript · Tailwind CSS · shadcn/ui (dashboard/admin) · Astryx + StyleX (marketing — ADR 0007) · Lucide React · GSAP (marketing animations only)
  — approved libs (dashboard phase): TanStack Query · TanStack Table · react-hook-form + zod · Recharts · Vitest + Testing Library (tests)
- **Auth:** better-auth in the Next.js BFF (identity provider — ADR 0006); the Go backend validates its JWTs via JWKS and issues none.
- **Hosting:** Hestia · Nginx · Apache · PHP-FPM · MariaDB · Certbot · Cloudflare (DNS + Tunnel — ADR 0003) · BIND9 (fallback only)
- **Platform:** Docker · Docker Compose · Prometheus · Grafana · Fail2ban · UFW

Do **not** introduce technologies outside this list without explicit approval.

## 3. Where to find the rules

| Topic | Read before touching… | Doc |
|---|---|---|
| System design, data flow | architecture decisions | [`ARCHITECTURE.md`](ARCHITECTURE.md) |
| Go control plane | anything in `backend/` | [`docs/BACKEND.md`](docs/BACKEND.md) |
| Dashboard | the Next.js app (`app/` at repo root) | [`docs/FRONTEND.md`](docs/FRONTEND.md) |
| Schema, migrations, Redis | DB or models | [`docs/DATABASE.md`](docs/DATABASE.md) |
| REST conventions | endpoints | [`docs/API.md`](docs/API.md) |
| Hestia / hosting stack | provisioner | [`docs/HOSTING.md`](docs/HOSTING.md) |
| Docker, monitoring, env | infra/config | [`docs/INFRASTRUCTURE.md`](docs/INFRASTRUCTURE.md) |
| Auth, secrets, hardening | security-sensitive code | [`docs/SECURITY.md`](docs/SECURITY.md) |
| Build, deploy, rollback | release process | [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) |
| Naming, formatting, style | any code | [`docs/CODING_STANDARDS.md`](docs/CODING_STANDARDS.md) |
| Tests | any non-trivial logic | [`docs/TESTING.md`](docs/TESTING.md) |
| UI/UX, components, a11y | UI work | [`docs/UI_GUIDELINES.md`](docs/UI_GUIDELINES.md) |
| Branching, PRs, commits | committing | [`docs/CONTRIBUTING.md`](docs/CONTRIBUTING.md) |

Architecture decisions are recorded in [`docs/adr/`](docs/adr/).

## 4. Architecture in one screen

```
Customer/Admin → Cloudflare (DNS · Tunnel) → Next.js dashboard → Go/Gin API (/api/v1, JWT)
                                               ├─ PostgreSQL (Bun)   system of record · job queue
                                               ├─ Redis              cache · sessions · rate limits
                                               └─ Provisioner ──→ Hestia node
                                                              │      (Nginx · Apache · PHP-FPM ·
                                                              │       MariaDB · Certbot)
                                                              └──→ Cloudflare API (DNS zones)
```

**Backend layering — never skip a layer:**
```
handler (Gin) → service (logic, transactions) → repository (Bun) → PostgreSQL
                       └→ provisioner (only Hestia caller) → hosting node
```

- Handlers translate HTTP ↔ domain; **no business logic, no DB access**.
- Services own business rules and transactions; the only layer spanning repos/provisioner.
- Repositories own all DB access; **every customer query is scoped by `account_id`**.
- The provisioner is the **only** thing that talks to a hosting node or the
  Cloudflare API, and it is idempotent.
- Work that can exceed ~1s is **enqueued as a `jobs` row** (same transaction as the
  write that triggered it) and handled by the worker, not run inline.

## 5. AI Coding Rules (apply every time)

1. **Read before writing.** Match existing patterns in the touched area. This file
   and the linked docs override defaults; when silent, follow the surrounding code.
2. **Stay in the lines** — respect layering, naming, and folder structure.
3. **Smallest correct change.** Shortest diff that fully solves the task. No
   unrequested abstractions, files, or dependencies.
4. **No new dependency** without justification that stdlib / an installed package
   can't do it — and confirmation.
5. **Don't invent contracts.** If an endpoint, schema, or type isn't defined,
   propose or ask — don't fabricate an API shape silently.
6. **Security and tenant-scoping are never simplified away.** Validate input at
   boundaries; scope every customer query by `account_id`.
7. **Non-trivial logic ships with a test** (a branch, loop, parser, money/auth path).
8. **Explain briefly, in proportion to the change.** Code first; note what you
   skipped and when to add it. No essays.
9. **Surface, don't paper over.** If a request conflicts with these docs, a
   migration looks destructive, or a fix hides a real bug — say so first.
10. **Keep docs true.** Change architecture/conventions/stack → update the relevant
    doc in the same PR, and add a `CHANGELOG.md` entry.

## 6. Things to never do

- ❌ Business logic in handlers or React components.
- ❌ DB access outside repositories; SQL built by string concatenation.
- ❌ Querying customer data without an `account_id` scope.
- ❌ Talking to a hosting node or the Cloudflare API from anywhere but the provisioner.
- ❌ Tokens in `localStorage`; logging secrets; committing `.env`.
- ❌ `any` in TypeScript; `interface{}` as a lazy escape hatch in Go.
- ❌ Floats for money; non-UTC timestamps; magic numbers.
- ❌ Editing a shipped migration instead of adding a new one.
- ❌ Long-running work synchronously inside an HTTP handler.
- ❌ Mixing component libraries **within a route group** — shadcn/ui owns `(dashboard)`/`(admin)`, Astryx owns `(marketing)` (ADR 0007); never both on one screen.
- ❌ Leaking stack traces, SQL, or Hestia output to API clients.
- ❌ Committing/pushing without the human's go-ahead; committing straight to `main`.

## 7. Quick command reference

```bash
# Backend (backend/)
go run ./cmd/api          # API server
go run ./cmd/worker       # job worker
go run ./cmd/migrate up   # migrations
go test ./...             # tests
golangci-lint run         # lint

# Frontend (repo root)
npm run dev               # dev server
npm run lint              # oxlint
npm run build             # production build

# Stack
docker compose up --build
```

---

*If you found this file stale while working, you were the right person to fix it.*
