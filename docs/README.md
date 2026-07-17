# docs/

Detailed documentation for OpenCloud. The contract and index live in
[`../CLAUDE.md`](../CLAUDE.md); the system map in [`../ARCHITECTURE.md`](../ARCHITECTURE.md).
This folder holds the per-topic depth.

## Topic docs

| Doc | Topic |
|---|---|
| [`BACKEND.md`](BACKEND.md) | Go control plane — layering, packages, conventions |
| [`FRONTEND.md`](FRONTEND.md) | Next.js dashboard — rendering, data, components |
| [`DEPENDENCIES.md`](DEPENDENCIES.md) | Dependency adoption blueprint — status, phase, and install gates |
| [`DATABASE.md`](DATABASE.md) | PostgreSQL schema, migrations, Redis |
| [`API.md`](API.md) | REST conventions + endpoint reference |
| [`HOSTING.md`](HOSTING.md) | Hestia and the hosting data plane |
| [`INFRASTRUCTURE.md`](INFRASTRUCTURE.md) | Docker, monitoring, environments |
| [`SECURITY.md`](SECURITY.md) | Auth, secrets, isolation, hardening |
| [`DEPLOYMENT.md`](DEPLOYMENT.md) | Build, release, rollback |
| [`CODING_STANDARDS.md`](CODING_STANDARDS.md) | Code conventions and naming |
| [`TESTING.md`](TESTING.md) | Testing strategy |
| [`UI_GUIDELINES.md`](UI_GUIDELINES.md) | Design and UX rules |
| [`CONTRIBUTING.md`](CONTRIBUTING.md) | Contribution workflow |

## Other folders

| Folder / file | Holds |
|---|---|
| `adr/` | Architecture Decision Records — one file per significant decision |
| `adr/0000-template.md` | Copy this to start a new ADR |
| `runbooks/` | Operational how-tos (incident response, restores). Added as procedures stabilize |

## Writing an ADR

1. Copy `adr/0000-template.md` to `adr/NNNN-short-title.md` (next number).
2. Fill it in — context, decision, consequences. Keep it short.
3. ADRs are **immutable** once `Accepted`. To change a decision, write a new ADR
   that supersedes the old one and mark the old one `Superseded`.

ADRs record *why*. If a reader (or Claude) asks "why is it built this way?", the
answer should be findable here.
