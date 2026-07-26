# API Standards & Reference

The OpenCloud backend exposes a versioned REST/JSON API consumed by the Next.js
dashboard (and, later, public API clients). This document is the contract for
**how** the API behaves; endpoint depth grows as features land.

Base URL: `/api/v1` · Format: JSON · Auth: JWT (see [`SECURITY.md`](SECURITY.md)).

---

## 1. Principles

- **Resource-oriented**, plural nouns: `/sites`, `/sites/{id}/domains`.
- **HTTP methods are the verbs** — no verbs in paths (`/createSite` is wrong).
- **Consistent envelopes** for success and error (below).
- **All fields `snake_case`**; timestamps are ISO-8601 UTC.
- **Predictable status codes**; the client can branch on them safely.
- **Versioned** — breaking changes bump `/v1` → `/v2`; additive changes don't.

## 2. Methods

| Method | Use | Idempotent | Body |
|---|---|---|---|
| `GET` | read | yes | no |
| `POST` | create / trigger action | no | yes |
| `PUT` | full replace | yes | yes |
| `PATCH` | partial update | no | yes |
| `DELETE` | remove | yes | no |

## 3. Status codes

| Code | Meaning |
|---|---|
| `200 OK` | successful read/update |
| `201 Created` | resource created synchronously |
| `202 Accepted` | async operation queued (provisioning) — poll for status |
| `204 No Content` | successful delete / empty success |
| `400 Bad Request` | malformed request |
| `401 Unauthorized` | missing/invalid auth |
| `403 Forbidden` | authenticated but not allowed |
| `404 Not Found` | resource doesn't exist (or not in caller's account) |
| `409 Conflict` | state conflict (e.g. domain already taken) |
| `422 Unprocessable Entity` | validation failed |
| `429 Too Many Requests` | rate-limited |
| `5xx` | our fault — logged, generic message to client |

> A resource the caller isn't entitled to returns `404`, not `403`, so we don't
> leak the existence of other tenants' resources.

## 4. Response envelopes

**Success** — single resource:
```json
{ "data": { "id": "9f3…", "domain": "example.com", "status": "active" } }
```

**Success** — collection with pagination:
```json
{
  "data": [ { "id": "…" }, { "id": "…" } ],
  "meta": { "page": 1, "per_page": 25, "total": 142 }
}
```

**Error** — always this shape:
```json
{
  "error": {
    "code": "DOMAIN_TAKEN",
    "message": "That domain is already in use.",
    "details": [ { "field": "domain", "issue": "must be unique" } ]
  }
}
```

`code` is a **stable, machine-readable** string (clients branch on it).
`message` is human-readable. `details` is optional, used for validation.

## 5. Errors

