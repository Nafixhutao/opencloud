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

## 5. Errors {#errors}

- The backend maps typed errors (`apperr.*`) → status + envelope centrally; see
  [`BACKEND.md`](BACKEND.md#10-error-handling).
- **Never leak internals** — no SQL, stack traces, or Hestia output in responses.
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
- `401` → refresh via `/auth/refresh`; on refresh failure, re-authenticate.
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

## 11. Endpoint reference (illustrative)

Endpoints are added as features ship ([`../ROADMAP.md`](../ROADMAP.md)). The set
below shows the intended shape and naming.

### Auth
| Method | Path | Purpose |
|---|---|---|
| `POST` | `/auth/register` | create account + first user |
| `POST` | `/auth/login` | issue access + refresh tokens |
| `GET`  | `/auth/oauth/{provider}` | start Google/GitHub OAuth flow (redirect, `state` CSRF param — ADR 0005) |
| `GET`  | `/auth/oauth/{provider}/callback` | exchange code → issue the same access + refresh tokens |
| `POST` | `/auth/refresh` | rotate tokens |
| `POST` | `/auth/logout` | revoke refresh token |
| `GET`  | `/auth/me` | current user + account |

### Sites
| Method | Path | Purpose |
|---|---|---|
| `GET`    | `/sites` | list caller's sites (paginated) |
| `POST`   | `/sites` | create a site (async → `202`) |
| `GET`    | `/sites/{id}` | site detail + status |
| `PATCH`  | `/sites/{id}` | update settings |
| `POST`   | `/sites/{id}/suspend` | suspend (admin/billing) |
| `DELETE` | `/sites/{id}` | delete (async → `202`) |

### Domains / DNS
| Method | Path | Purpose |
|---|---|---|
| `GET`  | `/sites/{id}/domains` | domains on a site |
| `POST` | `/sites/{id}/domains` | attach a domain |
| `GET`  | `/dns/zones/{id}/records` | list DNS records |
| `POST` | `/dns/zones/{id}/records` | create a record |

### Databases
| Method | Path | Purpose |
|---|---|---|
| `GET`  | `/databases` | list databases |
| `POST` | `/databases` | provision a MariaDB database (async) |

### Admin
| Method | Path | Purpose |
|---|---|---|
| `GET`  | `/admin/accounts` | list all accounts (admin) |
| `GET`  | `/admin/nodes` | hosting node status |
| `POST` | `/admin/nodes/{id}/drain` | drain a node |

### System
| Method | Path | Purpose |
|---|---|---|
| `GET` | `/healthz` | liveness |
| `GET` | `/readyz` | readiness (DB + Redis reachable) |
| `GET` | `/metrics` | Prometheus metrics (internal network only) |
