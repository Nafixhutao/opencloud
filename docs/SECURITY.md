# Security

Security practices for OpenCloud. A hosting platform is a high-value target — it
holds customer credentials and runs customer code on shared infrastructure. These
rules are not optional and are never "simplified away" (see [`../CLAUDE.md`](../CLAUDE.md)).

---

## 1. Threat model (summary)

- **Cross-tenant access** — a customer reaching another's data/files/processes.
  *Mitigation:* `account_id` scoping (app + DB) + per-site Docker
  network/volume/runtime policy; Hestia remains the stronger OS-user fallback.
- **Credential theft** — stolen tokens, passwords, or node keys.
  *Mitigation:* hashing, short-lived tokens, httpOnly cookies, secret manager.
- **Injection** — SQL, command, or template injection via customer input.
  *Mitigation:* parameterized queries, typed provisioner inputs, input validation.
- **Abuse / DoS** — brute force, scraping, resource exhaustion.
  *Mitigation:* rate limiting, Fail2ban, UFW, quotas, timeouts.
- **Supply chain** — vulnerable dependencies.
  *Mitigation:* pinned versions, `govulncheck` / `npm audit` in CI.

## 2. Authentication

Authentication is owned by **better-auth** in the Next.js BFF, not the Go backend
([ADR 0006](adr/0006-better-auth-identity-provider.md)).

- **better-auth** handles email/password, Google/GitHub social login, sessions,
  and email verification. Passwords are hashed by better-auth (scrypt default);
  the Go backend holds **no** password or OAuth code. `state`/PKCE and
  provider-verified-email auto-linking are better-auth's.
- Email/password login requires a verified address. Verification and password
  reset links expire after one hour; reset tokens and verification claims are
  single-use. OpenCloud stores only a SHA-256 verification-token digest outside
  Better Auth and never logs a token, reset URL, cookie, password, or secret.
- **JWT** is issued by better-auth's `jwt` plugin — asymmetric, exposed at a
  **JWKS** endpoint — carrying `sub` plus custom claims (`account_id`, `role`).
  The Go backend is a **resource server**: `middleware/auth` validates the JWT
  against the cached JWKS on every protected route; it **issues no tokens**.
  Protected routes then re-read the current membership and replace the JWT role
  with the database role. Demotion, suspension, or disable therefore takes
  effect immediately for already-issued bearer JWTs.
- **Sessions** (rotation and revocation) live in better-auth (`auth.session`),
  short-lived by config.
- **Sensitive actions** (delete account, change billing, role change) require
  re-authentication or 2FA where configured.

## 3. Tokens in the frontend

- Tokens live in **httpOnly, Secure, SameSite cookies** — **never** `localStorage`
  or client-readable JS.
