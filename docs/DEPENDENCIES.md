# Dependency Adoption Blueprint

This document records **which supporting libraries OpenCloud intends to use and
when**. It is a plan, not an instruction to install everything now. The actual
installed set is always `package.json` / `backend/go.mod`.

OpenCloud keeps **Next.js** as the frontend/BFF framework and **Gin** as the Go
API framework. TanStack libraries complement Next.js where needed; TanStack
Start and TanStack Router do not replace it.

## 1. Status and adoption rules

| Status | Meaning |
|---|---|
| **Adopted** | Already present in the relevant manifest or generated component source |
| **Planned** | Approved direction for a named phase; add with its first real consumer |
| **Conditional** | Add only after the stated need is demonstrated |
| **Not planned** | Overlaps the chosen stack or solves no current requirement |

For every new dependency:

1. Prefer an existing package, a platform feature, or the Go/JavaScript standard
   library first.
2. Install it in the same change as its first consumer, with the smallest useful
   configuration.
3. Record why the existing stack is insufficient and obtain the confirmation
   required by [`../CLAUDE.md`](../CLAUDE.md#5-ai-coding-rules).
4. Update the manifest, lockfile, tests, and relevant topic doc together.

“Planned” does **not** authorize an early install.

## 2. Frontend and BFF

| Need | Choice | Status / phase | Adoption trigger |
|---|---|---|---|
| Framework and routing | Next.js App Router + React | **Adopted** | Core framework; keep Next.js routing |
| Authentication | better-auth | **Adopted — Phase 1** | Owns identity, sessions, social login, JWT/JWKS |
| Dashboard/admin UI | shadcn/ui + Tailwind CSS | **Adopted — Phase 1** | Add primitives per screen, never `--all` |
| Marketing UI | Astryx + StyleX | **Planned — marketing rework** | First rebuilt marketing route; keep separate from shadcn route groups |
| Forms and client validation | react-hook-form + zod + resolver | **Adopted — Phase 1** | Shared form pattern; backend still re-validates |
| Interactive server state | TanStack Query | **Planned — Phase 2** | Client mutations, cache invalidation, retry, or polling provisioning until `active` / `failed` |
| Complex data tables | TanStack Table | **Planned — Phase 2+** | Sorting, filtering, pagination, selection, or reusable table behavior exceeds a simple semantic table |
| Usage charts | Recharts | **Planned — Phase 5** | Real metering/billing time-series data is available |
| Component tests | Vitest + React Testing Library | **Planned — Phase 1** | First logic-bearing dashboard tests; do not test static presentation for its own sake |
| Browser journeys | Playwright | **Planned — Phase 2** | A complete signup → login → provision → delete journey can run against a stack |
| Localization | next-intl | **Conditional — Phase 7** | A second supported locale and translation workflow are committed |
| Very large lists | TanStack Virtual | **Conditional** | Measured rendering problems from hundreds/thousands of visible rows |

Detailed rendering, state, and component rules remain in
[`FRONTEND.md`](FRONTEND.md); test scope remains in
[`TESTING.md`](TESTING.md).

## 3. API contract and backend

| Need | Choice | Status / phase | Adoption trigger |
|---|---|---|---|
| REST contract generation | OpenAPI + `oapi-codegen` + `openapi-typescript` | **Planned — Phase 2** | First stable resource contracts would otherwise duplicate Go DTOs and TypeScript types |
| Cloudflare DNS | Official Cloudflare Go client | **Planned — Phase 3** | Provisioner implements zone and record management |
| PostgreSQL integration tests | Testcontainers for Go | **Planned — Phase 2** | Repository/job-queue tests need isolated real PostgreSQL lifecycle beyond the CI service container |
| Metrics | Prometheus Go client | **Adopted** | Existing API and worker metrics |
| Distributed tracing | OpenTelemetry for Go | **Conditional — Phase 6** | API → database/job → worker/provisioner traces have an operating collector and a concrete diagnostic need |

OpenAPI generators may produce contract types/clients, but they must not bypass
the handler → service → repository layering in [`BACKEND.md`](BACKEND.md).
Cloudflare calls remain inside the provisioner.

## 4. Deliberately not planned

| Library/framework | Reason |
|---|---|
| TanStack Start / TanStack Router | Next.js already owns the framework and routing |
| TanStack Form | react-hook-form + zod is the chosen form stack |
| SWR | Overlaps TanStack Query |
| Axios | Native `fetch` and Go `net/http` cover current HTTP needs |
| Redux / Zustand | Local React state, URL state, and small contexts cover the current client-state model |
| NextAuth/Auth.js / Clerk | better-auth already owns authentication |
| Prisma / Drizzle | PostgreSQL access belongs to the Go backend using Bun + pgx |
| tRPC | The frontend/backend contract is REST/JSON |
| Asynq / BullMQ | The transactional PostgreSQL job queue is fixed by ADR 0002 |
| A WebSocket library | TanStack Query polling is sufficient for initial provisioning status |

Reconsider a “not planned” item only when a measured requirement appears; a
significant architecture change requires an ADR.