- The backend maps typed errors (`apperr.*`) → status + envelope centrally; see
  [`BACKEND.md`](BACKEND.md#10-error-handling).
- **Never leak internals** — no SQL, stack traces, or hosting-provider output in responses.
- Validation errors (`422`) list offending fields in `details`.
- Common codes: `VALIDATION_FAILED`, `UNAUTHENTICATED`, `FORBIDDEN`, `NOT_FOUND`,
  `CONFLICT`, `DOMAIN_TAKEN`, `RATE_LIMITED`, `INTERNAL`.

## 6. Pagination, filtering, sorting

- Pagination: `?page=1&per_page=25`. `per_page` defaults to 25 and is capped
  (e.g. 100) to protect the DB. Total returned in `meta.total`.
- Large/hot collections may use cursor pagination: `?cursor=…&limit=…` returning
  `meta.next_cursor`.
- Filtering: explicit query params (`?status=active`). No arbitrary query DSL.
- Sorting: `?sort=created_at&order=desc` against an allowlist of sortable fields.

## 7. Authentication

- Send the access token as `Authorization: Bearer <jwt>`. The dashboard does this
  server-side from an httpOnly cookie — tokens never live in client JS.
- `401` → the BFF refreshes the session via better-auth; on failure, re-authenticate.
- Authorization (RBAC) is enforced server-side; hiding a button in the UI is not
  security. Full model: [`SECURITY.md`](SECURITY.md).

## 8. Idempotency

- Unsafe operations that may be retried (provisioning, billing) accept an
  `Idempotency-Key` header. The server stores the key + result so a retry returns
  the original outcome instead of acting twice.

## 9. Async operations

Long operations return `202` with the created resource and a status to poll:

```http
POST /api/v1/sites
202 Accepted
{ "data": { "id": "…", "domain": "example.com", "status": "provisioning" } }
```
```http
GET /api/v1/sites/{id}
200 OK
{ "data": { "id": "…", "status": "active" } }   // or "failed"
```

## 10. Versioning & deprecation

- Additive changes (new fields/endpoints) ship within `/v1`.
- Breaking changes introduce `/v2`; `/v1` is supported through a published window.
- Deprecations are announced in [`../CHANGELOG.md`](../CHANGELOG.md) and via a
  `Deprecation` response header before removal.

## 11. Endpoint reference

Endpoints are added as features ship ([`../ROADMAP.md`](../ROADMAP.md)). Entries
explicitly marked planned are not implemented yet.

### Auth

Auth is served by **better-auth** in the Next.js BFF under **`/api/auth/*`** —
**not** this Go API ([ADR 0006](adr/0006-better-auth-identity-provider.md)): it
owns sign-up, sign-in, social login, session, logout, and the JWT/JWKS endpoints.
The Go API exposes **no** auth endpoints; every route below requires a valid
better-auth JWT, and the current user/tenant is read from its claims.

### Sites
| Method | Path | Purpose |
|---|---|---|
| `GET`    | `/sites?page=&per_page=` | list caller's sites (paginated) |
| `POST`   | `/sites` | create a curated static site (async → `202`; accepts `Idempotency-Key`) |
| `GET`    | `/sites/{id}` | caller-owned site detail + status |
| `POST`   | `/sites/{id}/suspend` | suspend caller-owned site (async → `202`) |
| `POST`   | `/sites/{id}/resume` | resume caller-owned site (async → `202`) |
| `DELETE` | `/sites/{id}` | delete caller-owned site (async → `202`) |

Create body:

```json
{ "domain": "site.example.com", "template": "static" }
```

Status progresses through `provisioning`, `active`, `suspending`, `suspended`,
`resuming`, `deleting`, `deleted`, or `failed`. The dashboard polls only while a
site is in a transitional state. All reads and writes include the authenticated
`account_id`; another tenant's UUID returns `404`. Customer responses omit
control-plane placement, image, port, and resource-limit fields.

### Domains / DNS
| Method | Path | Purpose |
|---|---|---|
| `GET`  | `/sites/{id}/domains` | planned: domains on a site |
| `POST` | `/sites/{id}/domains` | planned: attach a domain |
| `GET`  | `/dns/zones/{id}/records` | planned: list DNS records |
| `POST` | `/dns/zones/{id}/records` | planned: create a record |

### Databases
| Method | Path | Purpose |
|---|---|---|
| `GET`  | `/databases` | planned: list databases |
| `POST` | `/databases` | planned: provision a scoped database (async) |

### Admin
| Method | Path | Purpose |
|---|---|---|
| `GET`  | `/admin/accounts` | planned: list all accounts |
| `GET`  | `/admin/nodes` | audited global platform-admin node list |
| `POST` | `/admin/nodes` | audited global platform-admin node registration |
| `PATCH` | `/admin/nodes/{id}` | audited status change: `online`, `draining`, or `offline` |

### System
| Method | Path | Purpose |
|---|---|---|
| `GET` | `/healthz` | liveness |
| `GET` | `/readyz` | readiness (DB + Redis reachable) |
| `GET` | `:9090/metrics` | Prometheus metrics (separate internal listener) |

## 12. Phase 1 endpoints

### `GET /api/v1/me`
Returns the caller's membership + account (requires JWT with `account_id` +
`role`). The API re-checks current membership role/status in PostgreSQL, so a
stale token cannot preserve demoted or suspended access.

### `PATCH /api/v1/me`
Body: `{ "name": string }` — updates the tenant account display name for the JWT account only.

### `GET /api/v1/admin/users?page=&per_page=`
Global platform-admin only. Paginated memberships with safe user name/email and
account identity; implemented as a count plus one joined query. Cross-account
listing is explicitly audited.

### `GET /api/v1/admin/users/{membership_id}`
Global platform-admin only. Single safe membership/user/account projection;
cross-account access is audited.

### `PATCH /api/v1/admin/users/{membership_id}`
Global platform-admin only. Body: `{ "role"?: "customer"|"admin", "status"?: "active"|"suspended"|"disabled" }`.
Refuses self-disable/demote and atomically prevents removal of the last active
platform admin. Mutation and audit append commit together.

Identity (register/login/session/password reset) is under `/api/auth/*` on the
Next.js BFF (better-auth), not the Go API.