- The Next.js BFF attaches the token to backend calls **server-side**; client code
  never sees it. See [`FRONTEND.md`](FRONTEND.md#3-data-fetching--the-bff).
- CSRF is mitigated by `SameSite` cookies plus a CSRF token on state-changing
  cross-site requests where applicable.

## 4. Authorization (RBAC)

- Roles: `customer`, `admin` (extendable). Enforced **server-side** in
  middleware/services — hiding a button in the UI is not authorization.
- **Tenant scoping is the #1 invariant:** every customer data path is scoped by
  `account_id`. A missing scope is a vulnerability, not a style nit. Admin
  cross-account access is a separate, explicit, **audited** path.
- `admin` is a **global platform-operator role**, not a tenant administrator.
  Only `/api/v1/admin/*` may cross account boundaries. Customer routes remain
  scoped to the JWT account and never inherit platform-wide access.
- Resources the caller can't access return `404`, not `403`, to avoid leaking
  existence ([`API.md`](API.md#3-status-codes)).

## 5. Input validation & injection

- **Validate and sanitize all input at trust boundaries.** Treat every request as
  hostile; bind + validate DTOs in handlers ([`BACKEND.md`](BACKEND.md#5-http-layer-gin)).
- **Parameterized queries only** — Bun handles this. Never build SQL by string
  concatenation.
- **No raw shell interpolation** into Docker, Caddy, or fallback-provider
  operations — typed, validated arguments only. Validate customer-supplied values
  (domains, image/template IDs, DB names, environment keys) against strict
  allowlists before they reach the provisioner.
- Escape/encode output; the frontend relies on React's escaping plus a strict CSP.

## 6. Secrets management

- All secrets via environment / secret manager, loaded by Viper. **Never commit
  `.env`**; ship `.env.example` with documented, non-secret defaults.
- No secrets in source, logs, images, or client bundles / `NEXT_PUBLIC_*`.
- Rotate credentials on exposure; scope each credential to least privilege
  (permissioned Docker boundary, loopback/private Caddy admin API, scoped fallback
  Hestia access key, and a DB user that can't `DROP`).
- Redact secrets at the logging boundary ([`BACKEND.md`](BACKEND.md#11-logging-zap)).

## 7. Transport security

- **HTTPS everywhere** with HSTS. Caddy manages customer certificates and renewal
  ([`HOSTING.md`](HOSTING.md#5-domains-and-https)).
- TLS for PostgreSQL and Redis connections in production.
- Internal-only services (metrics, datastores) bound to the private network, never
  the public internet.

## 8. HTTP headers & CORS

- Strict **Content-Security-Policy**, `X-Content-Type-Options: nosniff`,
  `X-Frame-Options`/frame-ancestors, `Referrer-Policy`, `Strict-Transport-Security`.
- **CORS allowlist** (`CORS_ORIGINS`) — never `*` in production.

## 9. Rate limiting & abuse prevention

- Redis-backed rate limits on auth and expensive endpoints; stricter budgets on
  login/refresh.
- **Fail2ban** bans IPs after repeated auth failures (SSH and app).
- **UFW** on every host: default-deny inbound, only required ports open.
- Per-account resource quotas on nodes prevent one tenant exhausting a host.
- Timeouts/deadlines on every external call so a slow node can't exhaust pools.

## 10. Node & host hardening

- Containers run as **non-root**, minimal images, read-only filesystems where possible.
- Site containers drop capabilities, use `no-new-privileges`, dedicated networks
  and volumes, and enforce CPU/memory/PID limits. No privileged containers or
  arbitrary host mounts.
- The Docker daemon and Caddy admin endpoint are worker-only. Neither is mounted
  into the dashboard or public API; arbitrary Dockerfiles remain disabled until
  isolated builds and image policy exist.
- SSH: key-only, no root login, bastion-restricted. Automatic security updates.
- Hestia fallback nodes follow [`HESTIA_FALLBACK.md`](HESTIA_FALLBACK.md) and run
  on separate clean hosts.

## 11. Dependency & supply-chain security

- Pin dependency versions (`go.mod`, `package-lock.json`).
- CI runs `govulncheck` (Go) and `npm audit` (frontend); patch promptly.
- Review new dependencies before adding — prefer stdlib/installed packages
  ([`../CLAUDE.md`](../CLAUDE.md)).

## 12. Audit logging

- Sensitive actions — provisioning, suspension, deletion, billing, role and
  password changes — are written to an **append-only** `audit_logs` trail with
  actor, target, and metadata ([`DATABASE.md`](DATABASE.md#3-core-schema)).
- Audit logs are retained and protected from tampering; they are not customer-editable.
- Profile, bootstrap, and role/status mutations append audit rows in the same
  PostgreSQL transaction. Database triggers reject audit UPDATE/DELETE. Better
  Auth owns password credential transactions in `auth.*`; if the following
  domain audit append fails, the API returns `AUDIT_FAILED` and never reports
  success. A durable outbox is deferred until cross-database delivery exists.

## 13. Data protection & privacy

- Customer DB credentials are shown **once** on creation and not stored in
  plaintext.
- Money stored as integer minor units; PII access is logged.
- Backups are encrypted at rest and access-controlled ([`DATABASE.md`](DATABASE.md#9-backups)).

## 14. Secure SDLC

- Threat-aware code review on every PR; security-sensitive changes flagged.
- Secrets scanning is added before production credentials enter the repository.
- No security control is removed to "simplify" — if a change weakens one, call it
  out explicitly and get sign-off.

## 15. Incident response

- On suspected compromise: rotate affected credentials, revoke refresh-token
  chains, review `audit_logs`, and isolate the affected node (`draining` → offline).
- Post-incident: write/refresh a runbook in `docs/runbooks/` and an ADR if the
  architecture changes. Notify per policy/regulation.

## 16. Admin bootstrap (Phase 1)

Admin role is **never** granted via signup or client-supplied fields. Operators
promote a user explicitly and idempotently:

```
docker compose exec api /app/bootstrap-admin --user-id <better-auth-user-id>
```

Or on a host with DATABASE_URL:

```
go run ./cmd/bootstrap-admin --user-id <id>
```

Password reset tokens are single-use, expire (default 1h), stored hashed under
better-auth `auth.verification`, and must never appear in application logs.
Mail delivery uses `MAIL_PROVIDER` (`log` / `memory` / `smtp`). `log` and
`memory` never deliver and are forbidden in production. Production requires a
real SMTP host, sender, username/password, TLS 1.2+, and valid certificates;
startup fails fast when incomplete. Production email is not claimed active
until those external credentials are configured and a staging delivery test
passes.
